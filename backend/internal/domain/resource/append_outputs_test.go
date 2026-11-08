package resource

import (
	"errors"
	"testing"
)

// UC-02 EXT-01..03: appended outputs are optional additions only.
func TestAppendOutputs(t *testing.T) {
	base := Type{Key: "postgres", Inputs: []InputField{{Name: "database", Type: "string", Required: true}}, Outputs: []OutputField{{Name: "host", Type: "string", Required: true}, {Name: "password", Type: "string", Required: true, Secret: true}}}
	next, err := AppendOutputs(base, []OutputField{{Name: "uri", Type: "string", Secret: true}, {Name: "jdbcUrl", Type: "string"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Outputs) != 4 || next.Outputs[0] != base.Outputs[0] || next.Outputs[1] != base.Outputs[1] || next.Outputs[2].Name != "uri" || !next.Outputs[2].Secret || next.Outputs[3].Required {
		t.Fatalf("existing fields must stay first and unchanged: %+v", next.Outputs)
	}
	if len(base.Outputs) != 2 || len(next.Inputs) != 1 {
		t.Fatalf("the source Type and inputs must not change: %+v %+v", base, next)
	}
	for name, outputs := range map[string][]OutputField{
		"empty":      nil,
		"required":   {{Name: "uri", Type: "string", Required: true}},
		"blank name": {{Name: "", Type: "string"}},
		"bad type":   {{Name: "uri", Type: "object"}},
		"same twice": {{Name: "uri", Type: "string"}, {Name: "uri", Type: "string"}},
		"redeclare":  {{Name: "password", Type: "string"}},
	} {
		if _, err := AppendOutputs(base, outputs); err == nil {
			t.Errorf("%s: must be refused", name)
		}
	}
	if _, err := AppendOutputs(base, []OutputField{{Name: "host", Type: "string", Secret: true}}); !errors.Is(err, ErrOutputExists) {
		t.Fatalf("an existing name must be ErrOutputExists, got %v", err)
	}
}
