package codec

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/dcm-io/dcm/pkg/apis/meta"
	"github.com/dcm-io/dcm/pkg/apis/v1alpha1"
	"sigs.k8s.io/yaml"
)

// registry maps "apiVersion.Kind" to the Go type for that resource.
var registry = map[string]reflect.Type{
	key(v1alpha1.GroupVersion, v1alpha1.KindApplication):     reflect.TypeOf(v1alpha1.Application{}),
	key(v1alpha1.GroupVersion, v1alpha1.KindEnvironment):     reflect.TypeOf(v1alpha1.Environment{}),
	key(v1alpha1.GroupVersion, v1alpha1.KindResourceType):    reflect.TypeOf(v1alpha1.ResourceType{}),
	key(v1alpha1.GroupVersion, v1alpha1.KindRecipe):          reflect.TypeOf(v1alpha1.Recipe{}),
	key(v1alpha1.GroupVersion, v1alpha1.KindPlacementPolicy): reflect.TypeOf(v1alpha1.PlacementPolicy{}),
}

func key(apiVersion, kind string) string {
	return apiVersion + "." + kind
}

// typeMeta is used to peek at apiVersion and kind before full deserialization.
type typeMeta struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

// Decode deserializes YAML or JSON bytes into the appropriate domain object.
// It peeks at apiVersion and kind to determine the target Go type.
func Decode(data []byte) (meta.Object, error) {
	var tm typeMeta
	if err := yaml.Unmarshal(data, &tm); err != nil {
		return nil, fmt.Errorf("failed to peek apiVersion/kind: %w", err)
	}
	if tm.APIVersion == "" || tm.Kind == "" {
		return nil, fmt.Errorf("missing apiVersion or kind")
	}

	t, ok := registry[key(tm.APIVersion, tm.Kind)]
	if !ok {
		return nil, fmt.Errorf("unknown type: apiVersion=%q kind=%q", tm.APIVersion, tm.Kind)
	}

	obj := reflect.New(t).Interface()
	// sigs.k8s.io/yaml converts YAML to JSON, then uses encoding/json tags
	if err := yaml.Unmarshal(data, obj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %s/%s: %w", tm.APIVersion, tm.Kind, err)
	}

	metaObj, ok := obj.(meta.Object)
	if !ok {
		return nil, fmt.Errorf("type %s/%s does not implement meta.Object", tm.APIVersion, tm.Kind)
	}
	return metaObj, nil
}

// Encode serializes a domain object to YAML bytes.
func Encode(obj meta.Object) ([]byte, error) {
	data, err := yaml.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: %w", err)
	}
	return data, nil
}

// EncodeJSON serializes a domain object to JSON bytes.
func EncodeJSON(obj meta.Object) ([]byte, error) {
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return data, nil
}
