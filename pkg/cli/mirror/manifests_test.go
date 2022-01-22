package mirror

import (
	"testing"

	operatorv1alpha1 "github.com/openshift/api/operator/v1alpha1"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestICSPGeneration(t *testing.T) {
	tests := []struct {
		name        string
		sourceImage reference.DockerImageReference
		destImage   reference.DockerImageReference
		typ         icspType
		expected    []operatorv1alpha1.ImageContentSourcePolicy
		err         string
	}{{
		name: "Valid/OperatorType",
		sourceImage: reference.DockerImageReference{
			Registry:  "some-registry",
			Namespace: "namespace",
			Name:      "image",
			ID:        "digest",
		},
		destImage: reference.DockerImageReference{
			Registry:  "disconn-registry",
			Namespace: "namespace",
			Name:      "image",
			ID:        "digest",
		},
		typ: typeOperator,
		expected: []operatorv1alpha1.ImageContentSourcePolicy{{
			TypeMeta: metav1.TypeMeta{
				APIVersion: operatorv1alpha1.GroupVersion.String(),
				Kind:       "ImageContentSourcePolicy"},
			ObjectMeta: metav1.ObjectMeta{
				Name:   "some-registry-namespace-image:digest-0",
				Labels: map[string]string{"operators.openshift.org/catalog": "true"},
			},
			Spec: operatorv1alpha1.ImageContentSourcePolicySpec{
				RepositoryDigestMirrors: []operatorv1alpha1.RepositoryDigestMirrors{
					{
						Source:  "some-registry/namespace/image",
						Mirrors: []string{"disconn-registry/namespace/image"},
					},
				},
			},
		},
		},
	}, {
		name: "Valid/GenericType",
		sourceImage: reference.DockerImageReference{
			Registry:  "some-registry",
			Namespace: "namespace",
			Name:      "image",
			ID:        "digest",
		},
		destImage: reference.DockerImageReference{
			Registry:  "disconn-registry",
			Namespace: "namespace",
			Name:      "image",
			ID:        "digest",
		},
		expected: []operatorv1alpha1.ImageContentSourcePolicy{{
			TypeMeta: metav1.TypeMeta{
				APIVersion: operatorv1alpha1.GroupVersion.String(),
				Kind:       "ImageContentSourcePolicy"},
			ObjectMeta: metav1.ObjectMeta{
				Name: "some-registry-namespace-image:digest-0",
			},
			Spec: operatorv1alpha1.ImageContentSourcePolicySpec{
				RepositoryDigestMirrors: []operatorv1alpha1.RepositoryDigestMirrors{
					{
						Source:  "some-registry/namespace/image",
						Mirrors: []string{"disconn-registry/namespace/image"},
					},
				},
			},
		},
		},
	}, {
		name: "Valid/ReleaseType",
		sourceImage: reference.DockerImageReference{
			Registry:  "some-registry",
			Namespace: "namespace",
			Name:      "image",
			ID:        "digest",
		},
		destImage: reference.DockerImageReference{
			Registry:  "disconn-registry",
			Namespace: "namespace",
			Name:      "image",
			ID:        "digest",
		},
		typ: typeOCPRelease,
		expected: []operatorv1alpha1.ImageContentSourcePolicy{{
			TypeMeta: metav1.TypeMeta{
				APIVersion: operatorv1alpha1.GroupVersion.String(),
				Kind:       "ImageContentSourcePolicy"},
			ObjectMeta: metav1.ObjectMeta{
				Name: "some-registry-namespace-image:digest-0",
			},
			Spec: operatorv1alpha1.ImageContentSourcePolicySpec{
				RepositoryDigestMirrors: []operatorv1alpha1.RepositoryDigestMirrors{
					{
						Source:  "some-registry/namespace/image",
						Mirrors: []string{"disconn-registry/namespace/image"},
					},
					{
						Source:  "quay.io/openshift-release-dev/ocp-v4.0-art-dev",
						Mirrors: []string{"disconn-registry/namespace/image"},
					},
				},
			},
		},
		},
	}, {
		name: "Invalid/NoDigestMapping",
		sourceImage: reference.DockerImageReference{
			Registry:  "some-registry",
			Namespace: "namespace",
			Name:      "image",
			Tag:       "latest",
		},
		destImage: reference.DockerImageReference{
			Registry:  "disconnected-registry",
			Namespace: "namespace",
			Name:      "image",
			Tag:       "latest",
		},
		expected: nil,
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gen := icspGenerator{
				imageName: test.sourceImage.Exact(),
				icspMapping: map[reference.DockerImageReference]reference.DockerImageReference{
					test.sourceImage: test.destImage,
				},
				icspType: test.typ,
			}
			icsps, err := gen.Run(icspScope, icspSizeLimit)
			require.NoError(t, err)
			require.Equal(t, test.expected, icsps)
		})
	}
}
