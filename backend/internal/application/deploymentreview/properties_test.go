package deploymentreview

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/planning"
)

func propertyContract() planning.TerraformContract {
	return planning.TerraformContract{Descriptor: "postgres.default#shared.db", Module: "postgres-k8s", Source: &tfmodule.SourceRef{ContentSHA256: "pinned-digest"}, OutputMapping: map[string]string{"port": "port"}, Inputs: []planning.ContractInput{
		{Name: "database", Type: "string", Source: "resource-input", Value: "catalog"},
		{Name: "namespace", Type: "string", Source: "driver-input", Value: "app-staging"},
		{Name: "storage", Type: "string", Source: "terraform-default", Value: "1Gi"},
		{Name: "password", Type: "string", Source: "driver-input", Value: "never-copy-this-secret"},
	}}
}

func TestNodePropertiesExposeBoundedPublicFactsAndPinnedLiteralPort(t *testing.T) {
	reader := &pinnedReader{files: map[string][]byte{"main.tf": []byte(`output "port" { value = 5432 }`)}}
	contract := propertyContract()
	plan := &planning.Plan{PlanHash: "plan", Graph: planning.Graph{Nodes: []planning.Node{{Descriptor: contract.Descriptor, Kind: planning.NodeResource, ResourceType: "postgres"}}}, Terraform: []planning.TerraformContract{contract}}
	intent := planning.ResourceIntent{Descriptor: contract.Descriptor, Module: &planning.IntentModule{ContentSHA256: "pinned-digest"}}
	got := (Assembler{Sources: reader}).Assemble(context.Background(), plan, []planning.ResourceIntent{intent}, nil)
	data, err := json.Marshal(got.Nodes[0].Properties)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"value":"catalog"`) || !strings.Contains(string(data), `"value":"5432"`) || !strings.Contains(string(data), `"value":"1Gi"`) || strings.Contains(string(data), "never-copy") {
		t.Fatalf("unexpected safe properties: %s", data)
	}
}

func TestNodePropertiesDoNotCopyNilSensitiveUnclassifiedOrDeferredValues(t *testing.T) {
	contract := propertyContract()
	contract.Inputs = []planning.ContractInput{
		{Name: "database", Type: "string", Source: "resource-input", Value: "${resources['private'].outputs.password}"},
		{Name: "namespace", Type: "string", Source: "executor", Value: "executor-secret"},
		{Name: "storage", Type: "string", Source: "driver-input", Value: "https://user:password@host"},
		{Name: "engine_version", Type: "string", Source: "terraform-default", Value: nil},
		{Name: "password", Type: "string", Source: "resource-input", Value: "secret"},
	}
	reader := &pinnedReader{files: map[string][]byte{"main.tf": []byte(`output "port" { value = var.private_port }`)}}
	got := (Assembler{Sources: reader}).properties(context.Background(), planning.Node{ResourceType: "postgres"}, contract)
	data, _ := json.Marshal(got)
	if len(got) != 2 || strings.Contains(string(data), "private") || strings.Contains(string(data), "password") || strings.Contains(string(data), "secret") || strings.Contains(string(data), `"value"`) {
		t.Fatalf("unsafe properties: %s", data)
	}
	for _, fact := range got {
		if fact.Availability != "UNKNOWN" {
			t.Fatalf("deferred value marked known: %#v", fact)
		}
	}
}

func TestNodePortNeverGuessesOrPublishesSecretOrNonliteralOutputs(t *testing.T) {
	for _, expression := range []string{`"5432"`, `tonumber("5432")`, `var.port`, `5432.5`, `0`, `65536`} {
		t.Run(expression, func(t *testing.T) {
			reader := &pinnedReader{files: map[string][]byte{"main.tf": []byte(`output "port" { value = ` + expression + ` }`)}}
			got, ok := (Assembler{Sources: reader}).portProperty(context.Background(), propertyContract())
			if !ok || got.Availability != "UNKNOWN" || got.Value != "" {
				t.Fatalf("unproven port became visible: %#v", got)
			}
		})
	}
	for _, classified := range []bool{false, true} {
		contract := propertyContract()
		body := "output \"port\" {\nvalue = 5432\nsensitive = true\n}"
		if classified {
			contract.SecretOutputs = []string{"port"}
			body = `output "port" { value = 5432 }`
		}
		reader := &pinnedReader{files: map[string][]byte{"main.tf": []byte(body)}}
		if _, ok := (Assembler{Sources: reader}).portProperty(context.Background(), contract); ok {
			t.Fatal("secret output published as a fact")
		}
	}
}
