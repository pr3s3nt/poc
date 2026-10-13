package tfexec

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"

	"orchestrator/internal/domain/tfmodule"
)

// Executor-owned variable names the executor knows how to supply. A module
// that declares another executor-owned name cannot run (fail closed).
const (
	varRegion         = "region"
	varTags           = "tags"
	varName           = "name"
	varMasterPassword = "master_password"
	varPassword       = "password"
)

func executorSecret(name string) bool { return name == varMasterPassword || name == varPassword }

// executorValues are the values the executor itself pins for one instance.
type executorValues struct {
	Region string
	Tags   map[string]string
	Name   string
	// Secrets holds executor-generated secret variable values by name.
	Secrets map[string]string
}

// variables is the resolved variable set of one invocation.
type variables struct {
	// All is the tfvars content. It may hold secrets and is written only to a
	// private file.
	All map[string]any
	// Public is the nonsecret part kept in the attempt module snapshot.
	Public map[string]any
	// SecretNames are executor-owned secret variables, held by secret-store
	// reference outside the database.
	SecretNames []string
}

// buildVariables merges the resolved Definition/param inputs with the
// executor-owned values and checks the result against the pinned module
// contract: unknown or overriding names, missing required variables and value
// types are refused with the variable name only, never a value.
func buildVariables(b tfmodule.Bundle, inputs map[string]any, ev executorValues) (variables, error) {
	owned := map[string]bool{}
	for _, name := range b.Contract.ExecutorOwned {
		owned[name] = true
	}
	out := variables{All: map[string]any{}, Public: map[string]any{}}
	names := make([]string, 0, len(inputs))
	for name := range inputs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		declared, ok := b.Contract.Inputs[name]
		switch {
		case owned[name]:
			return variables{}, fmt.Errorf("%w: %s is executor-owned", ErrInput, safeName(name))
		case !ok:
			return variables{}, fmt.Errorf("%w: %s is not declared by the module", ErrInput, safeName(name))
		}
		want, err := tfmodule.ParseType(declared.Type)
		if err != nil {
			return variables{}, fmt.Errorf("%w: %s has an unsupported type", ErrInput, safeName(name))
		}
		if err := checkValue(want, inputs[name]); err != nil {
			return variables{}, fmt.Errorf("%w: %s %v", ErrInput, safeName(name), err)
		}
		out.All[name] = inputs[name]
		if !declared.Sensitive && !secretKey(name) {
			out.Public[name] = inputs[name]
		}
	}
	ownedNames := make([]string, 0, len(owned))
	for name := range owned {
		ownedNames = append(ownedNames, name)
	}
	sort.Strings(ownedNames)
	for _, name := range ownedNames {
		switch {
		case name == varRegion && ev.Region != "":
			out.All[name], out.Public[name] = ev.Region, ev.Region
		case name == varTags:
			out.All[name], out.Public[name] = ev.Tags, ev.Tags
		case name == varName && ev.Name != "":
			out.All[name], out.Public[name] = ev.Name, ev.Name
		case executorSecret(name):
			value, ok := ev.Secrets[name]
			if !ok || value == "" {
				return variables{}, fmt.Errorf("%w: the executor secret %s is unavailable", ErrInput, safeName(name))
			}
			out.All[name] = value
			out.SecretNames = append(out.SecretNames, name)
		default:
			return variables{}, fmt.Errorf("%w: the executor cannot supply %s", ErrInput, safeName(name))
		}
	}
	required := make([]string, 0)
	for name, in := range b.Contract.Inputs {
		if !in.HasDefault {
			required = append(required, name)
		}
	}
	sort.Strings(required)
	for _, name := range required {
		if _, ok := out.All[name]; !ok {
			return variables{}, fmt.Errorf("%w: the required variable %s has no value", ErrInput, safeName(name))
		}
	}
	return out, nil
}

// secretKey mirrors the secret-looking input names the orchestrator never
// persists in snapshots.
func secretKey(name string) bool {
	switch name {
	case "password", "secret", "token", "credentials", "masterPassword", "master_password":
		return true
	}
	return false
}

// declaredSecretVariables lists the variables a module declares sensitive.
func declaredSecretVariables(b tfmodule.Bundle) map[string]bool {
	out := map[string]bool{}
	for name, in := range b.Contract.Inputs {
		if in.Sensitive {
			out[name] = true
		}
	}
	return out
}

func safeName(name string) string {
	if len(name) > 64 {
		name = name[:64]
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return "?"
		}
	}
	return name
}

// checkValue verifies a concrete value against a supported Terraform type.
// The error names the expected type only.
func checkValue(want tfmodule.ValueType, value any) error {
	switch want.Kind {
	case "any":
		return nil
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("must be a string")
		}
	case "number":
		switch value.(type) {
		case float64, float32, int, int32, int64, uint, uint32, uint64:
		default:
			return fmt.Errorf("must be a number")
		}
	case "bool":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("must be a bool")
		}
	case "list":
		items, ok := value.([]any)
		if !ok {
			if strs, isStrings := value.([]string); isStrings {
				for _, s := range strs {
					if err := checkValue(*want.Elem, s); err != nil {
						return fmt.Errorf("must be a list(%s)", want.Elem.String())
					}
				}
				return nil
			}
			return fmt.Errorf("must be a list(%s)", want.Elem.String())
		}
		for _, item := range items {
			if err := checkValue(*want.Elem, item); err != nil {
				return fmt.Errorf("must be a list(%s)", want.Elem.String())
			}
		}
	case "map":
		items, ok := value.(map[string]any)
		if !ok {
			if strs, isStrings := value.(map[string]string); isStrings {
				for _, s := range strs {
					if err := checkValue(*want.Elem, s); err != nil {
						return fmt.Errorf("must be a map(%s)", want.Elem.String())
					}
				}
				return nil
			}
			return fmt.Errorf("must be a map(%s)", want.Elem.String())
		}
		for _, item := range items {
			if err := checkValue(*want.Elem, item); err != nil {
				return fmt.Errorf("must be a map(%s)", want.Elem.String())
			}
		}
	default:
		return fmt.Errorf("has an unsupported type")
	}
	return nil
}

// randomSecret returns a fresh 32-character URL-safe secret.
func randomSecret() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("tfexec: cannot draw a secret")
	}
	return hex.EncodeToString(raw), nil
}
