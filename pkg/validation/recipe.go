package validation

import "github.com/dcm-io/dcm/pkg/apis/v1alpha1"

var validRecipeTypes = map[string]bool{
	"terraform":           true,
	"ansible":             true,
	"helm":                true,
	"kubernetes-operator": true,
	"pulumi":              true,
	"custom":              true,
}

// ValidateRecipe checks structural validity of a Recipe.
func ValidateRecipe(recipe *v1alpha1.Recipe) *Result {
	r := &Result{}

	validateObjectMeta(recipe.Metadata, r)

	if recipe.Spec.ResourceType == "" {
		r.AddError("spec.resourceType", "is required")
	}

	if recipe.Spec.Type == "" {
		r.AddError("spec.type", "is required")
	} else if !validRecipeTypes[recipe.Spec.Type] {
		r.AddErrorf("spec.type", "%q is not valid (valid: terraform, ansible, helm, kubernetes-operator, pulumi, custom)", recipe.Spec.Type)
	}

	if len(recipe.Spec.Source) == 0 {
		r.AddError("spec.source", "is required")
	}

	return r
}
