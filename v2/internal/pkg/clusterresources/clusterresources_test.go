package clusterresources

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	confv1 "github.com/openshift/api/config/v1"
	cm "github.com/openshift/oc-mirror/v2/internal/pkg/api/kubernetes/core"
	ofv1alpha1 "github.com/openshift/oc-mirror/v2/internal/pkg/api/operator-framework/v1alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	updateservicev1 "github.com/openshift/oc-mirror/v2/internal/pkg/clusterresources/updateservice/v1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/common"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

var (
	imageListMixed = []v2alpha1.CopyImageSchema{
		{
			Source:      "docker://localhost:5000/kubebuilder/kube-rbac-proxy:v0.5.0",
			Destination: "docker://myregistry/mynamespace/kubebuilder/kube-rbac-proxy:v0.5.0",
			Origin:      "docker://gcr.io/kubebuilder/kube-rbac-proxy:v0.5.0",
			Type:        v2alpha1.TypeOperatorRelatedImage,
		},
		{
			Source:      "docker://localhost:5000/cockroachdb/cockroach-helm-operator:6.0.0",
			Destination: "docker://myregistry/mynamespace/cockroachdb/cockroach-helm-operator:6.0.0",
			Origin:      "docker://quay.io/cockroachdb/cockroach-helm-operator:6.0.0",
			Type:        v2alpha1.TypeOperatorRelatedImage,
		},
		{
			Source:      "docker://localhost:5000/helmoperators/cockroachdb:v5.0.3",
			Destination: "docker://myregistry/mynamespace/helmoperators/cockroachdb:v5.0.3",
			Origin:      "docker://quay.io/helmoperators/cockroachdb:v5.0.3",
			Type:        v2alpha1.TypeOperatorRelatedImage,
		},
		{
			Source:      "docker://localhost:5000/helmoperators/cockroachdb:v5.0.4",
			Destination: "docker://myregistry/mynamespace/helmoperators/cockroachdb:v5.0.4",
			Origin:      "docker://quay.io/helmoperators/cockroachdb:v5.0.4",
			Type:        v2alpha1.TypeOperatorRelatedImage,
		},
		{
			Source:      "docker://localhost:5000/openshift-community-operators/cockroachdb@sha256:a5d4f4467250074216eb1ba1c36e06a3ab797d81c431427fc2aca97ecaf4e9d8",
			Destination: "docker://myregistry/mynamespace/openshift-community-operators/cockroachdb@sha256:a5d4f4467250074216eb1ba1c36e06a3ab797d81c431427fc2aca97ecaf4e9d8",
			Origin:      "docker://quay.io/openshift-community-operators/cockroachdb@sha256:a5d4f4467250074216eb1ba1c36e06a3ab797d81c431427fc2aca97ecaf4e9d8",
			Type:        v2alpha1.TypeOperatorBundle,
		},
		{
			Source:      "docker://localhost:5000/openshift-community-operators/cockroachdb@sha256:d3016b1507515fc7712f9c47fd9082baf9ccb070aaab58ed0ef6e5abdedde8ba",
			Destination: "docker://myregistry/mynamespace/openshift-community-operators/cockroachdb@sha256:d3016b1507515fc7712f9c47fd9082baf9ccb070aaab58ed0ef6e5abdedde8ba",
			Origin:      "docker://quay.io/openshift-community-operators/cockroachdb@sha256:d3016b1507515fc7712f9c47fd9082baf9ccb070aaab58ed0ef6e5abdedde8ba",
			Type:        v2alpha1.TypeOperatorBundle,
		},
		{
			Source:      "docker://localhost:5000/openshift/openshift-community-operators@sha256:f42337e7b85a46d83c94694638e2312e10ca16a03542399a65ba783c94a32b63",
			Destination: "docker://myregistry/mynamespace/openshift/openshift-community-operators@sha256:f42337e7b85a46d83c94694638e2312e10ca16a03542399a65ba783c94a32b63",
			Origin:      "docker://quay.io/openshift/openshift-community-operators@sha256:f42337e7b85a46d83c94694638e2312e10ca16a03542399a65ba783c94a32b63",
			Type:        v2alpha1.TypeOperatorCatalog,
		},
		{
			Source:      "docker://localhost:5000/openshift/redhat-operator-index@sha256:f42337e7b85a46d83c94694638e2312e10ca16a03542399a65ba783c94a32b63",
			Destination: "docker://myregistry/mynamespace/openshift/redhat-operator-index@sha256:f42337e7b85a46d83c94694638e2312e10ca16a03542399a65ba783c94a32b63",
			Origin:      "oci:///tmp/app1",
			Type:        v2alpha1.TypeOperatorCatalog,
		},
		{
			Source:      "docker://localhost:55000/ubi8-minimal:b93deceb59a58588d5b16429fc47f98920f84740a1f2ed6454e33275f0701b59",
			Destination: "docker://myregistry/mynamespace/ubi8-minimal@sha256:b93deceb59a58588d5b16429fc47f98920f84740a1f2ed6454e33275f0701b59",
			Origin:      "docker://registry.redhat.io/ubi8-minimal@sha256:b93deceb59a58588d5b16429fc47f98920f84740a1f2ed6454e33275f0701b59",
			Type:        v2alpha1.TypeOperatorRelatedImage,
		},
		{
			Source:      "docker://localhost:5000/ubi8/ubi:latest",
			Destination: "docker://myregistry/mynamespace/ubi8/ubi:latest",
			Origin:      "docker://registry.redhat.io/ubi8/ubi:latest",
			Type:        v2alpha1.TypeGeneric,
		},
		{
			Source:      "docker://localhost:5000/openshift/graph-image:latest",
			Destination: "docker://myregistry/mynamespace/openshift/graph-image:latest",
			Origin:      "docker://localhost:5000/openshift/graph-image:latest",
			Type:        v2alpha1.TypeCincinnatiGraph,
		},
		{
			Source:      "docker://localhost:5000/openshift-release-dev/ocp-v4.0-art-dev@sha256:6d76ffca7a233213325907bae611e835b49c5b933095be1328351f4f5fc67615",
			Destination: "docker://myregistry/mynamespace/openshift-release-dev/ocp-v4.0-art-dev@sha256:6d76ffca7a233213325907bae611e835b49c5b933095be1328351f4f5fc67615",
			Origin:      "docker://quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:6d76ffca7a233213325907bae611e835b49c5b933095be1328351f4f5fc67615",
			Type:        v2alpha1.TypeOCPRelease,
		},
		{
			Source:      "docker://localhost:5000/openshift-release-dev/ocp-v4.0-art-dev@sha256:4c181f5cbea53472acd9695232f77a0933a73f7f40f543cbd48dff00e6f03090",
			Destination: "docker://myregistry/mynamespace/openshift-release-dev/ocp-v4.0-art-dev@sha256:4c181f5cbea53472acd9695232f77a0933a73f7f40f543cbd48dff00e6f03090",
			Origin:      "docker://quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:4c181f5cbea53472acd9695232f77a0933a73f7f40f543cbd48dff00e6f03090",
			Type:        v2alpha1.TypeOCPReleaseContent,
		},
	}

	imageListDigestsOnly = []v2alpha1.CopyImageSchema{
		{
			Source:      "docker://localhost:5000/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Destination: "docker://myregistry/mynamespace/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Origin:      "docker://quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Type:        v2alpha1.TypeOCPReleaseContent,
		},
		{
			Source:      "docker://localhost:5000/openshift-release-dev/ocp-v4.0-art-dev@sha256:6d76ffca7a233213325907bae611e835b49c5b933095be1328351f4f5fc67615",
			Destination: "docker://myregistry/mynamespace/openshift-release-dev/ocp-v4.0-art-dev@sha256:6d76ffca7a233213325907bae611e835b49c5b933095be1328351f4f5fc67615",
			Origin:      "docker://quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:6d76ffca7a233213325907bae611e835b49c5b933095be1328351f4f5fc67615",
			Type:        v2alpha1.TypeOCPRelease,
		},
		{
			Source:      "docker://localhost:5000/openshift-release-dev/ocp-v4.0-art-dev@sha256:4c181f5cbea53472acd9695232f77a0933a73f7f40f543cbd48dff00e6f03090",
			Destination: "docker://myregistry/mynamespace/openshift-release-dev/ocp-v4.0-art-dev@sha256:4c181f5cbea53472acd9695232f77a0933a73f7f40f543cbd48dff00e6f03090",
			Origin:      "docker://quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:4c181f5cbea53472acd9695232f77a0933a73f7f40f543cbd48dff00e6f03090",
			Type:        v2alpha1.TypeOCPReleaseContent,
		},
		{
			Source:      "docker://localhost:5000/openshift-release-dev/ocp-v4.0-art-dev@sha256:ff8ef167b679606b17baf75d94a02589048849b550c4cc17d36506a28f22b29c",
			Destination: "docker://myregistry/mynamespace/openshift-release-dev/ocp-v4.0-art-dev@sha256:ff8ef167b679606b17baf75d94a02589048849b550c4cc17d36506a28f22b29c",
			Origin:      "docker://quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:ff8ef167b679606b17baf75d94a02589048849b550c4cc17d36506a28f22b29c",
			Type:        v2alpha1.TypeOCPReleaseContent,
		},
		{
			Source:      "docker://localhost:5000/openshift-release-dev/ocp-v4.0-art-dev@sha256:d62f2612d3b9618a04ac0dea3ee2e1dec63d8fbe2279e86aa2a605d8755f2b8f",
			Destination: "docker://myregistry/mynamespace/openshift-release-dev/ocp-v4.0-art-dev@sha256:d62f2612d3b9618a04ac0dea3ee2e1dec63d8fbe2279e86aa2a605d8755f2b8f",
			Origin:      "docker://quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:d62f2612d3b9618a04ac0dea3ee2e1dec63d8fbe2279e86aa2a605d8755f2b8f",
			Type:        v2alpha1.TypeOCPReleaseContent,
		},
		{
			Source:      "docker://localhost:5000/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Destination: "docker://myregistry/mynamespace/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Origin:      "docker://quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Type:        v2alpha1.TypeOCPReleaseContent,
		},
	}
	imageListMaxNestedPaths = []v2alpha1.CopyImageSchema{
		{
			Source:      "docker://localhost:5000/cockroachdb/cockroach-helm-operator:6.0.0",
			Destination: "docker://myregistry/mynamespace/cockroachdb-cockroach-helm-operator:6.0.0",
			Origin:      "docker://quay.io/cockroachdb/cockroach-helm-operator:6.0.0",
			Type:        v2alpha1.TypeOperatorRelatedImage,
		},
	}
)

func TestIDMS_ITMSGenerator(t *testing.T) {
	log := clog.New("trace")

	type testCase struct {
		caseName                     string
		imgList                      []v2alpha1.CopyImageSchema
		expectedNumberFilesGenerated int
		expectedItms                 bool
		expectedIdms                 bool
		expectedError                bool
	}
	testCases := []testCase{
		{
			caseName:                     "Testing IDMS_ITMSGenerator - tags and digests : should generate idms and itms",
			imgList:                      imageListMixed,
			expectedNumberFilesGenerated: 2,
			expectedItms:                 true,
			expectedIdms:                 true,
			expectedError:                false,
		},
		{
			caseName:                     "Testing IDMS_ITMSGenerator - digests only : should generate only idms",
			imgList:                      imageListDigestsOnly,
			expectedNumberFilesGenerated: 1,
			expectedItms:                 false,
			expectedIdms:                 true,
			expectedError:                false,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			tmpDir := t.TempDir()
			workingDir := tmpDir + "/working-dir"

			defer os.RemoveAll(tmpDir)
			cr := &ClusterResourcesGenerator{
				Log:              log,
				WorkingDir:       workingDir,
				LocalStorageFQDN: "localhost:55000",
			}
			err := cr.IDMS_ITMSGenerator(testCase.imgList, false)
			if err != nil {
				t.Fatalf("should not fail")
			}

			_, err = os.Stat(filepath.Join(workingDir, clusterResourcesDir))
			if err != nil {
				t.Fatalf("output folder should exist")
			}

			msFiles, err := os.ReadDir(filepath.Join(workingDir, clusterResourcesDir))
			if err != nil {
				t.Fatalf("ls output folder should not fail")
			}

			if len(msFiles) != testCase.expectedNumberFilesGenerated {
				t.Fatalf("output folder should contain %d files, but found %d", testCase.expectedNumberFilesGenerated, len(msFiles))
			}
			isIdmsFound := false
			isItmsFound := false
			for _, file := range msFiles {
				// check idmsFile has a name that is
				//compliant with Kubernetes requested
				// RFC-1035 + RFC1123
				// https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-label-names
				filename := file.Name()
				customResourceName := strings.TrimSuffix(filename, ".yaml")
				if !isValidRFC1123(customResourceName) {
					t.Fatalf("I*MS custom resource name %s doesn't  respect RFC1123", msFiles[0].Name())
				}
				if filename == "idms-oc-mirror.yaml" {
					isIdmsFound = true
				}
				if filename == "itms-oc-mirror.yaml" {
					isItmsFound = true
				}
			}
			if testCase.expectedIdms && !isIdmsFound {
				t.Fatalf("output folder should contain 1 idms file which was not found")
			}
			if testCase.expectedItms && !isItmsFound {
				t.Fatalf("output folder should contain 1 itms file which was not found")
			}
		})

	}
}

func TestGenerateIDMS(t *testing.T) {
	log := clog.New("trace")

	type testCase struct {
		caseName         string
		imgList          []v2alpha1.CopyImageSchema
		expectedIdmsList []confv1.ImageDigestMirrorSet
		expectedError    bool
	}
	testCases := []testCase{
		{
			caseName: "Testing GenerateIDMS - tags and digests : should pass",
			imgList:  imageListMixed,
			expectedIdmsList: []confv1.ImageDigestMirrorSet{
				{
					TypeMeta:   v1.TypeMeta{Kind: "ImageDigestMirrorSet", APIVersion: "config.openshift.io/v1"},
					ObjectMeta: v1.ObjectMeta{Name: "idms-operator-0"},
					Spec: confv1.ImageDigestMirrorSetSpec{
						ImageDigestMirrors: []confv1.ImageDigestMirrors{
							{
								Source:  "quay.io/openshift-community-operators",
								Mirrors: []confv1.ImageMirror{"myregistry/mynamespace/openshift-community-operators"},
							},
							{
								Source:  "registry.redhat.io",
								Mirrors: []confv1.ImageMirror{"myregistry/mynamespace"},
							},
						},
					},
				},
				{
					TypeMeta:   v1.TypeMeta{Kind: "ImageDigestMirrorSet", APIVersion: "config.openshift.io/v1"},
					ObjectMeta: v1.ObjectMeta{Name: "idms-release-0"},
					Spec: confv1.ImageDigestMirrorSetSpec{
						ImageDigestMirrors: []confv1.ImageDigestMirrors{
							{
								Source:  "quay.io/openshift-release-dev",
								Mirrors: []confv1.ImageMirror{"myregistry/mynamespace/openshift-release-dev"},
							},
						},
					},
				},
			},
			expectedError: false,
		},
		{
			caseName: "Testing GemerateIDMS - digests only : should pass",
			imgList:  imageListDigestsOnly,
			expectedIdmsList: []confv1.ImageDigestMirrorSet{
				{
					TypeMeta:   v1.TypeMeta{Kind: "ImageDigestMirrorSet", APIVersion: "config.openshift.io/v1"},
					ObjectMeta: v1.ObjectMeta{Name: "idms-release-0"},
					Spec: confv1.ImageDigestMirrorSetSpec{
						ImageDigestMirrors: []confv1.ImageDigestMirrors{
							{
								Source:  "quay.io/openshift-release-dev",
								Mirrors: []confv1.ImageMirror{"myregistry/mynamespace/openshift-release-dev"},
							},
						},
					},
				},
			},
			expectedError: false,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			tmpDir := t.TempDir()
			workingDir := tmpDir + "/working-dir"

			defer os.RemoveAll(tmpDir)
			cr := &ClusterResourcesGenerator{
				Log:              log,
				WorkingDir:       workingDir,
				LocalStorageFQDN: "localhost:55000",
			}
			idmsList, err := cr.generateImageMirrors(testCase.imgList, DigestsOnlyMode, false)
			if err != nil {
				t.Fatalf("should not fail")
			}
			actualIdmses, err := cr.generateIDMS(idmsList)
			if err != nil {
				t.Fatalf("should not fail")
			}
			assert.Equal(t, len(testCase.expectedIdmsList), len(actualIdmses))
			for _, expectedIdms := range testCase.expectedIdmsList {
				isFound := false
				for _, actualIdms := range actualIdmses {
					if expectedIdms.Name == actualIdms.Name {
						isFound = true
						assert.ElementsMatch(t, expectedIdms.Spec.ImageDigestMirrors, actualIdms.Spec.ImageDigestMirrors)
						break
					}
				}
				if !isFound {
					t.Fatalf("list of IDMS resources should contain %s but it was not found", expectedIdms.Name)
				}
			}
		})
	}
}

func TestGenerateITMS(t *testing.T) {
	log := clog.New("trace")

	type testCase struct {
		caseName         string
		imgList          []v2alpha1.CopyImageSchema
		expectedItmsList []confv1.ImageTagMirrorSet
		expectedError    bool
	}
	testCases := []testCase{
		{
			caseName: "Testing GenerateITMS - tags and digests : should pass",
			imgList:  imageListMixed,
			expectedItmsList: []confv1.ImageTagMirrorSet{
				{
					TypeMeta:   v1.TypeMeta{Kind: "ImageTagMirrorSet", APIVersion: "config.openshift.io/v1"},
					ObjectMeta: v1.ObjectMeta{Name: "itms-operator-0"},
					Spec: confv1.ImageTagMirrorSetSpec{
						ImageTagMirrors: []confv1.ImageTagMirrors{
							{
								Source:  "gcr.io/kubebuilder",
								Mirrors: []confv1.ImageMirror{"myregistry/mynamespace/kubebuilder"},
							},
							{
								Source:  "quay.io/cockroachdb",
								Mirrors: []confv1.ImageMirror{"myregistry/mynamespace/cockroachdb"},
							},
							{
								Source:  "quay.io/helmoperators",
								Mirrors: []confv1.ImageMirror{"myregistry/mynamespace/helmoperators"},
							},
						},
					},
				},
				{
					TypeMeta:   v1.TypeMeta{Kind: "ImageTagMirrorSet", APIVersion: "config.openshift.io/v1"},
					ObjectMeta: v1.ObjectMeta{Name: "itms-generic-0"},
					Spec: confv1.ImageTagMirrorSetSpec{
						ImageTagMirrors: []confv1.ImageTagMirrors{
							{
								Source:  "registry.redhat.io/ubi8",
								Mirrors: []confv1.ImageMirror{"myregistry/mynamespace/ubi8"},
							},
						},
					},
				},
			},
			expectedError: false,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			tmpDir := t.TempDir()
			workingDir := tmpDir + "/working-dir"

			defer os.RemoveAll(tmpDir)
			cr := &ClusterResourcesGenerator{
				Log:              log,
				WorkingDir:       workingDir,
				LocalStorageFQDN: "localhost:55000",
			}
			itmsList, err := cr.generateImageMirrors(testCase.imgList, TagsOnlyMode, false)
			if err != nil {
				t.Fatalf("should not fail")
			}
			actualItmses, err := cr.generateITMS(itmsList)
			if err != nil {
				t.Fatalf("should not fail")
			}
			assert.Equal(t, len(testCase.expectedItmsList), len(actualItmses))
			for _, expectedItms := range testCase.expectedItmsList {
				isFound := false
				for _, actualItms := range actualItmses {
					if expectedItms.Name == actualItms.Name {
						isFound = true
						assert.ElementsMatch(t, expectedItms.Spec.ImageTagMirrors, actualItms.Spec.ImageTagMirrors)
						break
					}
				}
				if !isFound {
					t.Fatalf("list of IDMS resources should contain %s but it was not found", expectedItms.Name)
				}
			}
		})
	}
}

func TestCatalogSourceGenerator(t *testing.T) {
	log := clog.New("trace")

	tmpDir := t.TempDir()
	workingDir := tmpDir + "/working-dir"

	defer os.RemoveAll(tmpDir)
	imageList := []v2alpha1.CopyImageSchema{
		{
			Source:      "docker://localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Destination: "docker://myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Origin:      "docker://quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Type:        v2alpha1.TypeOCPRelease,
		},
		{
			Source:      "docker://localhost:5000/redhat/redhat-operator-index:v4.15",
			Destination: "docker://myregistry/mynamespace/redhat/redhat-operator-index:v4.15",
			Origin:      "docker://registry.redhat.io/redhat/redhat-operator-index:v4.15",
			Type:        v2alpha1.TypeOperatorCatalog,
		},
		{ // OCPBUGS-41608 - this should be skipped because it mirrors to the cache
			Source:      "docker://localhost:5000/redhat/redhat-operator-index:v4.15",
			Destination: "docker://localhost:55000/redhat/redhat-operator-index:v4.15",
			Origin:      "docker://registry.redhat.io/redhat/redhat-operator-index:v4.15",
			Type:        v2alpha1.TypeOperatorCatalog,
		},
		{
			Source:      "docker://localhost:5000/kubebuilder/kube-rbac-proxy:v0.5.0",
			Destination: "docker://myregistry/mynamespace/kubebuilder/kube-rbac-proxy:v0.5.0",
			Origin:      "docker://gcr.io/kubebuilder/kube-rbac-proxy:v0.5.0",
			Type:        v2alpha1.TypeOperatorRelatedImage,
		},
		{
			Source:      "docker://localhost:5000/openshift-community-operators/cockroachdb@sha256:f42337e7b85a46d83c94694638e2312e10ca16a03542399a65ba783c94a32b63",
			Destination: "docker://myregistry/mynamespace/openshift-community-operators/cockroachdb@sha256:f42337e7b85a46d83c94694638e2312e10ca16a03542399a65ba783c94a32b63",
			Origin:      "docker://quay.io/openshift-community-operators/cockroachdb@sha256:f42337e7b85a46d83c94694638e2312e10ca16a03542399a65ba783c94a32b63",
			Type:        v2alpha1.TypeOperatorBundle,
		},
	}

	t.Run("Testing GenerateCatalogSource : should pass", func(t *testing.T) {

		cr := &ClusterResourcesGenerator{
			Log:              log,
			WorkingDir:       workingDir,
			LocalStorageFQDN: "localhost:55000",
			Config: v2alpha1.ImageSetConfiguration{
				ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
					Mirror: v2alpha1.Mirror{
						Operators: []v2alpha1.Operator{
							{
								Catalog: "registry.redhat.io/redhat/redhat-operator-index:v4.15",
							},
						},
					},
				},
			},
		}
		err := cr.CatalogSourceGenerator(imageList)
		if err != nil {
			t.Fatalf("should not fail")
		}
		_, err = os.Stat(filepath.Join(workingDir, clusterResourcesDir))
		if err != nil {
			t.Fatalf("output folder should exist")
		}

		csFiles, err := os.ReadDir(filepath.Join(workingDir, clusterResourcesDir))
		if err != nil {
			t.Fatalf("ls output folder should not fail")
		}

		if len(csFiles) != 1 {
			t.Fatalf("output folder should contain 1 catalogsource yaml file")
		}

		expectedCSName := "cs-redhat-operator-index-v4-15"
		// check idmsFile has a name that is
		//compliant with Kubernetes requested
		// RFC-1035 + RFC1123
		// https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-label-names
		customResourceName := strings.TrimSuffix(csFiles[0].Name(), ".yaml")
		if !isValidRFC1123(customResourceName) {
			t.Fatalf("CatalogSource custom resource name %s doesn't  respect RFC1123", csFiles[0].Name())
		}
		assert.Equal(t, expectedCSName, customResourceName)
		bytes, err := os.ReadFile(filepath.Join(workingDir, clusterResourcesDir, csFiles[0].Name()))
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}
		var actualCS ofv1alpha1.CatalogSource
		err = yaml.Unmarshal(bytes, &actualCS)
		if err != nil {
			t.Fatalf("failed to unmarshal catalogsource: %v", err)
		}
		expectedCS := ofv1alpha1.CatalogSource{
			TypeMeta: metav1.TypeMeta{
				APIVersion: ofv1alpha1.GroupName + "/" + ofv1alpha1.GroupVersion,
				Kind:       "CatalogSource",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      expectedCSName,
				Namespace: "openshift-marketplace",
			},
			Spec: ofv1alpha1.CatalogSourceSpec{
				SourceType: "grpc",
				Image:      "myregistry/mynamespace/redhat/redhat-operator-index:v4.15",
			},
		}

		assert.Equal(t, expectedCS, actualCS, "contents of catalogSource file incorrect")

	})

	t.Run("Testing GenerateCatalogSource with template: should pass", func(t *testing.T) {

		cr := &ClusterResourcesGenerator{
			Log:              log,
			WorkingDir:       workingDir,
			LocalStorageFQDN: "localhost:55000",
			Config: v2alpha1.ImageSetConfiguration{
				ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
					Mirror: v2alpha1.Mirror{
						Operators: []v2alpha1.Operator{
							{
								Catalog:                     "registry.redhat.io/redhat/redhat-operator-index:v4.15",
								TargetCatalogSourceTemplate: common.TestFolder + "catalog-source_template.yaml",
							},
						},
					},
				},
			},
		}
		err := cr.CatalogSourceGenerator(imageList)
		if err != nil {
			t.Fatalf("should not fail")
		}
		_, err = os.Stat(filepath.Join(workingDir, clusterResourcesDir))
		if err != nil {
			t.Fatalf("output folder should exist")
		}

		csFiles, err := os.ReadDir(filepath.Join(workingDir, clusterResourcesDir))
		if err != nil {
			t.Fatalf("ls output folder should not fail")
		}

		if len(csFiles) != 1 {
			t.Fatalf("output folder should contain 1 catalogSource yaml file")
		}
		// check idmsFile has a name that is
		//compliant with Kubernetes requested
		// RFC-1035 + RFC1123
		// https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-label-names
		customResourceName := strings.TrimSuffix(csFiles[0].Name(), ".yaml")
		if !isValidRFC1123(customResourceName) {
			t.Fatalf("CatalogSource custom resource name %s doesn't  respect RFC1123", csFiles[0].Name())
		}
		bytes, err := os.ReadFile(filepath.Join(workingDir, clusterResourcesDir, csFiles[0].Name()))
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}
		var actualCS ofv1alpha1.CatalogSource
		err = yaml.Unmarshal(bytes, &actualCS)
		if err != nil {
			t.Fatalf("failed to unmarshal catalogsource: %v", err)
		}
		expectedCS := ofv1alpha1.CatalogSource{
			TypeMeta: metav1.TypeMeta{
				APIVersion: ofv1alpha1.GroupName + "/" + ofv1alpha1.GroupVersion,
				Kind:       "CatalogSource",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      strings.TrimSuffix(csFiles[0].Name(), ".yaml"),
				Namespace: "openshift-marketplace",
			},
			Spec: ofv1alpha1.CatalogSourceSpec{
				SourceType: "grpc",
				Image:      "myregistry/mynamespace/redhat/redhat-operator-index:v4.15",
				UpdateStrategy: &ofv1alpha1.UpdateStrategy{
					RegistryPoll: &ofv1alpha1.RegistryPoll{
						RawInterval: "30m0s",
						Interval: &metav1.Duration{
							Duration: time.Minute * 30,
						},

						ParsingError: "",
					},
				},
			},
		}

		assert.Equal(t, expectedCS, actualCS, "contents of catalogSource file incorrect")

	})

	templateFailCases := []ClusterResourcesGenerator{
		{
			Log:              log,
			WorkingDir:       workingDir,
			LocalStorageFQDN: "localhost:55000",
			Config: v2alpha1.ImageSetConfiguration{
				ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
					Mirror: v2alpha1.Mirror{
						Operators: []v2alpha1.Operator{
							{
								Catalog:                     "registry.redhat.io/redhat/redhat-operator-index:v4.15",
								TargetCatalogSourceTemplate: common.TestFolder + "catalog-source_template_KO.yaml",
							},
						},
					},
				},
			},
		},
		{
			Log:              log,
			WorkingDir:       workingDir,
			LocalStorageFQDN: "localhost:55000",
			Config: v2alpha1.ImageSetConfiguration{
				ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
					Mirror: v2alpha1.Mirror{
						Operators: []v2alpha1.Operator{
							{
								Catalog:                     "registry.redhat.io/redhat/redhat-operator-index:v4.15",
								TargetCatalogSourceTemplate: "doesnt_exist.yaml",
							},
						},
					},
				},
			},
		},
		{
			Log:        log,
			WorkingDir: workingDir,
			Config: v2alpha1.ImageSetConfiguration{
				ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
					Mirror: v2alpha1.Mirror{
						Operators: []v2alpha1.Operator{
							{
								Catalog:                     "registry.redhat.io/redhat/redhat-operator-index:v4.15",
								TargetCatalogSourceTemplate: common.TestFolder + "catalog-source_template_KO2.yaml",
							},
						},
					},
				},
			},
		},
	}
	t.Run("Testing GenerateCatalogSource with KO template: should not fail", func(t *testing.T) {

		for _, tc := range templateFailCases {
			err := tc.CatalogSourceGenerator(imageList)
			if err != nil {
				t.Fatalf("should not fail")
			}
			_, err = os.Stat(filepath.Join(workingDir, clusterResourcesDir))
			if err != nil {
				t.Fatalf("output folder should exist")
			}

			csFiles, err := os.ReadDir(filepath.Join(workingDir, clusterResourcesDir))
			if err != nil {
				t.Fatalf("ls output folder should not fail")
			}

			if len(csFiles) != 1 {
				t.Fatalf("output folder should contain 1 catalogSource yaml file")
			}
			// check idmsFile has a name that is
			//compliant with Kubernetes requested
			// RFC-1035 + RFC1123
			// https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-label-names
			customResourceName := strings.TrimSuffix(csFiles[0].Name(), ".yaml")
			if !isValidRFC1123(customResourceName) {
				t.Fatalf("CatalogSource custom resource name %s doesn't  respect RFC1123", csFiles[0].Name())
			}
			bytes, err := os.ReadFile(filepath.Join(workingDir, clusterResourcesDir, csFiles[0].Name()))
			if err != nil {
				t.Fatalf("failed to read file: %v", err)
			}
			var actualCS ofv1alpha1.CatalogSource
			err = yaml.Unmarshal(bytes, &actualCS)
			if err != nil {
				t.Fatalf("failed to unmarshal catalogsource: %v", err)
			}
			expectedCS := ofv1alpha1.CatalogSource{
				TypeMeta: metav1.TypeMeta{
					APIVersion: ofv1alpha1.GroupName + "/" + ofv1alpha1.GroupVersion,
					Kind:       "CatalogSource",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      strings.TrimSuffix(csFiles[0].Name(), ".yaml"),
					Namespace: "openshift-marketplace",
				},
				Spec: ofv1alpha1.CatalogSourceSpec{
					SourceType: "grpc",
					Image:      "myregistry/mynamespace/redhat/redhat-operator-index:v4.15",
				},
			}

			assert.Equal(t, expectedCS, actualCS, "contents of catalogSource file incorrect")

		}
	})
	t.Run("Testing GenerateCatalogSource with catalog using a digest as tag : should pass", func(t *testing.T) {
		tmpDir := t.TempDir()
		workingDir := tmpDir + "/working-dir"

		defer os.RemoveAll(tmpDir)
		listCatalogDigestAsTag := []v2alpha1.CopyImageSchema{

			{
				Source:      "docker://localhost:5000/redhat/redhat-operator-index:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
				Destination: "docker://myregistry/mynamespace/redhat/redhat-operator-index:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
				Origin:      "docker://registry.redhat.io/redhat/redhat-operator-index@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
				Type:        v2alpha1.TypeOperatorCatalog,
			},
		}
		cr := &ClusterResourcesGenerator{
			Log:              log,
			WorkingDir:       workingDir,
			LocalStorageFQDN: "localhost:55000",
			Config: v2alpha1.ImageSetConfiguration{
				ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
					Mirror: v2alpha1.Mirror{
						Operators: []v2alpha1.Operator{
							{
								Catalog: "registry.redhat.io/redhat/redhat-operator-index@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
							},
						},
					},
				},
			},
		}
		err := cr.CatalogSourceGenerator(listCatalogDigestAsTag)
		if err != nil {
			t.Fatalf("should not fail")
		}
		_, err = os.Stat(filepath.Join(workingDir, clusterResourcesDir))
		if err != nil {
			t.Fatalf("output folder should exist")
		}

		csFiles, err := os.ReadDir(filepath.Join(workingDir, clusterResourcesDir))
		if err != nil {
			t.Fatalf("ls output folder should not fail")
		}

		if len(csFiles) != 1 {
			t.Fatalf("output folder should contain 1 catalogsource yaml file")
		}

		expectedCSName := "cs-redhat-operator-index-7c4ef7434c97"
		// check catalogsource has a name that is
		//compliant with Kubernetes requested
		// RFC-1035 + RFC1123
		// https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-label-names
		customResourceName := strings.TrimSuffix(csFiles[0].Name(), ".yaml")
		if !isValidRFC1123(customResourceName) {
			t.Fatalf("CatalogSource custom resource name %s doesn't  respect RFC1123", csFiles[0].Name())
		}
		assert.Equal(t, expectedCSName, customResourceName)
		bytes, err := os.ReadFile(filepath.Join(workingDir, clusterResourcesDir, csFiles[0].Name()))
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}
		var actualCS ofv1alpha1.CatalogSource
		err = yaml.Unmarshal(bytes, &actualCS)
		if err != nil {
			t.Fatalf("failed to unmarshal catalogsource: %v", err)
		}
		expectedCS := ofv1alpha1.CatalogSource{
			TypeMeta: metav1.TypeMeta{
				APIVersion: ofv1alpha1.GroupName + "/" + ofv1alpha1.GroupVersion,
				Kind:       "CatalogSource",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      expectedCSName,
				Namespace: "openshift-marketplace",
			},
			Spec: ofv1alpha1.CatalogSourceSpec{
				SourceType: "grpc",
				Image:      "myregistry/mynamespace/redhat/redhat-operator-index:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			},
		}

		assert.Equal(t, expectedCS, actualCS, "contents of catalogSource file incorrect")

	})
}

func TestGenerateImageMirrors(t *testing.T) {
	type testCase struct {
		caseName                   string
		imgList                    []v2alpha1.CopyImageSchema
		mode                       imageMirrorsGeneratorMode
		forceRepositoryScope       bool
		expectedCategorizedMirrors []categorizedMirrors
		expectedError              bool
	}
	testCases := []testCase{
		{
			caseName:             "Testing GenerateImageMirrors - All images by digest - IDMS only Mode : should pass",
			imgList:              imageListDigestsOnly,
			mode:                 DigestsOnlyMode,
			forceRepositoryScope: false,
			expectedError:        false,
			expectedCategorizedMirrors: []categorizedMirrors{
				{
					category: releaseCategory,
					mirrors:  map[string][]confv1.ImageMirror{"quay.io/openshift-release-dev": {"myregistry/mynamespace/openshift-release-dev"}},
				},
			},
		},
		{
			caseName:                   "Testing GenerateImageMirrors - All images by digest - ITMS only Mode : should generate empty mirrors",
			imgList:                    imageListDigestsOnly,
			mode:                       TagsOnlyMode,
			forceRepositoryScope:       false,
			expectedError:              false,
			expectedCategorizedMirrors: []categorizedMirrors{},
		},
		{
			caseName:             "Testing GenerateImageMirrors for ITMS - Mixed content - TagsOnlyMode : should pass",
			imgList:              imageListMixed,
			mode:                 TagsOnlyMode,
			forceRepositoryScope: false,
			expectedError:        false,
			expectedCategorizedMirrors: []categorizedMirrors{
				{
					category: operatorCategory,
					mirrors:  map[string][]confv1.ImageMirror{"gcr.io/kubebuilder": {"myregistry/mynamespace/kubebuilder"}, "quay.io/cockroachdb": {"myregistry/mynamespace/cockroachdb"}, "quay.io/helmoperators": {"myregistry/mynamespace/helmoperators"}},
				},
				{
					category: genericCategory,
					mirrors:  map[string][]confv1.ImageMirror{"registry.redhat.io/ubi8": {"myregistry/mynamespace/ubi8"}},
				},
			},
		},
		{
			caseName:             "Testing GenerateImageMirrors for IDMS - Mixed content - DigestsOnlyMode : should pass",
			imgList:              imageListMixed,
			mode:                 DigestsOnlyMode,
			forceRepositoryScope: false,
			expectedError:        false,
			expectedCategorizedMirrors: []categorizedMirrors{
				{
					category: operatorCategory,
					mirrors:  map[string][]confv1.ImageMirror{"quay.io/openshift-community-operators": {"myregistry/mynamespace/openshift-community-operators"}, "registry.redhat.io": {"myregistry/mynamespace"}},
				},
				{
					category: releaseCategory,
					mirrors:  map[string][]confv1.ImageMirror{"quay.io/openshift-release-dev": {"myregistry/mynamespace/openshift-release-dev"}},
				},
			},
		},
		{
			caseName:             "Testing GenerateImageMirrors for IDMS - Mixed content - DigestsOnlyMode + repositoryScope : should pass",
			imgList:              imageListMixed,
			mode:                 DigestsOnlyMode,
			forceRepositoryScope: true,
			expectedError:        false,
			expectedCategorizedMirrors: []categorizedMirrors{
				{
					category: operatorCategory,
					mirrors: map[string][]confv1.ImageMirror{
						"quay.io/openshift-community-operators/cockroachdb": {
							"myregistry/mynamespace/openshift-community-operators/cockroachdb"},
						"registry.redhat.io/ubi8-minimal": {
							"myregistry/mynamespace/ubi8-minimal"}},
				},
				{
					category: releaseCategory,
					mirrors: map[string][]confv1.ImageMirror{
						"quay.io/openshift-release-dev/ocp-v4.0-art-dev": {
							"myregistry/mynamespace/openshift-release-dev/ocp-v4.0-art-dev",
						}},
				},
			},
		},
		{
			caseName:             "Testing GenerateImageMirrors for IDMS - Mixed content - TagsOnlyMode + repositoryScope : should pass",
			imgList:              imageListMaxNestedPaths,
			mode:                 TagsOnlyMode,
			forceRepositoryScope: true,
			expectedError:        false,
			expectedCategorizedMirrors: []categorizedMirrors{
				{
					category: operatorCategory,
					mirrors: map[string][]confv1.ImageMirror{
						"quay.io/cockroachdb/cockroach-helm-operator": {
							"myregistry/mynamespace/cockroachdb-cockroach-helm-operator",
						},
					},
				},
			},
		},
	}

	cr := &ClusterResourcesGenerator{
		Log:              clog.New("trace"),
		WorkingDir:       "",
		LocalStorageFQDN: "localhost:55000",
	}
	for _, test := range testCases {
		t.Run(test.caseName, func(t *testing.T) {
			mirrors, err := cr.generateImageMirrors(test.imgList, test.mode, test.forceRepositoryScope)
			if err == nil && test.expectedError {
				t.Fatalf("expecting error, but function did not return in error")
			}
			if err != nil && !test.expectedError {
				t.Fatalf("should not fail")
			}
			assert.Equal(t, len(test.expectedCategorizedMirrors), len(mirrors))
			for _, expectedMirrorsForCategory := range test.expectedCategorizedMirrors {
				isMirrorsForCategoryFound := false
				for _, actualMirrorsForCategory := range mirrors {
					if expectedMirrorsForCategory.category == actualMirrorsForCategory.category {
						isMirrorsForCategoryFound = true
						assert.Equal(t, expectedMirrorsForCategory.mirrors, actualMirrorsForCategory.mirrors)
						break
					}
				}
				if !isMirrorsForCategoryFound {
					t.Fatalf("expecting mirrors for category %s but didn't find one", expectedMirrorsForCategory.category.toString())
				}
			}
		})
	}

}

func TestUpdateServiceGenerator(t *testing.T) {
	log := clog.New("trace")

	tmpDir := t.TempDir()
	workingDir := tmpDir + "/working-dir"

	releaseImage := "quay.io/openshift-release-dev/ocp-release:4.13.10-x86_64"
	graphImage := "localhost:5000/openshift/graph-image:latest"

	t.Run("Testing IDMSGenerator - Disk to Mirror : should pass", func(t *testing.T) {
		cr := &ClusterResourcesGenerator{
			Log:        log,
			WorkingDir: workingDir,
		}
		err := cr.UpdateServiceGenerator(graphImage, releaseImage)
		if err != nil {
			t.Fatalf("should not fail")
		}

		_, err = os.Stat(filepath.Join(workingDir, clusterResourcesDir))
		if err != nil {
			t.Fatalf("output folder should exist")
		}

		resourceFiles, err := os.ReadDir(filepath.Join(workingDir, clusterResourcesDir))
		if err != nil {
			t.Fatalf("ls output folder should not fail")
		}

		if len(resourceFiles) != 1 {
			t.Fatalf("output folder should contain 1 updateservice.yaml file")
		}

		assert.Equal(t, updateServiceFilename, resourceFiles[0].Name())

		// Read the contents of resourceFiles[0]
		filePath := filepath.Join(workingDir, clusterResourcesDir, resourceFiles[0].Name())
		fileContents, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}
		actualOSUS := updateservicev1.UpdateService{}
		err = yaml.Unmarshal(fileContents, &actualOSUS)
		if err != nil {
			t.Fatalf("failed to unmarshall file: %v", err)
		}

		assert.Equal(t, graphImage, actualOSUS.Spec.GraphDataImage)
		assert.Equal(t, "quay.io/openshift-release-dev/ocp-release", actualOSUS.Spec.Releases)
	})
}

func TestGenerateSignatureConfigMap(t *testing.T) {

	t.Run("Testing configmap both yaml&json should pass", func(t *testing.T) {

		tmpDir := t.TempDir()
		workingDir := filepath.Join(tmpDir, "working-dir")
		err := os.MkdirAll(workingDir+"/"+clusterResourcesDir, 0755)
		if err != nil {
			t.Fatal(err)
		}
		//defer os.RemoveAll(workingDir)

		log := clog.New("trace")
		cmJson := cm.ConfigMap{}
		cr := &ClusterResourcesGenerator{
			Log:        log,
			WorkingDir: workingDir,
		}
		data, err := os.ReadFile("../../../tests/test.sig")
		if err != nil {
			t.Fatal(err)
		}
		digest := "37433b71c073c6cbfc8173ec7ab2d99032c8e6d6fe29de06e062d85e33e34531"
		err = cr.GenerateSignatureConfigMap(digest, 0, data)
		if err != nil {
			t.Fatal(err)
		}
		resJson, err := os.ReadFile(workingDir + "/" + clusterResourcesDir + "/sha256-37433b71c073c6cbfc8173ec7ab2d99032c8e6d6fe29de06e062d85e33e34531-1.json")
		if err != nil {
			t.Fatal(err)
		}
		err = json.Unmarshal(resJson, &cmJson)
		if err != nil {
			t.Fatal(err)
		}
		bdJson := len(cmJson.BinaryData["sha256-37433b71c073c6cbfc8173ec7ab2d99032c8e6d6fe29de06e062d85e33e34531-1"])
		assert.Equal(t, bdJson, 1199)

		cmYaml := cm.ConfigMap{}
		resYaml, err := os.ReadFile(workingDir + "/" + clusterResourcesDir + "/sha256-37433b71c073c6cbfc8173ec7ab2d99032c8e6d6fe29de06e062d85e33e34531-1.yaml")
		if err != nil {
			t.Fatal(err)
		}
		err = yaml.Unmarshal(resYaml, &cmYaml)
		if err != nil {
			t.Fatal(err)
		}
		bdYaml := len(cmYaml.BinaryData["sha256-37433b71c073c6cbfc8173ec7ab2d99032c8e6d6fe29de06e062d85e33e34531-1"])
		assert.Equal(t, bdYaml, 1199)

	})

}
