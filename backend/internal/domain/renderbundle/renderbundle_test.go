package renderbundle

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func schemaOf(t *testing.T, text string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

const inputSchema = `{"type":"object","additionalProperties":false,"required":["team"],"properties":{
 "team":{"type":"string","pattern":"^[a-z]+$","maxLength":10},
 "replicas":{"type":"integer","minimum":1,"maximum":5,"default":2},
 "enabled":{"type":"boolean","default":true},
 "ratio":{"type":"number","default":0.5},
 "label":{"type":"string","default":"x"},
 "tags":{"type":"array","items":{"type":"string"},"default":["a"]},
 "tier":{"type":"string","enum":["gold","silver"]},
 "nested":{"type":"object","additionalProperties":false,"properties":{"depth":{"type":"integer","default":3}}}}}`

func TestEffectiveAppliesDefaultsOnlyToAbsentKeys(t *testing.T) {
	schema := schemaOf(t, inputSchema)
	if err := ValidateSchema(schema); err != nil {
		t.Fatal(err)
	}
	supplied := map[string]any{"team": "payments", "enabled": false, "ratio": 0.0, "label": "", "tags": []any{}, "nested": map[string]any{}}
	got, err := Effective(schema, supplied)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"team": "payments", "replicas": 2.0, "enabled": false, "ratio": 0.0, "label": "", "tags": []any{}, "nested": map[string]any{"depth": 3.0}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
	if _, ok := supplied["replicas"]; ok {
		t.Fatal("supplied inputs were changed")
	}
	again, _ := Effective(schema, supplied)
	a, _ := InputsHash(got)
	b, _ := InputsHash(again)
	if a != b {
		t.Fatal("hash is not deterministic")
	}
}

func TestEffectiveRejectsWithoutCoercionOrEcho(t *testing.T) {
	schema := schemaOf(t, inputSchema)
	for name, supplied := range map[string]map[string]any{
		"missing required": {},
		"string as number": {"team": "a", "replicas": "2"},
		"fraction":         {"team": "a", "replicas": 2.5},
		"bounds":           {"team": "a", "replicas": 9.0},
		"pattern":          {"team": "UPPER"},
		"enum":             {"team": "a", "tier": "bronze"},
		"unknown key":      {"team": "a", "secret-value-sentinel": "x"},
		"nested unknown":   {"team": "a", "nested": map[string]any{"other": 1.0}},
		"reference":        {"team": "a", "label": "${resources.db.outputs.host}"},
		"null":             {"team": nil},
	} {
		_, err := Effective(schema, supplied)
		if !errors.Is(err, ErrInputInvalid) {
			t.Fatalf("%s: want invalid input, got %v", name, err)
		}
		if strings.Contains(err.Error(), "secret-value-sentinel") || strings.Contains(err.Error(), "UPPER") {
			t.Fatalf("%s: error echoes submitted data: %v", name, err)
		}
	}
}

func TestValidateSchemaRejectsUnsupportedSubset(t *testing.T) {
	for name, text := range map[string]string{
		"root not object":     `{"type":"string"}`,
		"open object":         `{"type":"object","properties":{}}`,
		"ref":                 `{"type":"object","additionalProperties":false,"properties":{"a":{"$ref":"#/x"}}}`,
		"combinator":          `{"type":"object","additionalProperties":false,"properties":{"a":{"type":"string","oneOf":[]}}}`,
		"null type":           `{"type":"object","additionalProperties":false,"properties":{"a":{"type":"null"}}}`,
		"secret name":         `{"type":"object","additionalProperties":false,"properties":{"dbPassword":{"type":"string"}}}`,
		"bad default":         `{"type":"object","additionalProperties":false,"properties":{"a":{"type":"integer","default":"1"}}}`,
		"bad pattern":         `{"type":"object","additionalProperties":false,"properties":{"a":{"type":"string","pattern":"("}}}`,
		"misplaced keyword":   `{"type":"object","additionalProperties":false,"properties":{"a":{"type":"string","minimum":1}}}`,
		"required undeclared": `{"type":"object","additionalProperties":false,"required":["b"],"properties":{}}`,
		"array without items": `{"type":"object","additionalProperties":false,"properties":{"a":{"type":"array"}}}`,
	} {
		if err := ValidateSchema(schemaOf(t, text)); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s: want invalid schema, got %v", name, err)
		}
	}
	deep := `{"type":"object","additionalProperties":false,"properties":{"a":` + strings.Repeat(`{"type":"object","additionalProperties":false,"properties":{"a":`, 8) + `{"type":"string"}` + strings.Repeat(`}}`, 8) + `}}`
	if err := ValidateSchema(schemaOf(t, deep)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("depth over 8 accepted: %v", err)
	}
}

const manifest = `apiVersion: orchestrator.dev/render-bundle/v1
kind: RenderBundle
metadata:
  name: standard-workload
renderer:
  type: score-k8s
  version: 0.15.0
inputContract: orchestrator.dev/render-input/v1
inputsSchema: inputs.schema.json
defaults: defaults.yaml
normalize: normalize/score.tpl
provisioners:
  - provisioners/a.provisioners.yaml
patches: []
`

func TestParseManifestIsStrict(t *testing.T) {
	m, err := ParseManifest([]byte(manifest))
	if err != nil || m.Metadata.Name != "standard-workload" || len(m.Patches) != 0 {
		t.Fatalf("valid manifest: %v %#v", err, m)
	}
	for name, text := range map[string]string{
		"unknown field":  manifest + "command: sh\n",
		"duplicate key":  manifest + "kind: RenderBundle\n",
		"missing field":  strings.Replace(manifest, "patches: []\n", "", 1),
		"traversal":      strings.Replace(manifest, "normalize/score.tpl", "../score.tpl", 1),
		"absolute":       strings.Replace(manifest, "normalize/score.tpl", "/etc/score.tpl", 1),
		"encoded":        strings.Replace(manifest, "normalize/score.tpl", "normalize/%2e%2e/score.tpl", 1),
		"repeated":       strings.Replace(manifest, "patches: []", "patches: [p.tpl, p.tpl]", 1),
		"bad name":       strings.Replace(manifest, "standard-workload", "Standard_Workload", 1),
		"engine version": strings.Replace(manifest, "version: 0.15.0", "version: 0.16.0", 1),
		"api version":    strings.Replace(manifest, "render-bundle/v1", "render-bundle/v2", 1),
		"input contract": strings.Replace(manifest, "render-input/v1", "render-input/v2", 1),
	} {
		if _, err := ParseManifest([]byte(text)); err == nil {
			t.Fatalf("%s accepted", name)
		} else if strings.HasPrefix(name, "engine") || strings.HasPrefix(name, "api") || strings.HasPrefix(name, "input") {
			if !errors.Is(err, ErrUnsupported) {
				t.Fatalf("%s: want unsupported, got %v", name, err)
			}
		}
	}
}
