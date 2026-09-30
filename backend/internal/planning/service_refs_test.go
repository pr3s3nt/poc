package planning

import (
	"strings"
	"testing"

	"orchestrator/internal/domain/environment"
)

func serviceSet(consumer environment.Container) environment.Document {
	set := environment.NewDocument()
	set.Modules["backend"] = environment.Module{Profile: "p", Spec: environment.ModuleSpec{
		Containers: map[string]environment.Container{"main": {Image: "b"}},
		Service:    &environment.Service{Ports: map[string]environment.Port{"http": {Port: 8080}}},
	}}
	set.Modules["frontend"] = environment.Module{Profile: "p", Spec: environment.ModuleSpec{Containers: map[string]environment.Container{"main": consumer}}}
	return set
}

func TestValidateServiceReferences(t *testing.T) {
	variable := func(v string) environment.Container {
		return environment.Container{Image: "f", Variables: map[string]string{"URL": v}}
	}
	for name, tc := range map[string]struct {
		consumer environment.Container
		drop     bool
		want     string
	}{
		"resolves":                {consumer: variable("${context.service.backend.http}")},
		"resolves with spaces":    {consumer: variable("${ context.service.backend.http }")},
		"escaped literal ignored": {consumer: variable("$${context.service.gone.http}")},
		"other context ignored":   {consumer: variable("${context.app.id}")},
		"provider removed":        {consumer: variable("${context.service.backend.http}"), drop: true, want: "not in the resulting Environment"},
		"undeclared port":         {consumer: variable("${context.service.backend.grpc}"), want: "not declared"},
		"malformed path":          {consumer: variable("${context.service.backend}"), want: "malformed"},
		"blank segment":           {consumer: variable("${context.service..http}"), want: "malformed"},
		"extra segment":           {consumer: variable("${context.service.backend.http.x}"), want: "malformed"},
		"args inspected":          {consumer: environment.Container{Image: "f", Args: []string{"--api=${context.service.backend.http}"}}, drop: true, want: "not in the resulting Environment"},
		"command inspected":       {consumer: environment.Container{Image: "f", Command: []string{"${context.service.backend.grpc}"}}, want: "not declared"},
	} {
		set := serviceSet(tc.consumer)
		if tc.drop {
			delete(set.Modules, "backend")
		}
		err := ValidateServiceReferences(set)
		if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("%s: got %v, want %q", name, err, tc.want)
		}
	}
}
