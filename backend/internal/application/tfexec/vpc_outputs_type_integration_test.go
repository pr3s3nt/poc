package tfexec

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"reflect"
	"testing"
	"time"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/adapters/terraform/registry/registrytest"
	"orchestrator/internal/adapters/terraform/runner"
	"orchestrator/internal/adapters/terraform/runner/runnertest"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/terraformexec"
)

// This uses Terraform 1.9.8's real output JSON and built-in terraform_data
// only. It proves why the VPC module must convert splat tuples to lists before
// exposing subnet IDs, and runs those values through the registry contract
// and executor output validator.
func TestVPCSubnetOutputsConvertRuntimeTupleToContractList(t *testing.T) {
	cli := runnertest.HostTerraform(t)
	versionJSON, err := exec.Command(cli, "version", "-json").Output()
	if err != nil {
		t.Fatalf("read Terraform version: %v", err)
	}
	var version struct {
		TerraformVersion string `json:"terraform_version"`
	}
	if err := json.Unmarshal(versionJSON, &version); err != nil || version.TerraformVersion != "1.9.8" {
		t.Fatalf("Terraform CLI version = %q, want 1.9.8 (decode error %v)", version.TerraformVersion, err)
	}
	ctx := context.Background()
	module := runnertest.Module{Files: map[string]string{"main.tf": `
locals { subnet_ids = ["subnet-a", "subnet-b"] }
resource "terraform_data" "probe" { input = "provider-free" }
output "privateSubnetIds" { value = local.subnet_ids[*] }
output "subnetIds" { value = tolist(local.subnet_ids[*]) }
`}}
	artifact := runnertest.Artifact(t, module, cli)
	t.Cleanup(func() { _ = artifact.Close() })
	session, err := runner.New(runner.Options{}).Open(ctx, terraformexec.SessionRequest{
		Artifact: artifact, LocalState: []byte{}, Variables: map[string]any{}, Deadline: time.Now().Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	if err := session.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	plan, err := session.Plan(ctx, terraformexec.PlanApply)
	if err != nil || !plan.OnlyCreates() || plan.Destructive() {
		t.Fatalf("provider-free plan: %+v %v", plan, err)
	}
	if err := session.Apply(ctx); err != nil {
		t.Fatalf("apply: %v", err)
	}
	actual, err := session.Outputs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if typ, ok := actual["privateSubnetIds"].Type.([]any); !ok || len(typ) != 2 || typ[0] != "tuple" {
		t.Fatalf("Terraform raw splat runtime type = %#v, want tuple", actual["privateSubnetIds"].Type)
	}
	if typ, ok := actual["subnetIds"].Type.([]any); !ok || len(typ) != 2 || typ[0] != "list" || typ[1] != "string" {
		t.Fatalf("Terraform converted runtime type = %#v, want list(string)", actual["subnetIds"].Type)
	}
	wantIDs := []any{"subnet-a", "subnet-b"}
	if !reflect.DeepEqual(actual["subnetIds"].Value, wantIDs) {
		t.Fatalf("converted subnet values = %#v", actual["subnetIds"].Value)
	}

	refs, err := registry.PackagedRefs()
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := registrytest.Open(t, store.New()).Bundle(ctx, refs["vpc"])
	if err != nil {
		t.Fatal(err)
	}
	if bundle.TerraformVersion != version.TerraformVersion {
		t.Fatalf("registry Terraform version = %q, CLI = %q", bundle.TerraformVersion, version.TerraformVersion)
	}
	staticOutput, ok := bundle.Contract.Outputs["privateSubnetIds"]
	if !ok || staticOutput.Type != "list(any)" {
		t.Fatalf("VPC registry output contract = %+v, want list(any)", staticOutput)
	}
	resourceType := resource.Type{Key: "vpc", Outputs: []resource.OutputField{{Name: "privateSubnetIds", Type: "any", Required: true}}}
	mapping := map[string]string{"privateSubnetIds": "privateSubnetIds"}
	raw := map[string]terraformexec.Output{"privateSubnetIds": actual["privateSubnetIds"]}
	if _, err := checkOutputs(bundle, mapping, resourceType, raw, nil); !errors.Is(err, ErrOutputContract) {
		t.Fatalf("raw tuple should fail the list(any) contract; got %v", err)
	}
	converted := map[string]terraformexec.Output{"privateSubnetIds": actual["subnetIds"]}
	checked, err := checkOutputs(bundle, mapping, resourceType, converted, nil)
	if err != nil {
		t.Fatalf("tolist output should satisfy the VPC runtime contract: %v", err)
	}
	if !reflect.DeepEqual(checked.Module["privateSubnetIds"], wantIDs) {
		t.Fatalf("checked output = %#v", checked.Module["privateSubnetIds"])
	}
}
