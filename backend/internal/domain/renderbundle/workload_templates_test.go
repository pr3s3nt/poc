package renderbundle

import "testing"

func testTemplate() WorkloadTemplate {
	return WorkloadTemplate{ID: "backend", Version: "1.0.0", Kind: "Deployment", Labels: map[string]string{"vi": "Backend"}, InputsSchema: map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"workers": map[string]any{"type": "integer", "minimum": float64(0)}}}, Defaults: map[string]any{"workers": float64(0)}}
}
func TestWorkloadTemplateVersionsAndInputs(t *testing.T) {
	template := testTemplate()
	if err := template.Validate(); err != nil {
		t.Fatal(err)
	}
	bundle := Bundle{}
	bundle.Contract.Manifest.WorkloadTemplates = []WorkloadTemplate{template}
	catalog, err := TemplateCatalog(map[string]Bundle{"a": bundle, "b": bundle.Clone()})
	if err != nil || len(catalog) != 1 {
		t.Fatalf("dedup: %v %v", catalog, err)
	}
	_, effective, err := MatchWorkloadTemplate(catalog, template.ID, template.Version, template.SHA256(), template.Kind, nil)
	if err != nil || effective["workers"] != float64(0) {
		t.Fatalf("explicit default lost: %v %v", effective, err)
	}
	if _, _, err = MatchWorkloadTemplate(catalog, template.ID, template.Version, template.SHA256(), template.Kind, map[string]any{"workers": -1.0}); err == nil {
		t.Fatal("schema constraint ignored")
	}
	cloned := bundle.Clone()
	cloned.Contract.Manifest.WorkloadTemplates[0].Labels["vi"] = "Changed"
	if bundle.Contract.Manifest.WorkloadTemplates[0].Labels["vi"] != "Backend" {
		t.Fatal("mutable template alias")
	}
	if _, err = TemplateCatalog(map[string]Bundle{"a": bundle, "b": cloned}); err == nil {
		t.Fatal("conflicting immutable version accepted")
	}
}
