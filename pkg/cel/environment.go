package cel

import (
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// NewPlacementEnv creates a CEL environment configured for evaluating
// placement policy rules and preferences. The environment exposes an "env"
// variable representing a DCM Environment as a nested map structure.
func NewPlacementEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("env", cel.MapType(cel.StringType, cel.DynType)),
	)
}

// CompileRule compiles a CEL rule expression (must return bool).
func CompileRule(env *cel.Env, expr string) (cel.Program, error) {
	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}

	// Check output type is bool
	if ast.OutputType() != cel.BoolType {
		return nil, &TypeMismatchError{Expected: "bool", Got: ast.OutputType().String()}
	}

	return env.Program(ast)
}

// CompilePrefer compiles a CEL prefer expression (must return a number).
func CompilePrefer(env *cel.Env, expr string) (cel.Program, error) {
	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}

	// Prefer expressions should return a numeric type
	outType := ast.OutputType()
	if outType != cel.IntType && outType != cel.DoubleType && outType != cel.DynType {
		return nil, &TypeMismatchError{Expected: "number", Got: outType.String()}
	}

	return env.Program(ast)
}

// CheckSyntax validates that a CEL expression is syntactically valid.
func CheckSyntax(env *cel.Env, expr string) error {
	_, issues := env.Parse(expr)
	if issues != nil && issues.Err() != nil {
		return issues.Err()
	}
	return nil
}

// TypeMismatchError indicates a CEL expression returned the wrong type.
type TypeMismatchError struct {
	Expected string
	Got      string
}

func (e *TypeMismatchError) Error() string {
	return "expected " + e.Expected + ", got " + e.Got
}

// EnvironmentToMap converts an Environment's relevant fields to a map
// suitable for CEL evaluation. This is the "env" variable in placement policies.
func EnvironmentToMap(env map[string]any) map[string]any {
	return env
}

// EvalBool evaluates a compiled CEL program that should return a boolean.
func EvalBool(program cel.Program, vars map[string]any) (bool, error) {
	out, _, err := program.Eval(vars)
	if err != nil {
		return false, err
	}
	b, ok := out.Value().(bool)
	if !ok {
		return false, &TypeMismatchError{Expected: "bool", Got: out.Type().TypeName()}
	}
	return b, nil
}

// EvalFloat evaluates a compiled CEL program that should return a number.
func EvalFloat(program cel.Program, vars map[string]any) (float64, error) {
	out, _, err := program.Eval(vars)
	if err != nil {
		return 0, err
	}
	return toFloat(out)
}

func toFloat(val ref.Val) (float64, error) {
	switch v := val.Value().(type) {
	case float64:
		return v, nil
	case int64:
		return float64(v), nil
	case types.Double:
		return float64(v), nil
	case types.Int:
		return float64(v), nil
	default:
		return 0, &TypeMismatchError{Expected: "number", Got: val.Type().TypeName()}
	}
}
