package resource

import (
	"strings"
	"testing"
)

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

func TestDefinitionRequiresNameAndPositiveVersion(t *testing.T) {
	d := Definition{Key: "postgres", Name: "PostgreSQL", Version: 1, ResourceTypeKey: "postgres", DriverType: DriverKubernetes, Criteria: []Criterion{{}}}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Definition){
		"blank name":   func(d *Definition) { d.Name = "  \t " },
		"long name":    func(d *Definition) { d.Name = strings.Repeat("a", MaxDefinitionNameLength+1) },
		"control name": func(d *Definition) { d.Name = "a\nb" },
		"no version":   func(d *Definition) { d.Version = 0 },
	} {
		bad := d
		change(&bad)
		if bad.Validate() == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	long := d
	long.Name = strings.Repeat("ạ", MaxDefinitionNameLength)
	if err := long.Validate(); err != nil {
		t.Fatalf("a %d-character name is valid: %v", MaxDefinitionNameLength, err)
	}
}

func TestRankIsTheSharedFiveFieldMatcher(t *testing.T) {
	mk := func(key string, criteria ...Criterion) Definition {
		return Definition{Key: key, Name: key, Version: 1, ResourceTypeKey: "postgres", DriverType: DriverTerraform, DriverAccount: "any-" + key, Criteria: criteria}
	}
	ctx := MatchContext{EnvironmentType: "dev", ApplicationID: "app", EnvironmentID: "env", ResourceID: "db", Class: "default"}
	defs := []Definition{
		mk("wildcard", Criterion{}),
		mk("by-class", Criterion{Class: "default"}),
		mk("by-app-or-env", Criterion{ApplicationID: "app"}, Criterion{EnvironmentID: "env"}),
		mk("other-class", Criterion{Class: "x"}),
		{Key: "elsewhere", Name: "e", Version: 1, ResourceTypeKey: "redis", DriverType: DriverTerraform, Criteria: []Criterion{{}}},
	}
	r := Rank(defs, "postgres", ctx, nil)
	if len(r.Matched) != 3 || len(r.Top) != 1 || r.Top[0].Definition.Key != "by-class" || r.Top[0].Score != 16 {
		t.Fatalf("ranking = %+v", r)
	}
	// AND inside a criterion: both fields must match.
	and := Rank([]Definition{mk("and", Criterion{ApplicationID: "app", EnvironmentID: "other"})}, "postgres", ctx, nil)
	if len(and.Matched) != 0 {
		t.Fatal("AND row matched with one field wrong")
	}
	// Equal top scores are a tie; the account is never a ranking input.
	tie := Rank([]Definition{mk("a", Criterion{Class: "default"}), mk("b", Criterion{Class: "default"})}, "postgres", ctx, nil)
	if len(tie.Top) != 2 || tie.Top[0].Definition.Key != "a" {
		t.Fatalf("tie = %+v", tie)
	}
	// Weights 1/2/4/8/16.
	for i, c := range []Criterion{{EnvironmentType: "dev"}, {ApplicationID: "app"}, {EnvironmentID: "env"}, {ResourceID: "db"}, {Class: "default"}} {
		if got := c.Specificity(); got != 1<<i {
			t.Fatalf("weight %d = %d", i, got)
		}
	}
}

func TestDefinitionRegionAndAwsRegionFormat(t *testing.T) {
	d := Definition{DriverInputs: map[string]any{"values": map[string]any{"variables": map[string]any{"region": "ap-southeast-1"}}}}
	if region, ok := d.DefinitionRegion(); !ok || region != "ap-southeast-1" {
		t.Fatalf("region = %q %v", region, ok)
	}
	if _, ok := (Definition{}).DefinitionRegion(); ok {
		t.Fatal("empty definition has no region")
	}
	for _, good := range []string{"us-east-1", "eu-west-3", "ap-southeast-1", "us-gov-west-1"} {
		if !ValidAWSRegion(good) {
			t.Fatalf("%s rejected", good)
		}
	}
	for _, bad := range []string{"", "mars", "us-east", "US-EAST-1", "${context.env.region}", "us-east-1; rm"} {
		if ValidAWSRegion(bad) {
			t.Fatalf("%q accepted", bad)
		}
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

func TestTypeValidateRejectsMalformedContracts(t *testing.T) {
	cases := []Type{
		{},
		{Key: "postgres", Inputs: []InputField{{Name: "database", Type: "string"}, {Name: "database", Type: "string"}}},
		{Key: "postgres", Inputs: []InputField{{Name: "database", Type: "object"}}},
		{Key: "postgres", Outputs: []OutputField{{Name: "", Type: "string"}}},
		{Key: "postgres", Outputs: []OutputField{{Name: "host", Type: "string"}, {Name: "host", Type: "string"}}},
	}
	for _, typ := range cases {
		if err := typ.Validate(); err == nil {
			t.Fatalf("expected invalid type contract: %#v", typ)
		}
	}
	if err := (Type{Key: "postgres", Inputs: []InputField{{Name: "database", Type: "string"}}, Outputs: []OutputField{{Name: "host", Type: "string"}}}).Validate(); err != nil {
		t.Fatalf("valid type contract rejected: %v", err)
	}
}
