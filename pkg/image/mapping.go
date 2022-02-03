package image

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/openshift/oc/pkg/cli/image/imagesource"
)

type TypedImage struct {
	imagesource.TypedImageReference
	// Category adds image category type to TypedImageReference
	Category ImageType
}

// ParseTypeImage will create a TypedImage from a string and type
func ParseTypedImage(image string, typ ImageType) (TypedImage, error) {
	ref, err := imagesource.ParseReference(image)
	if err != nil {
		return TypedImage{}, err
	}
	return TypedImage{ref, typ}, nil
}

type TypedImageMapping map[TypedImage]TypedImage

// ReadImageMapping reads a mapping.txt file and parses each line into a map k/v.
func ReadImageMapping(mappingsPath, separator string, typ ImageType) (TypedImageMapping, error) {
	f, err := os.Open(mappingsPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	mappings := TypedImageMapping{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		text := scanner.Text()
		split := strings.Split(text, separator)
		if len(split) != 2 {
			return nil, fmt.Errorf("mapping %q expected to have exactly one \"=\"", text)
		}
		srcTypedRef, err := ParseTypedImage(strings.TrimSpace(split[0]), typ)
		if err != nil {
			return nil, err
		}
		dstTypedRef, err := ParseTypedImage(strings.TrimSpace(split[1]), typ)
		if err != nil {
			return nil, err
		}
		mappings[srcTypedRef] = dstTypedRef
	}

	return mappings, scanner.Err()
}

// WriteImageMapping reads a mapping.txt file and parses each line into a map k/v.
func (m TypedImageMapping) WriteImageMapping(mappingsPath string) error {
	f, err := os.Create(mappingsPath)
	if err != nil {
		return err
	}
	defer f.Close()
	for fromStr, toStr := range m {
		_, err := f.WriteString(fmt.Sprintf("%s=%s\n", fromStr.Ref.Exact(), toStr.Ref.Exact()))
		if err != nil {
			return err
		}
	}
	return nil
}

// ToRegistry will convet a mapping to disk to a registry to registry mapping
func (m TypedImageMapping) ToRegistry(registry, namespace string) {
	for src, dest := range m {
		dest.Type = imagesource.DestinationRegistry
		dest.Ref.Registry = registry
		dest.Ref.Namespace = path.Join(namespace, dest.Ref.Namespace)
		dest.Ref.ID = src.Ref.ID
		dest.Ref.Tag = src.Ref.Tag
		m[src] = dest
	}
}

// Merge will add new image maps to current map
func (m TypedImageMapping) Merge(in TypedImageMapping) {
	for k, v := range in {
		m[k] = v
	}
}

// Add stores a key-value pair into image map
func (m TypedImageMapping) Add(srcRef, dstRef imagesource.TypedImageReference, typ ImageType) {
	srcTypedRef := TypedImage{
		TypedImageReference: srcRef,
		Category:            typ,
	}
	dstTypedRef := TypedImage{
		TypedImageReference: dstRef,
		Category:            TypeGeneric,
	}
	m[srcTypedRef] = dstTypedRef
}

// Remove will remove an image from the map given the TypeImageReference and type
func (m TypedImageMapping) Remove(ref imagesource.TypedImageReference, typ ImageType) {
	typedRef := TypedImage{
		TypedImageReference: ref,
		Category:            typ,
	}
	delete(m, typedRef)
}

// ByCategory will return a pruned mapping by containing provided types
func ByCategory(m TypedImageMapping, types ...ImageType) TypedImageMapping {
	foundTypes := map[ImageType]struct{}{}
	for _, typ := range types {
		foundTypes[typ] = struct{}{}
	}
	// return a new map with the pruned mapping
	prunedMap := TypedImageMapping{}
	for key, val := range m {
		_, ok := foundTypes[key.Category]
		if ok {
			prunedMap[key] = val
		}
	}
	return prunedMap
}
