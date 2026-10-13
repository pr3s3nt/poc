package terraform_test

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"orchestrator/internal/adapters/store"
	"orchestrator/internal/adapters/terraform"
	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/adapters/terraform/registry/registrytest"
	"orchestrator/internal/adapters/terraform/runner/runnertest"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/seed"
)

// Image digests verified against Docker Hub on 2026-10-13: the multi-platform
// OCI index digests of the pinned tags. The modules must default to exactly
// these references.
const (
	redisImage    = "redis:7.4.7-alpine@sha256:02f2cc4882f8bf87c79a220ac958f58c700bdec0dfb9b9ea61b62fb0e8f1bfcf"
	postgresImage = "postgres:16.14-alpine@sha256:57c72fd2a128e416c7fcc499958864df5301e940bca0a56f58fddf30ffc07777"
	// absentDigest is the SHA-256 of a fixed text, verified to answer 404 at
	// the registry: the E02 controlled fault pins a well-formed reference whose
	// digest does not exist.
	absentDigest = "095bac20aafb160ab173e6fed571218d4feba5644f7189f3480fd1b07b52ca03"
)

var k8sModules = []string{"k8s-namespace", "redis-k8s", "postgres-k8s"}

func redisType() resource.Type {
	return resource.Type{Key: "redis", Outputs: []resource.OutputField{
		{Name: "host", Type: "string", Required: true}, {Name: "port", Type: "number", Required: true},
		{Name: "username", Type: "string", Required: true}, {Name: "password", Type: "string", Required: true, Secret: true},
		{Name: "tls", Type: "bool", Required: true},
	}}
}

func seedType(t *testing.T, key string) resource.Type {
	t.Helper()
	for _, typ := range seed.ResourceTypes() {
		if typ.Key == key {
			return typ
		}
	}
	t.Fatalf("seed type %s missing", key)
	return resource.Type{}
}

func bundles(t *testing.T) map[string]tfmodule.Bundle {
	t.Helper()
	svc := registrytest.Open(t, store.New())
	refs, err := registry.PackagedRefs()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]tfmodule.Bundle{}
	for _, id := range k8sModules {
		bundle, err := svc.Bundle(t.Context(), refs[id])
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		out[id] = bundle
	}
	return out
}

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func equal(a, b []string) bool { return strings.Join(sorted(a), ",") == strings.Join(sorted(b), ",") }

// Registry schemas: inputs, executor-owned names, outputs, provider pins and
// the Resource Types each module serves.
func TestKubernetesModuleRegistryContracts(t *testing.T) {
	all := bundles(t)
	release, _ := registry.DefaultRelease()
	var kube registry.ReleaseProvider
	for _, p := range release.Providers {
		if strings.HasSuffix(p.Source, "/hashicorp/kubernetes") {
			kube = p
		}
	}
	if kube.Version != "2.38.0" || kube.Binding != tfmodule.BindingKubernetes || len(kube.Hashes) < 4 {
		t.Fatalf("kubernetes provider pin %+v", kube)
	}
	type want struct {
		types, owned []string
		inputs       map[string]string // name -> type
		required     []string
		outputs      map[string]string // name -> static type ("" deferred)
		sensitive    []string
	}
	wants := map[string]want{
		"k8s-namespace": {types: []string{"k8s-namespace"}, owned: []string{"tags"},
			inputs: map[string]string{"name": "string", "tags": "map(string)"}, required: []string{"name"},
			outputs: map[string]string{"name": ""}},
		"redis-k8s": {types: []string{"redis"}, owned: []string{"password", "tags"},
			inputs: map[string]string{"namespace": "string", "image": "string", "username": "string", "storage": "string", "storage_class": "string",
				"labels": "map(string)", "annotations": "map(string)", "password": "string", "tags": "map(string)"},
			required: []string{"namespace", "password", "tags"},
			outputs:  map[string]string{"host": "", "port": "number", "username": "", "tls": "bool", "password": ""}, sensitive: []string{"password"}},
		"postgres-k8s": {types: []string{"postgres"}, owned: []string{"password", "tags"},
			inputs: map[string]string{"namespace": "string", "database": "string", "username": "string", "image": "string", "storage": "string", "storage_class": "string",
				"labels": "map(string)", "annotations": "map(string)", "password": "string", "tags": "map(string)"},
			required: []string{"namespace", "database", "username", "password", "tags"},
			outputs:  map[string]string{"host": "", "port": "number", "database": "", "username": "", "password": ""}, sensitive: []string{"password"}},
	}
	for id, w := range wants {
		b := all[id]
		if !equal(b.Contract.ResourceTypes, w.types) || !equal(b.Contract.ExecutorOwned, w.owned) || len(b.Contract.ExecutorOutputs) != 0 {
			t.Errorf("%s types %v owned %v executor outputs %v", id, b.Contract.ResourceTypes, b.Contract.ExecutorOwned, b.Contract.ExecutorOutputs)
		}
		if len(b.Providers) != 1 || b.Providers["kubernetes"].Binding != tfmodule.BindingKubernetes || b.Providers["kubernetes"].Version != "2.38.0" {
			t.Errorf("%s providers %+v", id, b.Providers)
		}
		if len(b.Contract.Transitive) != 0 || b.Contract.SourceKind != tfmodule.SourceKindPackaged {
			t.Errorf("%s transitive %v kind %s", id, b.Contract.Transitive, b.Contract.SourceKind)
		}
		if len(b.Contract.Inputs) != len(w.inputs) {
			t.Errorf("%s inputs %v", id, b.Contract.Inputs)
		}
		for name, typ := range w.inputs {
			if got := b.Contract.Inputs[name].Type; got != typ {
				t.Errorf("%s input %s type %q, want %q", id, name, got, typ)
			}
		}
		var required []string
		for name, in := range b.Contract.Inputs {
			if in.Required {
				required = append(required, name)
			}
		}
		if !equal(required, w.required) {
			t.Errorf("%s required %v, want %v", id, required, w.required)
		}
		if len(b.Contract.Outputs) != len(w.outputs) {
			t.Errorf("%s outputs %v", id, b.Contract.Outputs)
		}
		for name, typ := range w.outputs {
			out, ok := b.Contract.Outputs[name]
			if !ok || out.Type != typ || out.Sensitive != contains(w.sensitive, name) {
				t.Errorf("%s output %s = %+v, want type %q sensitive %v", id, name, out, typ, contains(w.sensitive, name))
			}
		}
		// A credential never has a default, and the executor owns it.
		for name, in := range b.Contract.Inputs {
			if (name == "password" || in.Sensitive) && (in.HasDefault || !in.Sensitive) {
				t.Errorf("%s secret input %s must be sensitive without a default", id, name)
			}
		}
	}
	// The image defaults are the verified digests.
	if all["redis-k8s"].Contract.Inputs["image"].Default != redisImage || all["postgres-k8s"].Contract.Inputs["image"].Default != postgresImage {
		t.Fatal("image defaults are not the verified digest pins")
	}
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

// Output mappings: every Resource Type output is covered by a same-named
// module output of a compatible type and the secret classification only grows.
func TestKubernetesModuleOutputMappings(t *testing.T) {
	all := bundles(t)
	cases := []struct {
		module string
		typ    resource.Type
		values map[string]any
	}{
		{"redis-k8s", redisType(), map[string]any{"host": "redis-x.ns.svc", "port": float64(6379), "username": "app", "password": "p", "tls": false}},
		{"postgres-k8s", seedType(t, "postgres"), map[string]any{"host": "postgres-x.ns.svc", "port": float64(5432), "database": "acceptance", "username": "app", "password": "p"}},
		{"k8s-namespace", seedType(t, "k8s-namespace"), map[string]any{"name": "orch-v2-test-run"}},
	}
	for _, c := range cases {
		b := all[c.module]
		for _, field := range c.typ.Outputs {
			out, ok := b.Contract.Outputs[field.Name]
			if !ok || !tfmodule.OutputCompatible(field.Type, out.Type) {
				t.Errorf("%s output %s (%s) does not satisfy the %s Type", c.module, field.Name, out.Type, c.typ.Key)
			}
			if field.Secret && !tfmodule.EffectiveSecret(field.Secret, out.Sensitive) {
				t.Errorf("%s output %s is not secret", c.module, field.Name)
			}
			if field.Secret != out.Sensitive {
				t.Errorf("%s output %s sensitive=%v differs from the Type secret=%v", c.module, field.Name, out.Sensitive, field.Secret)
			}
		}
		if err := c.typ.ValidateOutputs(c.values); err != nil {
			t.Errorf("%s sample outputs: %v", c.module, err)
		}
	}
	// PostgreSQL params keep the current contract and map to string variables.
	pg := seedType(t, "postgres")
	for _, in := range pg.Inputs {
		module := all["postgres-k8s"].Contract.Inputs[in.Name]
		if !module.Required || !tfmodule.InputCompatible(in.Type, module.Type) {
			t.Errorf("postgres param %s does not map to a required compatible module variable", in.Name)
		}
	}
	// Redis has no params: nothing user-facing selects an image or module.
	if len(redisType().Inputs) != 0 {
		t.Fatal("the Redis Type must stay parameterless")
	}
}

func moduleSource(t *testing.T, id string) string {
	t.Helper()
	data, err := fs.ReadFile(terraform.PackagedModules(), id+"/main.tf")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

var resourceBlock = regexp.MustCompile(`(?m)^resource "([a-z0-9_]+)"`)

// Static safety of the module source: resource allowlist, state-owned
// children, no wait in provider create, no credential in metadata.
func TestKubernetesModuleSourceSafety(t *testing.T) {
	allowed := map[string][]string{
		"k8s-namespace": {"kubernetes_namespace_v1"},
		"redis-k8s":     {"kubernetes_persistent_volume_claim_v1", "kubernetes_secret_v1", "kubernetes_service_v1", "kubernetes_stateful_set_v1"},
		"postgres-k8s":  {"kubernetes_persistent_volume_claim_v1", "kubernetes_secret_v1", "kubernetes_service_v1", "kubernetes_stateful_set_v1"},
	}
	for _, id := range k8sModules {
		src := moduleSource(t, id)
		var types []string
		for _, m := range resourceBlock.FindAllStringSubmatch(src, -1) {
			types = append(types, m[1])
		}
		if !equal(types, allowed[id]) {
			t.Errorf("%s resources %v, want %v (no cluster operators, storage classes or ingress)", id, types, allowed[id])
		}
		for _, banned := range []string{"local-exec", "remote-exec", "kubernetes_storage_class", "kubernetes_manifest", "helm_", "kubernetes_ingress", "kubernetes_cluster_role", "kubernetes_namespace", "terraform_remote_state", "provisioner"} {
			if banned == "kubernetes_namespace" && id == "k8s-namespace" {
				continue
			}
			if strings.Contains(src, banned) {
				t.Errorf("%s contains %q", id, banned)
			}
		}
		// Names derive from the stable instance tag, never from a run.
		if strings.Contains(src, "orchestrator-created-by-run") || strings.Contains(src, "timestamp()") || strings.Contains(src, "uuid()") || strings.Contains(src, "random_") {
			t.Errorf("%s derives something from a run or randomness", id)
		}
	}
	for _, id := range []string{"redis-k8s", "postgres-k8s"} {
		src := moduleSource(t, id)
		for _, want := range []string{"wait_for_rollout = false", "wait_until_bound = false", "automount_service_account_token  = false", "allow_privilege_escalation = false", "readiness_probe", "secret_key_ref"} {
			if !strings.Contains(src, want) {
				t.Errorf("%s misses %q", id, want)
			}
		}
		// The volume is a state-owned claim; a template claim has no owner and
		// would block namespace cleanup.
		if strings.Contains(src, "volume_claim_template") {
			t.Errorf("%s uses a volumeClaimTemplate", id)
		}
		// var.password is used only for the Secret data and the sensitive output.
		for n, line := range strings.Split(src, "\n") {
			trim := strings.TrimSpace(line)
			if strings.HasPrefix(trim, "#") || !strings.Contains(trim, "var.password") {
				continue
			}
			if trim != "password = var.password" && trim != "value     = var.password" {
				t.Errorf("%s:%d unexpected password use: %s", id, n+1, trim)
			}
		}
	}
	// Only the Secret object holds credentials: labels/annotations locals never
	// reference var.password.
	for _, id := range []string{"redis-k8s", "postgres-k8s"} {
		src := moduleSource(t, id)
		for _, local := range []string{"labels", "annotations", "platform_labels", "platform_annotations"} {
			re := regexp.MustCompile(`(?s)\b` + local + `\s*=\s*(?:merge\(|\{)[^\n]*`)
			for _, m := range re.FindAllString(src, -1) {
				if strings.Contains(m, "var.password") {
					t.Errorf("%s %s uses the password", id, local)
				}
			}
		}
	}
}

// ---- actual Terraform CLI: init with the pinned readonly lock, validate, and
// plan against a fake endpoint. No apply and no cluster: the provider is
// configured with an unreachable host and plan never contacts it.

func providerCache(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("orchestrator-tf-provider-cache-%d", os.Getuid()))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func tf(t *testing.T, dir, bin string, args ...string) ([]byte, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "TF_CLI_CONFIG_FILE=/dev/null", "TF_IN_AUTOMATION=1", "TF_INPUT=0",
		"TF_PLUGIN_CACHE_DIR=" + providerCache(t), "CHECKPOINT_DISABLE=1",
		"KUBE_HOST=https://127.0.0.1:1", "KUBE_TOKEN=unused", "KUBE_LOAD_CONFIG_FILE=false",
	}
	return cmd.CombinedOutput()
}

// prepared copies one packaged module into a private directory with the
// readonly lock the registry renders and runs init -lockfile=readonly.
func prepared(t *testing.T, id string) (dir, bin string) {
	t.Helper()
	bin = runnertest.HostTerraform(t)
	dir = t.TempDir()
	src, err := fs.ReadFile(terraform.PackagedModules(), id+"/main.tf")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), src, 0o600); err != nil {
		t.Fatal(err)
	}
	b, ok := bundles(t)[id]
	if !ok {
		b = awsBundles(t)[id]
	}
	if err := os.WriteFile(filepath.Join(dir, ".terraform.lock.hcl"), registry.LockFile(b.Providers), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := tf(t, dir, bin, "init", "-input=false", "-lockfile=readonly", "-no-color"); err != nil {
		if os.Getenv("REQUIRE_TERRAFORM") == "1" {
			t.Fatalf("%s: init with the readonly lock failed: %s", id, tail(out))
		}
		t.Skipf("%s: provider could not be installed (offline?)", id)
	}
	return dir, bin
}

func tail(out []byte) string {
	s := string(out)
	if len(s) > 1500 {
		s = s[len(s)-1500:]
	}
	return s
}

func TestKubernetesModulesInitValidateReadonlyLock(t *testing.T) {
	for _, id := range k8sModules {
		t.Run(id, func(t *testing.T) {
			dir, bin := prepared(t, id)
			if out, err := tf(t, dir, bin, "validate", "-no-color"); err != nil {
				t.Fatalf("validate: %s", tail(out))
			}
			// The lock file was not rewritten by init.
			got, _ := os.ReadFile(filepath.Join(dir, ".terraform.lock.hcl"))
			b := bundles(t)[id]
			if string(got) != string(registry.LockFile(b.Providers)) {
				t.Fatal("init changed the pinned lock file")
			}
		})
	}
}

type planned struct {
	Resources []struct {
		Type   string         `json:"type"`
		Name   string         `json:"name"`
		Values map[string]any `json:"values"`
	}
	Outputs map[string]struct {
		Sensitive bool `json:"sensitive"`
		Value     any  `json:"value"`
	}
}

func plan(t *testing.T, dir, bin string, vars map[string]any) (planned, string, error) {
	t.Helper()
	data, _ := json.Marshal(vars)
	if err := os.WriteFile(filepath.Join(dir, "vars.tfvars.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := tf(t, dir, bin, "plan", "-input=false", "-lock=false", "-refresh=false", "-no-color", "-var-file=vars.tfvars.json", "-out=plan.bin"); err != nil {
		return planned{}, string(out), err
	}
	raw, err := tf(t, dir, bin, "show", "-json", "plan.bin")
	if err != nil {
		t.Fatalf("show: %s", tail(raw))
	}
	var doc struct {
		Planned struct {
			Root struct {
				Resources []struct {
					Type   string         `json:"type"`
					Name   string         `json:"name"`
					Values map[string]any `json:"values"`
				} `json:"resources"`
			} `json:"root_module"`
			Outputs map[string]struct {
				Sensitive bool `json:"sensitive"`
				Value     any  `json:"value"`
			} `json:"outputs"`
		} `json:"planned_values"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return planned{Resources: doc.Planned.Root.Resources, Outputs: doc.Planned.Outputs}, string(raw), nil
}

func tags(instance, run string) map[string]any {
	return map[string]any{"orchestrator-instance": instance, "orchestrator-managed": "true", "orchestrator-created-by-run": run, "expires-at": "2026-10-13T20:30:00Z", "project": "poc"}
}

func metadata(values map[string]any) (labels, annotations map[string]any, name string) {
	list, _ := values["metadata"].([]any)
	if len(list) == 0 {
		return nil, nil, ""
	}
	m, _ := list[0].(map[string]any)
	labels, _ = m["labels"].(map[string]any)
	annotations, _ = m["annotations"].(map[string]any)
	name, _ = m["name"].(string)
	return
}

const secretValue = "pw-0123456789abcdef0123456789abcdef"

func TestKubernetesStatefulModulesPlan(t *testing.T) {
	for _, tc := range []struct {
		module, prefix, image string
		port                  float64
		extra                 map[string]any
	}{
		{"redis-k8s", "redis-", redisImage, 6379, map[string]any{}},
		{"postgres-k8s", "postgres-", postgresImage, 5432, map[string]any{"database": "acceptance", "username": "app"}},
	} {
		t.Run(tc.module, func(t *testing.T) {
			dir, bin := prepared(t, tc.module)
			vars := func(instance, run string, more map[string]any) map[string]any {
				v := map[string]any{"namespace": "orch-v2-test-run1", "password": secretValue, "tags": tags(instance, run)}
				for k, x := range tc.extra {
					v[k] = x
				}
				for k, x := range more {
					v[k] = x
				}
				return v
			}
			annotation := map[string]any{"annotations": map[string]any{"example.test/postgres-host": "postgres-aaaaaaaaaaaaaaaa.orch-v2-test-run1.svc"}, "labels": map[string]any{"example.test/tier": "cache"}}
			first, raw, err := plan(t, dir, bin, vars("0123456789abcdef", "run-1", annotation))
			if err != nil {
				t.Fatalf("plan: %s", tail([]byte(raw)))
			}
			types := map[string]map[string]any{}
			for _, r := range first.Resources {
				types[r.Type] = r.Values
			}
			if len(first.Resources) != 4 || types["kubernetes_stateful_set_v1"] == nil || types["kubernetes_persistent_volume_claim_v1"] == nil || types["kubernetes_service_v1"] == nil || types["kubernetes_secret_v1"] == nil {
				t.Fatalf("resources %+v", first.Resources)
			}
			name := tc.prefix + "0123456789abcdef"
			for typ, values := range types {
				labels, annotations, objectName := metadata(values)
				if !strings.HasPrefix(objectName, name) {
					t.Errorf("%s name %q", typ, objectName)
				}
				if labels["orchestrator-instance"] != "0123456789abcdef" || labels["app.kubernetes.io/managed-by"] != "orchestrator" || labels["example.test/tier"] != "cache" {
					t.Errorf("%s labels %v", typ, labels)
				}
				// The nonsecret annotation reaches the real object metadata.
				if annotations["example.test/postgres-host"] != "postgres-aaaaaaaaaaaaaaaa.orch-v2-test-run1.svc" || annotations["expires-at"] != "2026-10-13T20:30:00Z" {
					t.Errorf("%s annotations %v", typ, annotations)
				}
				// A timestamp is an invalid label value: it is only an annotation.
				if _, ok := labels["expires-at"]; ok {
					t.Errorf("%s has an invalid-label tag as a label", typ)
				}
				meta, _ := json.Marshal([]any{labels, annotations})
				if strings.Contains(string(meta), secretValue) {
					t.Errorf("%s metadata holds the credential", typ)
				}
			}
			ss, _ := json.Marshal(types["kubernetes_stateful_set_v1"])
			for _, want := range []string{tc.image, `"wait_for_rollout":false`, `"replicas":"1"`} {
				if !strings.Contains(string(ss), want) {
					t.Errorf("stateful set misses %s", want)
				}
			}
			if strings.Contains(string(ss), secretValue) {
				t.Error("the stateful set holds the credential")
			}
			if first.Outputs["port"].Value != tc.port || !first.Outputs["password"].Sensitive || first.Outputs["host"].Sensitive {
				t.Errorf("outputs %+v", first.Outputs)
			}
			if host, _ := first.Outputs["host"].Value.(string); host != name+".orch-v2-test-run1.svc" {
				t.Errorf("host %q", host)
			}
			// Stable identity: another run ID (retry) keeps every physical name.
			second, _, err := plan(t, dir, bin, vars("0123456789abcdef", "run-2-retry", annotation))
			if err != nil {
				t.Fatal(err)
			}
			for i, r := range second.Resources {
				if _, _, a := metadata(r.Values); func() string { _, _, b := metadata(first.Resources[i].Values); return b }() != a {
					t.Errorf("retry changed the name of %s", r.Type)
				}
			}
			// A different instance never shares a name.
			other, _, err := plan(t, dir, bin, vars("fedcba9876543210", "run-1", annotation))
			if err != nil {
				t.Fatal(err)
			}
			if _, _, a := metadata(other.Resources[0].Values); strings.Contains(a, "0123456789abcdef") {
				t.Errorf("another instance reuses a name: %s", a)
			}

			// E02 fault setting: a well-formed reference with a digest that does
			// not exist is accepted locally (the failure is a real pod pull).
			fault := tc.image[:strings.Index(tc.image, "@")] + "@sha256:" + absentDigest
			if _, raw, err := plan(t, dir, bin, vars("0123456789abcdef", "run-1", map[string]any{"image": fault})); err != nil {
				t.Fatalf("fault digest rejected locally: %s", tail([]byte(raw)))
			}
			// ...while a mutable tag, a missing instance tag and credential-like
			// metadata are refused before any object exists.
			for label, bad := range map[string]map[string]any{
				"mutable tag":     {"image": "redis:latest"},
				"short digest":    {"image": "redis:7@sha256:abc"},
				"secret key":      {"annotations": map[string]any{"example.test/password": "x"}},
				"platform key":    {"labels": map[string]any{"app.kubernetes.io/name": "x"}},
				"bad label value": {"labels": map[string]any{"example.test/tier": "has space"}},
				"no instance tag": {"tags": map[string]any{"orchestrator-managed": "true"}},
				"bad namespace":   {"namespace": "Not_A_Label"},
				"bad storage":     {"storage": "lots"},
			} {
				if _, _, err := plan(t, dir, bin, vars("0123456789abcdef", "run-1", bad)); err == nil {
					t.Errorf("%s was accepted", label)
				}
			}
		})
	}
}

func TestNamespaceModulePlan(t *testing.T) {
	dir, bin := prepared(t, "k8s-namespace")
	good, raw, err := plan(t, dir, bin, map[string]any{"name": "orch-v2-test-run1", "tags": tags("0123456789abcdef", "run-1")})
	if err != nil {
		t.Fatalf("plan: %s", tail([]byte(raw)))
	}
	if len(good.Resources) != 1 || good.Resources[0].Type != "kubernetes_namespace_v1" || good.Outputs["name"].Value != "orch-v2-test-run1" || good.Outputs["name"].Sensitive {
		t.Fatalf("plan %+v", good)
	}
	labels, annotations, name := metadata(good.Resources[0].Values)
	if name != "orch-v2-test-run1" || labels["orchestrator-instance"] != "0123456789abcdef" || annotations["expires-at"] != "2026-10-13T20:30:00Z" || labels["expires-at"] != nil {
		t.Fatalf("namespace metadata %q %v %v", name, labels, annotations)
	}
	// The same logical identity keeps the name across a retry run.
	retry, _, err := plan(t, dir, bin, map[string]any{"name": "orch-v2-test-run1", "tags": tags("0123456789abcdef", "run-2")})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, n := metadata(retry.Resources[0].Values); n != name {
		t.Fatal("retry changed the namespace name")
	}
	for _, bad := range []string{"default", "kube-system", "kube-public", "kube-node-lease", "kube-anything", "Upper", "-x", strings.Repeat("a", 64)} {
		if _, _, err := plan(t, dir, bin, map[string]any{"name": bad}); err == nil {
			t.Errorf("namespace %q accepted", bad)
		}
	}
}
