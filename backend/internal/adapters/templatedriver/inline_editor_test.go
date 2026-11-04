package templatedriver

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestInlineEditorStartingTemplatesRenderWorkload(t *testing.T) {
	raw, err := os.ReadFile("../../../../frontend/src/features/platform/inlineTemplates.ts")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"bundle.yaml": []byte("apiVersion: orchestrator.dev/template-source/v1\nkind: TemplateSource\nmetadata:\n  name: inline-editor\nrenderer:\n  type: template\n  version: 1.0.0\ninputContract: orchestrator.dev/template-input/v1\ninputsSchema: inputs.json\ndefaults: defaults.yaml\nresources: [deployment.tpl,service.tpl]\n"),
		"inputs.json": []byte(`{"type":"object","additionalProperties":false,"properties":{}}`), "defaults.yaml": []byte("{}"),
	}
	for _, name := range []string{"deployment", "service"} {
		_, rest, ok := strings.Cut(string(raw), "export const "+name+"Template = `")
		if !ok {
			t.Fatal("missing editor template")
		}
		body, _, ok := strings.Cut(rest, "`;")
		if !ok {
			t.Fatal("unterminated template")
		}
		files[name+".tpl"] = []byte(body)
	}
	r, selections := gitRenderer(t, map[string]map[string][]byte{"inline": files})
	req := gitRequest(selections["inline"])
	manifests, err := r.Render(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	primary, service := false, false
	for _, m := range manifests {
		primary = primary || m.Kind == "Deployment"
		service = service || m.Kind == "Service"
	}
	if !primary || !service {
		t.Fatal("missing Deployment/Service")
	}
	req.Module.Spec.Service = nil
	manifests, err = r.Render(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range manifests {
		if m.Kind == "Service" {
			t.Fatal("unexpected Service")
		}
	}
}
