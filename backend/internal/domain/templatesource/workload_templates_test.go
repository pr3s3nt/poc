package templatesource

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

func TestWorkloadCapabilitiesAreBoundedAndCloned(t *testing.T) {
	if err := validateCapabilities([]string{CustomExecutionV1, ContainerLifecycleV1, PodVolumesV1, ContainerProbesV1}); err != nil {
		t.Fatal(err)
	}
	for _, list := range [][]string{{"unknown"}, {PodVolumesV1, PodVolumesV1}} {
		if validateCapabilities(list) == nil {
			t.Fatal("accepted invalid capability")
		}
	}
	b := Bundle{Contract: Contract{Manifest: Manifest{WorkloadCapabilities: []string{CustomExecutionV1}}}}
	copy := b.Clone()
	copy.Contract.Manifest.WorkloadCapabilities[0] = "changed"
	if b.Contract.Manifest.WorkloadCapabilities[0] != CustomExecutionV1 {
		t.Fatal("clone aliases capability list")
	}
}

// Reading immutable retired source metadata must not break unrelated Preview.
func TestRetiredPolicySourceIsReadableButCannotAuthorOrExecute(t *testing.T) {
	manifest := Manifest{WorkloadCapabilities: []string{"workload-defaults-v1"}, WorkloadTemplates: []WorkloadTemplate{testTemplate()}}
	if err := validateCapabilities(manifest.WorkloadCapabilities); err != nil {
		t.Fatal("retired registry metadata unreadable", err)
	}
	catalog, err := TemplateCatalog(map[string]Bundle{"old": {Contract: Contract{Manifest: manifest}}})
	if err != nil || len(catalog) != 0 {
		t.Fatal("retired source offered for authoring", catalog, err)
	}
	if err := ValidateWorkloadCapabilities(manifest, nil, nil); err == nil {
		t.Fatal("retired source executable")
	}
}
