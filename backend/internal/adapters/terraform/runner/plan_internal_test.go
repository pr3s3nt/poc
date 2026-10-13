package runner

import "testing"

func TestStateResourcesCarryProviderUIDs(t *testing.T) {
	state := []byte(`{"values":{"root_module":{"resources":[
	 {"address":"kubernetes_namespace_v1.ns","mode":"managed","type":"kubernetes_namespace_v1","provider_name":"registry.terraform.io/hashicorp/kubernetes","values":{"id":"orc-ns","metadata":[{"name":"orc-ns","uid":"uid-1"}]}},
	 {"address":"terraform_data.x","mode":"managed","type":"terraform_data","provider_name":"terraform.io/builtin/terraform","values":{"id":"abc"}}],
	 "child_modules":[{"resources":[{"address":"module.m.kubernetes_secret_v1.s","mode":"managed","type":"kubernetes_secret_v1","provider_name":"p","values":{"id":"ns/s","metadata":{"uid":"uid-2"}}}]}]}}}`)
	got, err := parseStateResources(state)
	if err != nil || len(got) != 3 {
		t.Fatalf("resources %+v: %v", got, err)
	}
	uids := map[string]string{}
	for _, r := range got {
		uids[r.Address] = r.UID
	}
	if uids["kubernetes_namespace_v1.ns"] != "uid-1" || uids["module.m.kubernetes_secret_v1.s"] != "uid-2" || uids["terraform_data.x"] != "" {
		t.Fatalf("uids %v", uids)
	}
}
