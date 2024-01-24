package archive

import (
	"context"
	"fmt"
	"strconv"

	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

type ImageBlobGatherer struct {
	BlobsGatherer
	opts *mirror.CopyOptions
}

func NewImageBlobGatherer(opts *mirror.CopyOptions) BlobsGatherer {
	return &ImageBlobGatherer{
		opts: opts,
	}
}

func (o *ImageBlobGatherer) GatherBlobs(ctx context.Context, imgRef string) (blobs map[string]string, retErr error) {
	blobs = map[string]string{}
	o.opts.DeprecatedTLSVerify.WarnIfUsed([]string{"--src-tls-verify", "--dest-tls-verify"})
	// o.opts.All = true
	o.opts.RemoveSignatures, _ = strconv.ParseBool("true")

	if err := mirror.ReexecIfNecessaryForImages([]string{imgRef}...); err != nil {
		return blobs, err
	}

	// More broadly, should we include anything in the archive to
	// help with signature verification after the archive is untarred
	// inside the enclave?

	srcRef, err := alltransports.ParseImageName(imgRef)
	if err != nil {
		return nil, fmt.Errorf("invalid source name %s: %v", imgRef, err)
	}
	sourceCtx, err := o.opts.SrcImage.NewSystemContext()
	if err != nil {
		return nil, err
	}

	img, err := srcRef.NewImageSource(ctx, sourceCtx)
	if err != nil {
		return nil, err
	}

	manifestBytes, mime, err := img.GetManifest(ctx, nil)
	if err != nil {
		return nil, err
	}

	digest, err := manifest.Digest(manifestBytes)
	if err != nil {
		return nil, err
	}
	blobs[digest.String()] = ""

	// it seems this code never gets called
	// the manifest list gets moved to sha256 hash with .manifest.json
	// i.e ad59206271872ca50d4b202c8298018c53bfaefd03b64ee036ad5362fc0214ce.manifest.json
	/*
		if manifest.MIMETypeIsMultiImage(mime) {
			manifestList, err := manifest.ListFromBlob(manifestBytes, mime)
			if err != nil {
				return nil, err
			}
			instances := manifestList.Instances()
			for _, digest := range instances {
				blobs[digest.String()] = ""
				singleArchManifest, singleArchMime, err := img.GetManifest(ctx, &digest)
				if err != nil {
					return nil, err
				}
				singleArchBlobs, err := o.getBlobsOfManifest(singleArchManifest, singleArchMime)
				if err != nil {
					return nil, err
				}
				for _, digest := range singleArchBlobs {
					blobs[digest] = ""
				}
				if err != nil {
					return nil, err
				}
			}
		} else { */

	manifestBlobs, err := o.getBlobsOfManifest(manifestBytes, mime)
	if err != nil {
		return nil, err
	}
	for _, digest := range manifestBlobs {
		blobs[digest] = ""
	}
	//}
	return blobs, nil
}

func (o *ImageBlobGatherer) getBlobsOfManifest(manifestBytes []byte, mimeType string) ([]string, error) {
	blobs := []string{}
	singleArchManifest, err := manifest.FromBlob(manifestBytes, mimeType)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling manifest: %v", err)
	}
	for _, layer := range singleArchManifest.LayerInfos() {
		blobs = append(blobs, layer.Digest.String())
	}
	blobs = append(blobs, singleArchManifest.ConfigInfo().Digest.String())
	return blobs, nil
}
