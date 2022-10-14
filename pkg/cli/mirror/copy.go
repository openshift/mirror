package mirror

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	semver "github.com/blang/semver/v4"
	imagecopy "github.com/containers/image/v5/copy"
	"github.com/otiai10/copy"

	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	gocontreg "github.com/google/go-containerregistry/pkg/v1"
	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"
)

type style string

const (
	blobsPath           string = "/blobs/sha256/"
	ociProtocol         string = "oci:"
	configPath          string = "configs/"
	catalogJSON         string = "/catalog.json"
	relatedImages       string = "relatedImages"
	configsLabel        string = "operators.operatorframework.io.index.configs.v1"
	ociStyle            style  = "oci"
	originStyle         style  = "origin"
	artifactsFolderName string = "olm_artifacts"
)

// RemoteRegFuncs contains the functions to be used for working with remote registries
// In order to be able to mock these external packages,
// we pass them as parameters of bulkImageCopy and bulkImageMirror
type RemoteRegFuncs struct {
	pull           func(src string, opt ...crane.Option) (gocontreg.Image, error)
	saveOCI        func(img gocontreg.Image, path string) error
	saveLegacy     func(img gocontreg.Image, src, path string) error
	load           func(path string, opt ...crane.Option) (gocontreg.Image, error)
	push           func(ctx context.Context, policyContext *signature.PolicyContext, destRef types.ImageReference, srcRef types.ImageReference, options *imagecopy.Options) (copiedManifest []byte, retErr error)
	mirrorMappings func(cfg v1alpha2.ImageSetConfiguration, images image.TypedImageMapping, insecure bool) error
}

// getISConfig - simple function to read and unmarshal the imagesetconfig
// set via the command line
func (o *MirrorOptions) getISConfig() (*v1alpha2.ImageSetConfiguration, error) {
	var isc *v1alpha2.ImageSetConfiguration
	configData, err := ioutil.ReadFile(o.ConfigPath)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(configData, &isc)
	if err != nil {
		return nil, err
	}
	return isc, nil
}

// •bulkImageCopy•used•to•copy the•relevant•images•(pull•from•a•registry)•to
// •a•local directory↵
func (o *MirrorOptions) bulkImageCopy(isc *v1alpha2.ImageSetConfiguration, srcSkipTLS, dstSkipTLS bool, remoteRegFuncs RemoteRegFuncs) error {

	mapping := image.TypedImageMapping{}

	// artifactsPath is a folder within the destination folder used for copying.
	// It is used to untar the catalog contents, in order to process it and prepare copy.
	// For each operator, a folder with the name of the operator will be created under artifactsPath (ctlcConfigsDir)
	artifactsPath := filepath.Join(o.OutputDir, artifactsFolderName)

	for _, operator := range isc.Mirror.Operators {

		klog.Infof("downloading the catalog image %s\n", operator.Catalog)
		_, _, repo, _, _ := parseImageName(operator.Catalog)
		localOperatorDir := filepath.Join(o.OutputDir, repo)
		if err := os.RemoveAll(localOperatorDir); err != nil {
			klog.Warningf("unable to clear contents of %s: %v", localOperatorDir, err)
		}
		//Check if this is an OCI FBC catalog
		if strings.HasPrefix(operator.Catalog, ociProtocol) {
			// OCI catalog is already on disk. using it
			catalogPath := trimProtocol(operator.Catalog)
			err := copy.Copy(catalogPath, localOperatorDir)
			if err != nil {
				return fmt.Errorf("copying catalog image %s : %v", operator.Catalog, err)
			}
		} else {
			err := pullImage(operator.Catalog, localOperatorDir, o.SourceSkipTLS, ociStyle, remoteRegFuncs)
			if err != nil {
				return fmt.Errorf("copying catalog image %s : %v", operator.Catalog, err)
			}
		}

		// find the layer with the FB config
		catalogContentsDir := filepath.Join(artifactsPath, repo)
		klog.Infof("Finding file based config for %s (in catalog layers)\n", operator.Catalog)
		ctlgConfigsDir, err := o.findFBCConfig(localOperatorDir, catalogContentsDir)
		if err != nil {
			return fmt.Errorf("unable to find config in %s: %v", localOperatorDir, err)
		}

		klog.Infof("Filtering on selected packages for %s \n", operator.Catalog)
		files, err := ioutil.ReadDir(ctlgConfigsDir)
		if err != nil {
			log.Fatalf("unable to read catalog contents %v", err)
			return err
		}
		pkgList := []v1alpha2.IncludePackage{}
		if len(operator.Packages) > 0 {
			pkgList = operator.Packages
		} else {
			// take all packages found in catalog
			for _, file := range files {
				pkgList = append(pkgList, v1alpha2.IncludePackage{
					Name: file.Name(),
				})
			}
		}
		for _, pkg := range pkgList {

			klog.Infof("Collecting all related images for %s \n", pkg.Name)
			for _, file := range files {
				klog.V(2).Infof("File :%s\n", file.Name())
				// read the config.json to get releated images
				relatedImages, err := getRelatedImages(ctlgConfigsDir, []v1alpha2.IncludePackage{pkg})
				if err != nil {
					return err
				}
				for _, i := range relatedImages {
					if i.Image == "" {
						klog.Warningf("invalid related image %s: reference empty", i.Name)
						continue
					}
					name := i.Name
					if name == "" {
						//Creating a unique name for this image, that doesnt have a name
						name = fmt.Sprintf("%x", sha256.Sum256([]byte(i.Image)))[0:6]
					}
					srcTIR, err := image.ParseReference(i.Image)
					if err != nil {
						return err
					}
					srcTI := image.TypedImage{
						TypedImageReference: srcTIR,
						Category:            v1alpha2.TypeOperatorRelatedImage,
					}
					dstPath := "file://" + pkg.Name + "/" + name
					if srcTIR.Ref.ID != "" {
						dstPath = dstPath + "/" + strings.TrimPrefix(srcTI.Ref.ID, "sha256:")
					} else if srcTIR.Ref.ID == "" && srcTIR.Ref.Tag != "" {
						//recreating a fake digest to copy image into
						//this is because dclcfg.LoadFS will create a symlink to this folder
						//from the tag
						dstPath = dstPath + "/" + fmt.Sprintf("%x", sha256.Sum256([]byte(srcTIR.Ref.Tag)))[0:6]
					}
					dstTIR, err := image.ParseReference(dstPath)
					if err != nil {
						return err
					}
					if srcTI.Ref.Tag != "" {
						//put the tag back because it's needed to follow symlinks by LoadFS
						dstTIR.Ref.Tag = srcTI.Ref.Tag
					}
					dstTI := image.TypedImage{
						TypedImageReference: dstTIR,
						Category:            v1alpha2.TypeOperatorRelatedImage,
					}
					mapping[srcTI] = dstTI
				}
				break
			}
		}
	}

	o.Dir = strings.TrimPrefix(o.Dir, o.OutputDir+"/")
	if len(mapping) > 0 {
		err := remoteRegFuncs.mirrorMappings(*isc, mapping, srcSkipTLS)
		if err != nil {
			return err
		}
	} else {
		klog.Infof("no images to copy")
	}

	return nil
}

// bulkImageMirror used to mirror the relevant images (push from a directory) to
// a remote registry in oci format
func (o *MirrorOptions) bulkImageMirror(isc *v1alpha2.ImageSetConfiguration, destRepo, namespace string, remoteRegFuncs RemoteRegFuncs) error {
	mapping := image.TypedImageMapping{}

	for _, operator := range isc.Mirror.Operators {
		_, _, repo, _, _ := parseImageName(operator.Catalog)

		// Mirroring workflow assumes a Copy workflow has been executed before.
		// In this case, the copy workflow has untarred the contents of the catalog to
		// an artifacts subfolder under the destination directory previously set for the copy.
		// Example:
		// Copy workflow used oci://mydir as the destination folder to copy catalog mycatalog.
		// The OCI image for mycatalog will be in mydir/mycatalog.
		// The untarred contents of mycatalog will be in mydir/olm_artifacts/operator1..n
		// Here we reconstruct the artifacts subfolder for each operator for the operator oci://mydir/mycatalog
		// artifactsPath represents in this example mydir/olm_artifacts
		artifactsPath := filepath.Join(filepath.Dir(trimProtocol(operator.Catalog)), artifactsFolderName)

		klog.Infof("Processing contents of local catalog %s\n", operator.Catalog)

		configsLabel, err := o.getCatalogConfigPath(trimProtocol(operator.Catalog))
		if err != nil {
			log.Fatalf("unable to retrieve configs layer for image %s:\n%v\nMake sure you run oc-mirror with --use-oci-feature and --oci-feature-action=copy prior to executing this step", operator.Catalog, err)
			return err
		}
		catalogContentsDir := filepath.Join(artifactsPath, repo, configsLabel)
		files, err := ioutil.ReadDir(catalogContentsDir)
		if err != nil {
			log.Fatalf("unable to read catalog contents for %s: %v", operator.Catalog, err)
			return err
		}
		pkgList := []v1alpha2.IncludePackage{}
		if len(operator.Packages) > 0 {
			pkgList = operator.Packages
		} else {
			// take all packages found in catalog
			for _, file := range files {
				pkgList = append(pkgList, v1alpha2.IncludePackage{
					Name: file.Name(),
				})
			}
		}

		for _, pkg := range pkgList {
			klog.Infof("Collecting all related images for %s \n", pkg.Name)

			relatedImages, err := getRelatedImages(catalogContentsDir, []v1alpha2.IncludePackage{pkg})
			if err != nil {
				klog.Fatal(err)
				return err
			}

			for _, i := range relatedImages {
				if i.Image == "" {
					klog.Warningf("invalid related image %s: reference empty", i.Name)
					continue
				}
				folder := i.Name
				if folder == "" {
					//Regenerating the unique name for this image, that doesnt have a name
					folder = fmt.Sprintf("%x", sha256.Sum256([]byte(i.Image)))[0:6]
				}
				from, to := "", ""
				_, subns, imgName, tag, sha := parseImageName(i.Image)

				from = pkg.Name + "/" + folder
				if sha != "" {
					from = from + "/" + strings.TrimPrefix(sha, "sha256:")
				} else if sha == "" && tag != "" {
					from = from + "/" + fmt.Sprintf("%x", sha256.Sum256([]byte(tag)))[0:6]
				}
				if tag != "" {
					to = strings.Join([]string{destRepo, namespace, subns, imgName}, "/") + ":" + tag
				} else {
					to = strings.Join([]string{destRepo, namespace, subns, imgName}, "/") + "@sha256:" + sha
				}
				srcTIR, err := image.ParseReference("file://" + from)
				if err != nil {
					return err
				}
				if sha != "" && srcTIR.Ref.ID == "" {
					srcTIR.Ref.ID = "sha256:" + sha
				}
				if tag != "" && srcTIR.Ref.Tag == "" {
					srcTIR.Ref.Tag = tag
				}
				srcTI := image.TypedImage{
					TypedImageReference: srcTIR,
					Category:            v1alpha2.TypeOperatorRelatedImage,
				}

				dstTIR, err := image.ParseReference(to)
				if err != nil {
					return err
				}
				if sha != "" && dstTIR.Ref.ID == "" {
					dstTIR.Ref.ID = "sha256:" + sha
				}
				//If there is no tag mirrorMapping is unable to push the image
				//It would push manifests and layers, but image would not appear
				//in registry
				if sha != "" && dstTIR.Ref.Tag == "" {
					dstTIR.Ref.Tag = sha[0:6]
				}
				dstTI := image.TypedImage{
					TypedImageReference: dstTIR,
					Category:            v1alpha2.TypeOperatorRelatedImage,
				}
				mapping[srcTI] = dstTI
			}

		}
		to := strings.Join([]string{"docker://" + destRepo, namespace}, "/")
		klog.Infof("Pushing catalog %s to %s \n", operator.Catalog, to)

		if operator.TargetName != "" {
			to = strings.Join([]string{to, operator.TargetName}, "/")
		} else {
			to = strings.Join([]string{to, repo}, "/")
		}
		if operator.TargetTag != "" {
			to += ":" + operator.TargetTag
		}
		err = pushImage(operator.Catalog, to, o.DestSkipTLS, o.OCIInsecureSignaturePolicy, remoteRegFuncs)
		if err != nil {
			return err
		}
	}

	err := remoteRegFuncs.mirrorMappings(*isc, mapping, o.DestSkipTLS)
	if err != nil {
		return err
	}

	return nil

}

// findFBCConfig function to find the layer from the catalog
// that has the file based configuration
func (o *MirrorOptions) findFBCConfig(imagePath, catalogContentsPath string) (string, error) {
	// read the index.json of the catalog
	srcImg, err := getOCIImgSrcFromPath(context.TODO(), imagePath)
	if err != nil {
		return "", err
	}
	manifest, err := getManifest(context.TODO(), srcImg)
	if err != nil {
		return "", err
	}

	//Use the label in the config layer to determine the
	//folder containing the related images, when untarring layers
	cfgDirName, err := getConfigPathFromConfigLayer(imagePath, string(manifest.ConfigInfo().Digest))
	if err != nil {
		return "", err
	}
	// iterate through each layer

	for _, layer := range manifest.LayerInfos() {
		layerSha := layer.Digest.String()
		layerDirName := layerSha[7:]
		r, err := os.Open(imagePath + blobsPath + layerDirName)
		if err != nil {
			return "", err
		}
		// untar if it is the FBC
		err = UntarLayers(r, catalogContentsPath, cfgDirName)
		if err != nil {
			return "", err
		}
	}
	cfgContentsPath := filepath.Join(catalogContentsPath, cfgDirName)
	f, err := os.Open(cfgContentsPath)
	if err != nil {
		return "", fmt.Errorf("unable to open temp folder containing extracted catalogs %s: %w", cfgContentsPath, err)
	}
	contents, err := f.Readdir(0)
	if err != nil {
		return "", fmt.Errorf("unable to read temp folder containing extracted catalogs %s: %w", cfgContentsPath, err)
	}
	if len(contents) == 0 {
		return "", fmt.Errorf("no packages found in catalog")
	}
	return cfgContentsPath, nil
}

// getCatalogConfigPath takes an OCI FBC image as an input,
// it reads the manifest, then the config layer,
// more specifically the label `configLabel`
// and returns the value of that label
// The function fails if more than one manifest exist in the image
func (o *MirrorOptions) getCatalogConfigPath(imagePath string) (string, error) {
	// read the index.json of the catalog
	srcImg, err := getOCIImgSrcFromPath(context.TODO(), imagePath)
	if err != nil {
		return "", err
	}
	manifest, err := getManifest(context.TODO(), srcImg)
	if err != nil {
		return "", err
	}

	//Use the label in the config layer to determine the
	//folder containing the related images, when untarring layers
	cfgDirName, err := getConfigPathFromConfigLayer(imagePath, string(manifest.ConfigInfo().Digest))
	if err != nil {
		return "", err
	}
	return cfgDirName, nil
}

func getConfigPathFromConfigLayer(imagePath, configSha string) (string, error) {
	var cfg *manifest.Schema2V1Image
	configLayerDir := configSha[7:]
	cfgBlob, err := ioutil.ReadFile(filepath.Join(imagePath, blobsPath, configLayerDir))
	if err != nil {
		return "", fmt.Errorf("unable to read the config blob %s from the oci image: %w", configLayerDir, err)
	}
	err = json.Unmarshal(cfgBlob, &cfg)
	if err != nil {
		return "", fmt.Errorf("problem unmarshaling config blob in %s: %w", configLayerDir, err)
	}
	if dirName, ok := cfg.Config.Labels[configsLabel]; ok {
		return dirName, nil
	}
	return "", fmt.Errorf("label %s not found in config blob %s", configsLabel, configLayerDir)
}

func getRelatedImages(directory string, packages []v1alpha2.IncludePackage) ([]declcfg.RelatedImage, error) {
	allImages := []declcfg.RelatedImage{}
	// load the declarative config from the provided directory (if possible)
	cfg, err := declcfg.LoadFS(os.DirFS(directory))
	if err != nil {
		return nil, err
	}

	for _, bundle := range cfg.Bundles {
		isSelected, err := isPackageSelected(bundle, cfg.Channels, packages)
		if err != nil {
			return nil, err
		}
		if isSelected {
			allImages = append(allImages, declcfg.RelatedImage{Name: bundle.Package, Image: bundle.Image})
			allImages = append(allImages, bundle.RelatedImages...)
		}
	}
	//make sure there are no duplicates in the list with same image:
	finalList := []declcfg.RelatedImage{}
	for _, i := range allImages {
		found := false
		for _, j := range finalList {
			if i.Image == j.Image {
				found = true
				break
			}
		}
		if !found {
			finalList = append(finalList, i)
		}
	}
	return finalList, nil
}

func isPackageSelected(bundle declcfg.Bundle, channels []declcfg.Channel, packages []v1alpha2.IncludePackage) (bool, error) {
	isSelected := false
	for _, pkg := range packages {
		if pkg.Name == bundle.Package {
			var min, max semver.Version
			if pkg.MinVersion != "" || pkg.MaxVersion != "" {
				version_string, err := bundleVersion(bundle)
				if err != nil {
					return isSelected, err
				}
				pkgVer, err := semver.Make(version_string)
				if err != nil {
					return isSelected, err
				}
				if err != nil {
					return isSelected, err
				}
				if pkg.MinVersion != "" {
					min, err = semver.Make(pkg.MinVersion)
					if err != nil {
						return isSelected, err
					}
				}
				if pkg.MaxVersion != "" {
					max, err = semver.Make(pkg.MaxVersion)
					if err != nil {
						return isSelected, err
					}
				}

				if (pkg.MinVersion != "" && pkg.MaxVersion != "") && pkgVer.Compare(min) >= 0 && pkgVer.Compare(max) <= 0 {
					isSelected = true
				} else if pkg.MinVersion != "" && pkg.MaxVersion == "" && pkgVer.Compare(min) >= 0 {
					isSelected = true
				} else if pkg.MaxVersion != "" && pkg.MinVersion == "" && pkgVer.Compare(max) <= 0 {
					isSelected = true
				}

			} else { // no filtering required
				isSelected = true
			}
		}
	}
	return isSelected, nil
}

func bundleVersion(bundle declcfg.Bundle) (string, error) {
	for _, prop := range bundle.Properties {
		if prop.Type == property.TypePackage {
			var p property.Package
			if err := json.Unmarshal(prop.Value, &p); err != nil {
				return "", err
			}
			return p.Version, nil
		}
	}
	return "", fmt.Errorf("unable to find bundle version")
}

// getManifest reads the manifest of the OCI FBC image
// and returns it as a go structure of type manifest.Manifest
func getManifest(ctx context.Context, imgSrc types.ImageSource) (manifest.Manifest, error) {
	manifestBlob, manifestType, err := imgSrc.GetManifest(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to get manifest blob from image : %w", err)
	}
	manifest, err := manifest.FromBlob(manifestBlob, manifestType)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshall manifest of image : %w", err)
	}
	return manifest, nil
}

// getOCIImgSrcFromPath tries to "load" the OCI FBC image in the path
// for further processing.
// It supports path strings with or without the protocol (oci:) prefix
func getOCIImgSrcFromPath(ctx context.Context, path string) (types.ImageSource, error) {
	if !strings.HasPrefix(path, "oci") {
		path = ociProtocol + path
	}
	ociImgRef, err := alltransports.ParseImageName(path)
	if err != nil {
		return nil, err
	}
	imgsrc, err := ociImgRef.NewImageSource(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to get OCI Image from %s: %w", path, err)
	}
	return imgsrc, nil
}

// parseImageName returns the registry, organisation, repository, tag and digest
// from the imageName.
// It can handle both remote and local images.
func parseImageName(imageName string) (string, string, string, string, string) {
	registry, org, repo, tag, sha := "", "", "", "", ""
	imageName = trimProtocol(imageName)
	imageName = strings.TrimPrefix(imageName, "/")
	imageName = strings.TrimSuffix(imageName, "/")
	tmp := strings.Split(imageName, "/")

	registry = tmp[0]
	img := strings.Split(tmp[len(tmp)-1], ":")
	if len(tmp) > 2 {
		org = strings.Join(tmp[1:len(tmp)-1], "/")
	}
	if len(img) > 1 {
		if strings.Contains(img[0], "@") {
			nm := strings.Split(img[0], "@")
			repo = nm[0]
			sha = img[1]
		} else {
			repo = img[0]
			tag = img[1]
		}
	} else {
		repo = img[0]
	}

	return registry, org, repo, tag, sha
}

// trimProtocol removes oci://, file:// or docker:// from
// the parameter imageName
func trimProtocol(imageName string) string {
	imageName = strings.TrimPrefix(imageName, "oci:")
	imageName = strings.TrimPrefix(imageName, "file:")
	imageName = strings.TrimPrefix(imageName, "docker:")
	imageName = strings.TrimPrefix(imageName, "//")

	return imageName
}

// UntarLayers simple function that untars the layer that
// has the FB configuration
func UntarLayers(gzipStream io.Reader, path string, cfgDirName string) error {
	//Remove any separators in cfgDirName as received from the label
	cfgDirName = strings.TrimSuffix(cfgDirName, "/")
	cfgDirName = strings.TrimPrefix(cfgDirName, "/")
	uncompressedStream, err := gzip.NewReader(gzipStream)
	if err != nil {
		return fmt.Errorf("UntarLayers: NewReader failed - %w", err)
	}

	tarReader := tar.NewReader(uncompressedStream)
	for {
		header, err := tarReader.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return fmt.Errorf("UntarLayers: Next() failed: %s", err.Error())
		}

		if strings.Contains(header.Name, cfgDirName) {
			switch header.Typeflag {
			case tar.TypeDir:
				if header.Name != "./" {
					if err := os.MkdirAll(path+"/"+header.Name, 0755); err != nil {
						return fmt.Errorf("UntarLayers: Mkdir() failed: %v", err)
					}
				}
			case tar.TypeReg:
				outFile, err := os.Create(path + "/" + header.Name)
				if err != nil {
					return fmt.Errorf("UntarLayers: Create() failed: %v", err)
				}
				if _, err := io.Copy(outFile, tarReader); err != nil {
					return fmt.Errorf("UntarLayers: Copy() failed: %v", err)
				}
				outFile.Close()

			default:
				// just ignore errors as we are only interested in the FB configs layer
				klog.Warningf("UntarLayers: unknown type: %v in %s", header.Typeflag, header.Name)
			}
		}
	}
	return nil
}

// pullImage uses crane (and NOT containers/image) to pull images
// from the remote registry.
// Crane preserves the image format (OCI or docker.v2).
// This method supports 2 options:
// * pull as OCI image (used for catalogs ONLY)
// * pull as is (used for all related images)
func pullImage(from, to string, srcSkipTLS bool, inStyle style, funcs RemoteRegFuncs) error {
	ctx := context.Background()
	opts := []crane.Option{
		crane.WithAuthFromKeychain(authn.DefaultKeychain),
		crane.WithContext(ctx),
		crane.WithTransport(createRT(srcSkipTLS)),
	}
	if srcSkipTLS {
		opts = append(opts, crane.Insecure)
	}
	img, err := funcs.pull(from, opts...)
	if err != nil {
		return err
	}
	if inStyle == ociStyle {
		err = funcs.saveOCI(img, to)
		if err != nil {
			return fmt.Errorf("unable to save image %s in OCI format in %s: %w", from, to, err)
		}
	} else {
		tags, err := image.GetTagsFromImage(from)
		tag := "latest"
		if err == nil || len(tags) > 0 {
			tag = tags[0]
		}
		if err := funcs.saveLegacy(img, tag, to); err != nil {
			return fmt.Errorf("unable to save image %s in its original format in %s: %w", from, to, err)
		}
	}
	return nil
}

// pushImage is here used only to push images to the remote registry
// calls the underlying containers/image copy library
func pushImage(from, to string, dstSkipTLS bool, insecurePolicy bool, funcs RemoteRegFuncs) error {

	// find absolute path if from is a relative path
	fromPath := trimProtocol(from)
	if !strings.HasPrefix(fromPath, "/") {
		absolutePath, err := filepath.Abs(fromPath)
		if err != nil {
			return fmt.Errorf("unable to get absolute path of oci image %s: %v", from, err)
		}
		from = "oci://" + absolutePath
	}
	sourceCtx := newSystemContext(dstSkipTLS)
	destinationCtx := newSystemContext(dstSkipTLS)
	ctx := context.Background()

	// Pull the source image, and store it in the local storage, under the name main
	var sigPolicy *signature.Policy
	var err error
	if insecurePolicy {
		sigPolicy = &signature.Policy{Default: []signature.PolicyRequirement{signature.NewPRInsecureAcceptAnything()}}
	} else {
		sigPolicy, err = signature.DefaultPolicy(nil)
		if err != nil {
			return err
		}
	}
	policyContext, err := signature.NewPolicyContext(sigPolicy)
	if err != nil {
		return err
	}
	// define the source context
	srcRef, err := alltransports.ParseImageName(from)
	if err != nil {
		return err
	}
	// define the destination context
	destRef, err := alltransports.ParseImageName(to)
	if err != nil {
		return err
	}

	// call the copy.Image function with the set options
	_, err = funcs.push(ctx, policyContext, destRef, srcRef, &imagecopy.Options{
		RemoveSignatures:      true,
		SignBy:                "",
		ReportWriter:          os.Stdout,
		SourceCtx:             sourceCtx,
		DestinationCtx:        destinationCtx,
		ForceManifestMIMEType: "",
		ImageListSelection:    imagecopy.CopySystemImage,
		OciDecryptConfig:      nil,
		OciEncryptLayers:      nil,
		OciEncryptConfig:      nil,
	})
	if err != nil {
		return err
	}
	return nil
}

// newSystemContext set the context for source & destination resources
func newSystemContext(skipTLS bool) *types.SystemContext {
	skipTLSVerify := types.OptionalBoolFalse
	if skipTLS {
		skipTLSVerify = types.OptionalBoolTrue
	}
	ctx := &types.SystemContext{
		RegistriesDirPath:           "",
		ArchitectureChoice:          "",
		OSChoice:                    "",
		VariantChoice:               "",
		BigFilesTemporaryDir:        "", //*globalArgs.cache + "/tmp",
		DockerInsecureSkipTLSVerify: skipTLSVerify,
	}
	return ctx
}
