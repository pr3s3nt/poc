package resource

import "testing"

func TestDescriptorRoundTrip(t *testing.T) {
	d, err := ParseDescriptor("postgres.default#shared.acceptance-db")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d.Type != "postgres" || d.Class != "default" || d.ID != "shared.acceptance-db" {
		t.Fatalf("unexpected descriptor: %#v", d)
	}
	if d.String() != "postgres.default#shared.acceptance-db" {
		t.Fatalf("unexpected rendering: %s", d.String())
	}
}

func TestDescriptorRejectsMalformedInput(t *testing.T) {
	for _, in := range []string{"postgres#db", "postgres.default", "postgres.default#", ".default#db", "Postgres.default#db"} {
		if _, err := ParseDescriptor(in); err == nil {
			t.Fatalf("expected %q to be rejected", in)
		}
	}
}

// TestCriterionWeightsFollowTheMatchingTable pins the Humanitec specificity
// order: class outranks res_id, which outranks env_id, app_id and env_type.
func TestCriterionWeightsFollowTheMatchingTable(t *testing.T) {
	cases := []struct {
		criterion Criterion
		want      int
	}{
		{Criterion{EnvironmentType: "development"}, 1},
		{Criterion{ApplicationID: "acceptance"}, 2},
		{Criterion{EnvironmentID: "dev"}, 4},
		{Criterion{ResourceID: "shared.acceptance-db"}, 8},
		{Criterion{Class: "large"}, 16},
		{Criterion{EnvironmentType: "development", ApplicationID: "acceptance"}, 3},
		{Criterion{Class: "large", ResourceID: "shared.acceptance-db"}, 24},
	}
	for _, c := range cases {
		if got := c.criterion.Specificity(); got != c.want {
			t.Fatalf("specificity of %#v is %d, want %d", c.criterion, got, c.want)
		}
	}
	if (Criterion{Class: "large"}).Specificity() <= (Criterion{ResourceID: "x"}).Specificity() {
		t.Fatal("class must outrank res_id")
	}
	if (Criterion{ResourceID: "x"}).Specificity() <= (Criterion{EnvironmentID: "dev"}).Specificity() {
		t.Fatal("res_id must outrank env_id")
	}
}

func TestCriterionMatchesOnlyDeclaredFields(t *testing.T) {
	c := Criterion{ApplicationID: "acceptance-cloud"}
	if !c.Matches(MatchContext{ApplicationID: "acceptance-cloud", EnvironmentType: "development"}) {
		t.Fatal("expected a match")
	}
	if c.Matches(MatchContext{ApplicationID: "acceptance"}) {
		t.Fatal("expected no match for a different application")
	}
	if !(Criterion{}).Matches(MatchContext{ApplicationID: "anything"}) {
		t.Fatal("an empty criterion matches every context")
	}
}

func TestDefinitionRejectsUnknownExecutionProfile(t *testing.T) {
	d := Definition{Key: "postgres", ResourceTypeKey: "postgres", DriverType: DriverKubernetes, Criteria: []Criterion{{}}, ExecutionProfile: "unknown"}
	if err := d.Validate(); err == nil {
		t.Fatal("expected invalid profile rejection")
	}
	d.ExecutionProfile = "internal-k8s"
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateParamsEnforcesInputContract(t *testing.T) {
	typ := Type{Key: "postgres", Inputs: []InputField{
		{Name: "database", Type: "string", Required: true},
		{Name: "username", Type: "string", Required: true},
	}}
	if err := typ.ValidateParams(map[string]any{"database": "acceptance", "username": "app"}); err != nil {
		t.Fatalf("valid params rejected: %v", err)
	}
	if err := typ.ValidateParams(map[string]any{"database": "acceptance", "username": "app", "storage": "20Gi"}); err == nil {
		t.Fatal("expected an undeclared input error")
	}
	if err := typ.ValidateParams(map[string]any{"database": "acceptance"}); err == nil {
		t.Fatal("expected a missing required input error")
	}
	if err := typ.ValidateParams(map[string]any{"database": 16, "username": "app"}); err == nil {
		t.Fatal("expected a type error")
	}
}

func TestValidateOutputsEnforcesContract(t *testing.T) {
	typ := Type{Key: "postgres", Outputs: []OutputField{
		{Name: "host", Type: "string", Required: true},
		{Name: "port", Type: "number", Required: true},
		{Name: "password", Type: "string", Required: true, Secret: true},
	}}
	if err := typ.ValidateOutputs(map[string]any{"host": "db", "port": float64(5432), "password": "x"}); err != nil {
		t.Fatalf("valid outputs rejected: %v", err)
	}
	if err := typ.ValidateOutputs(map[string]any{"host": "db", "port": "5432", "password": "x"}); err == nil {
		t.Fatal("expected a type error for port")
	}
	if err := typ.ValidateOutputs(map[string]any{"host": "db", "port": float64(1)}); err == nil {
		t.Fatal("expected a missing required output error")
	}
	if err := typ.ValidateOutputs(map[string]any{"host": "db", "port": float64(1), "password": "x", "extra": "y"}); err == nil {
		t.Fatal("expected an undeclared output error")
	}
	if got := typ.SecretOutputs(); len(got) != 1 || got[0] != "password" {
		t.Fatalf("unexpected secret outputs: %v", got)
	}
}
