package scorek8s

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/renderbundle"
	"orchestrator/internal/domain/resource"
	"orchestrator/internal/ports/execution"
	"orchestrator/internal/ports/renderregistry"
	"orchestrator/internal/renderengine"
)

func TestMain(m *testing.M) {
	renderengine.RunWorkerIfRequested()
	os.Exit(m.Run())
}

// memoryBundles serves bundles by content digest, like the verified cache.
type memoryBundles map[string]renderengine.Bundle

func (m memoryBundles) Load(_ context.Context, pin resource.RenderBundle) (renderengine.Bundle, error) {
	b, ok := m[pin.Digest]
	if !ok {
		return renderengine.Bundle{}, renderregistry.ErrBundleUnavailable
	}
	return b, nil
}

func readBundle(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		content, err := os.ReadFile(p)
		files[filepath.ToSlash(rel)] = content
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

const standardDir = "../../../render-bundles/standard"

// gitRenderer returns a real score-k8s renderer serving the given trees and
// the plan selection of each, keyed by name.
func gitRenderer(t *testing.T, trees map[string]map[string][]byte) (*Renderer, map[string]resource.RenderingSelection) {
	t.Helper()
	r := installed(t)
	store := memoryBundles{}
	selections := map[string]resource.RenderingSelection{}
	for name, files := range trees {
		bundle, err := renderengine.Validate(files)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		digest := registry.Tree(files).Digest()
		store[digest] = bundle
		record := renderbundle.Bundle{ID: "00000000-0000-4000-8000-00000000000" + string(rune('1'+len(selections))), OrganizationKey: "org",
			Source:         renderbundle.Source{Kind: "git", URL: "https://example.com/r.git", Path: ".", Revision: strings.Repeat("a", 40), ContentSHA256: digest},
			EngineContract: renderbundle.EngineContract, RendererVersion: Version, RendererBinary: r.BinaryDigest(), Contract: bundle.Contract}
		inputs, err := renderbundle.Effective(bundle.Contract.InputsSchema, nil)
		if err != nil {
			t.Fatal(err)
		}
		hash, _ := renderbundle.InputsHash(inputs)
		selections[name] = resource.RenderingSelection{DefinitionKey: "render", DefinitionHash: "pin", DriverType: resource.DriverScoreK8s, Bundle: resource.GitRenderPin(record), Inputs: inputs, InputsHash: hash}
	}
	r.SetGitBundles(store, renderengine.Executor{})
	return r, selections
}

// semantic drops generator-only differences that do not change the applied
// workload: env order, empty rollout strategy and the score-k8s source
// annotation.
func semantic(t *testing.T, manifests []execution.Manifest) map[string]any {
	t.Helper()
	out := map[string]any{}
	for _, m := range manifests {
		raw, _ := json.Marshal(m.Object)
		var object map[string]any
		_ = json.Unmarshal(raw, &object)
		if m.Kind == "Deployment" {
			spec := object["spec"].(map[string]any)
			if s, ok := spec["strategy"].(map[string]any); ok && len(s) == 0 {
				delete(spec, "strategy")
			}
			pod := spec["template"].(map[string]any)["spec"].(map[string]any)
			for _, c := range pod["containers"].([]any) {
				container := c.(map[string]any)
				if env, ok := container["env"].([]any); ok {
					sort.Slice(env, func(i, j int) bool {
						return env[i].(map[string]any)["name"].(string) < env[j].(map[string]any)["name"].(string)
					})
				}
			}
		}
		out[m.Kind+"/"+m.Name] = object
	}
	return out
}

func gitRequest(selection resource.RenderingSelection) execution.RenderRequest {
	req := request(&Renderer{})
	req.Selection = selection
	return req
}

// The standard bundle reproduces the native semantics for every supported
// Score field, the VSO path and the legacy Agent path, without a native render.
func TestStandardBundleMatchesNativeSemantics(t *testing.T) {
	r, selections := gitRenderer(t, map[string]map[string][]byte{"standard": readBundle(t, standardDir)})
	agent := func(req *execution.RenderRequest) {
		req.ConfigSecretName, req.ConfigSecretKeys = "", nil
		req.Vault = &execution.VaultInjection{Address: "http://vault:8200", Role: "backend", ServiceAccount: "backend-vault", Bindings: map[string]map[string]string{"main": {"API_KEY": "kv2://kv/app/revision/key"}}}
		req.Module.Spec.Containers["sidecar"] = environment.Container{Image: "busybox:1", Args: []string{"sleep", "3600"}}
	}
	cases := map[string]func(*execution.RenderRequest){
		"vso":   func(*execution.RenderRequest) {},
		"agent": agent,
		"no service, no replicas, requests only": func(req *execution.RenderRequest) {
			req.Module.Spec.Service, req.Module.Spec.Replicas, req.ImagePullSecret = nil, nil, ""
			c := req.Module.Spec.Containers["main"]
			c.Resources = &environment.ContainerResourceRequirements{Requests: &environment.ComputeResources{CPU: "250m"}}
			c.ReadinessProbe, c.LivenessProbe = nil, &environment.Probe{Path: "/live", Port: 9090}
			req.Module.Spec.Containers["main"] = c
		},
		"empty and dollar values": func(req *execution.RenderRequest) {
			req.PlainEnv = map[string]map[string]string{"main": {"EMPTY": "", "DOLLAR": "a$b", "BRACE": "${literal}", "TEMPLATE": "{{ fail \"x\" }}", "MULTI": "a\nb"}}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			req := gitRequest(selections["standard"])
			mutate(&req)
			got, err := r.Render(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			want, err := kubernetes.NewRenderer().Render(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if a, b := semantic(t, got), semantic(t, want); !reflect.DeepEqual(a, b) {
				ja, _ := json.MarshalIndent(a, "", " ")
				jb, _ := json.MarshalIndent(b, "", " ")
				t.Fatalf("git render differs from native semantics\ngit: %s\nnative: %s", ja, jb)
			}
			for _, m := range got {
				if m.Kind == "Secret" {
					continue
				}
				raw, _ := json.Marshal(m.Object)
				if strings.Contains(string(raw), "database-secret-sentinel") {
					t.Fatal("raw secret leaked outside the platform Secret")
				}
			}
		})
	}
}

// edit returns a copy of the standard tree with one file replaced.
func edit(t *testing.T, name, old, replacement string) map[string][]byte {
	t.Helper()
	files := readBundle(t, standardDir)
	if old != "" && !strings.Contains(string(files[name]), old) {
		t.Fatalf("%s does not contain %q", name, old)
	}
	if old == "" {
		files[name] = []byte(replacement)
	} else {
		files[name] = []byte(strings.Replace(string(files[name]), old, replacement, 1))
	}
	return files
}

func deployment(t *testing.T, manifests []execution.Manifest) map[string]any {
	t.Helper()
	for _, m := range manifests {
		if m.Kind == "Deployment" {
			return m.Object
		}
	}
	t.Fatal("no Deployment")
	return nil
}

func container(d map[string]any, index int) map[string]any {
	return d["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)["containers"].([]any)[index].(map[string]any)
}

// Changing each stage's source changes the rendered outcome without a Go
// change: defaults, normalization (a separately authored normalizer),
// provisioner outputs and patches.
func TestEveryStageSourceIsConsumed(t *testing.T) {
	standard := readBundle(t, standardDir)
	defaultsB := edit(t, "defaults.yaml", "cpu: 10m", "cpu: 30m")
	defaultsB["defaults.yaml"] = []byte(strings.Replace(string(defaultsB["defaults.yaml"]), "podAnnotations: {}", "podAnnotations:\n  example.com/render-policy: b", 1))
	patchB := edit(t, "defaults.yaml", "podSecurityContext: {}", "podSecurityContext:\n  seccompProfile:\n    type: RuntimeDefault")
	provisionerB := edit(t, "provisioners/bindings.provisioners.yaml", `{{ $entry.value | quote }}`, `{{ printf "via-b:%s" $entry.value | quote }}`)
	normalizerB := edit(t, "normalize/score.tpl", "", `{{- /* A separately authored normalizer: one container, explicit 77m request, no bindings. */ -}}
{{- $c := get .Input.workload.spec.containers "main" -}}
{{- $out := dict "image" $c.image "command" $c.command "args" $c.args "resources" (dict "requests" (dict "cpu" "77m" "memory" "64Mi") "limits" $c.resources.limits) -}}
{{- $out = merge $out (dict "readinessProbe" (dict "httpGet" (dict "path" $c.readinessProbe.path "port" $c.readinessProbe.port))) -}}
{{- $vars := dict -}}{{- $params := dict -}}
{{- range $b := get .Input.bindings "main" -}}{{- $_ := set $vars $b.name (printf "${resources.vb.main.%s}" $b.name) -}}{{- if eq $b.kind "secret" }}{{ $_ := set $params $b.name (dict "secret" true "secretName" $b.secretName "secretKey" $b.secretKey) }}{{ else }}{{ $_ := set $params $b.name (dict "secret" false "value" (escapeScore $b.value)) }}{{ end -}}{{- end -}}
{{- $_ := set $out "variables" $vars -}}
{{- toJson (dict "apiVersion" "score.dev/v1b1" "metadata" (dict "name" .Input.identity.workload "annotations" (dict "example.com/normalized-by" "b")) "containers" (dict "main" $out) "service" (dict "ports" (dict "http" (dict "port" 8080 "targetPort" 8080 "protocol" "TCP"))) "resources" (dict "vb" (dict "type" "orchestrator-bindings" "params" (dict "main" $params)))) -}}`)
	patchSource := edit(t, "patches/deployment.tpl", `- op: delete
  path: "{{ $i }}.metadata.annotations"`, `- op: delete
  path: "{{ $i }}.metadata.annotations"
- op: set
  path: "{{ $i }}.spec.revisionHistoryLimit"
  value: 3`)
	r, selections := gitRenderer(t, map[string]map[string][]byte{"standard": standard, "defaults": defaultsB, "patch": patchB, "provisioner": provisionerB, "normalizer": normalizerB, "patchsrc": patchSource})
	render := func(name string, mutate func(*execution.RenderRequest)) map[string]any {
		req := gitRequest(selections[name])
		req.ConfigSecretName, req.ConfigSecretKeys = "", nil
		// No explicit CPU: the fallback comes from the bundle.
		c := req.Module.Spec.Containers["main"]
		c.Resources = &environment.ContainerResourceRequirements{Limits: &environment.ComputeResources{Memory: "64Mi"}}
		req.Module.Spec.Containers["main"] = c
		if mutate != nil {
			mutate(&req)
		}
		manifests, err := r.Render(context.Background(), req)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return deployment(t, manifests)
	}
	requests := func(d map[string]any) map[string]any {
		return container(d, 0)["resources"].(map[string]any)["requests"].(map[string]any)
	}
	if got := requests(render("standard", nil)); got["cpu"] != "10m" || got["memory"] != "64Mi" {
		t.Fatalf("standard fallback/same-field limit: %v", got)
	}
	b := render("defaults", nil)
	if got := requests(b); got["cpu"] != "30m" || got["memory"] != "64Mi" {
		t.Fatalf("defaults change not consumed: %v", got)
	}
	if b["spec"].(map[string]any)["template"].(map[string]any)["metadata"].(map[string]any)["annotations"].(map[string]any)["example.com/render-policy"] != "b" {
		t.Fatal("annotation default not consumed")
	}
	explicit := render("defaults", func(req *execution.RenderRequest) {
		c := req.Module.Spec.Containers["main"]
		c.Resources = &environment.ContainerResourceRequirements{Requests: &environment.ComputeResources{CPU: "5m"}}
		req.Module.Spec.Containers["main"] = c
	})
	if got := requests(explicit); got["cpu"] != "5m" || got["memory"] != "32Mi" {
		t.Fatalf("explicit request must win and memory must use the default: %v", got)
	}
	pod := render("patch", nil)["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)
	if !reflect.DeepEqual(pod["securityContext"], map[string]any{"seccompProfile": map[string]any{"type": "RuntimeDefault"}}) {
		t.Fatalf("patch securityContext not consumed: %v", pod["securityContext"])
	}
	// Provisioner outputs feed the environment bindings; a provisioner that
	// changes an approved value is consumed and then rejected by the guard.
	req := gitRequest(selections["provisioner"])
	req.ConfigSecretName, req.ConfigSecretKeys = "", nil
	if _, err := r.Render(context.Background(), req); !errors.Is(err, renderregistry.ErrRenderFailed) {
		t.Fatalf("changed provisioner output must be consumed and guarded: %v", err)
	}
	n := render("normalizer", nil)
	if got := requests(n); got["cpu"] != "77m" || got["memory"] != "64Mi" {
		t.Fatalf("alternative normalizer not consumed: %v", got)
	}
	if n["spec"].(map[string]any)["template"].(map[string]any)["metadata"].(map[string]any)["annotations"].(map[string]any)["example.com/normalized-by"] != "b" {
		t.Fatal("alternative normalizer Score annotations not consumed")
	}
	if got := render("patchsrc", nil)["spec"].(map[string]any)["revisionHistoryLimit"]; got != 3.0 {
		t.Fatalf("patch template change not consumed: %v", got)
	}
}

// Explicit Score intent is protected: a normalizer that overrides an explicit
// request is rejected rather than silently applied.
func TestGuardRejectsOverridingExplicitIntent(t *testing.T) {
	override := edit(t, "normalize/score.tpl", `{{- $value := get $declaredRequests $field | default (get $declaredLimits $field) | default (get $d.resources.requests $field) -}}`, `{{- $value := get $d.resources.requests $field -}}`)
	identity := edit(t, "patches/service.tpl", `value: {{ toJson $in.identity.selector }}`, `value: {"app.kubernetes.io/name": "other"}`)
	secretRef := edit(t, "provisioners/bindings.provisioners.yaml", `encodeSecretRef $entry.secretName $entry.secretKey`, `encodeSecretRef "foreign-secret" $entry.secretKey`)
	sidecar := edit(t, "patches/deployment.tpl", `{{- range $ci, $c := $m.spec.template.spec.containers }}`, `
- op: set
  path: "{{ $i }}.spec.template.spec.initContainers"
  value: [{"name": "x", "image": "busybox"}]
{{- range $ci, $c := $m.spec.template.spec.containers }}`)
	vault := edit(t, "patches/deployment.tpl", `- op: delete
  path: "{{ $i }}.metadata.annotations"`, `- op: set
  path: "{{ $i }}.metadata.annotations"
  value: {"vault.hashicorp.com/role": "admin"}`)
	r, selections := gitRenderer(t, map[string]map[string][]byte{"override": override, "identity": identity, "secret": secretRef, "sidecar": sidecar, "vault": vault})
	for name, selection := range selections {
		req := gitRequest(selection)
		req.ConfigSecretName, req.ConfigSecretKeys = "", nil
		c := req.Module.Spec.Containers["main"]
		c.Resources = &environment.ContainerResourceRequirements{Requests: &environment.ComputeResources{CPU: "250m"}}
		req.Module.Spec.Containers["main"] = c
		if _, err := r.Render(context.Background(), req); !errors.Is(err, renderregistry.ErrRenderFailed) {
			t.Fatalf("%s: want invariant failure, got %v", name, err)
		}
	}
}

// A cached bundle is required: no Git fetch or fallback at render.
func TestMissingCacheFailsClosed(t *testing.T) {
	r, selections := gitRenderer(t, map[string]map[string][]byte{"standard": readBundle(t, standardDir)})
	r.SetGitBundles(memoryBundles{}, renderengine.Executor{})
	if _, err := r.Render(context.Background(), gitRequest(selections["standard"])); !errors.Is(err, renderregistry.ErrBundleUnavailable) {
		t.Fatalf("want unavailable, got %v", err)
	}
	selection := selections["standard"]
	selection.Inputs = map[string]any{"defaultReplicas": 3.0, "runAsNonRoot": false}
	r2, _ := gitRenderer(t, map[string]map[string][]byte{"standard": readBundle(t, standardDir)})
	if err := r2.ValidateSelection(context.Background(), selection); !errors.Is(err, renderregistry.ErrBundleUnavailable) {
		t.Fatalf("changed inputs without hash must fail: %v", err)
	}
}

// Provisioner priority is the bundle.yaml array order (first entry wins),
// proven with the real CLI in both orders.
func TestProvisionerPriorityFollowsManifestOrder(t *testing.T) {
	second := `- uri: template://test/second
  type: orchestrator-bindings
  outputs: |
    {{- range $container, $entries := .Params }}
    {{ $container | quote }}:
    {{- range $name, $entry := $entries }}
      {{ $name | quote }}: {{ if $entry.secret }}{{ encodeSecretRef $entry.secretName $entry.secretKey | quote }}{{ else }}{{ printf "second:%s" $entry.value | quote }}{{ end }}
    {{- end }}
    {{- end }}
`
	first := readBundle(t, standardDir)
	first["provisioners/second.provisioners.yaml"] = []byte(second)
	first["bundle.yaml"] = []byte(strings.Replace(string(first["bundle.yaml"]), "  - provisioners/bindings.provisioners.yaml\n", "  - provisioners/bindings.provisioners.yaml\n  - provisioners/second.provisioners.yaml\n", 1))
	reversed := readBundle(t, standardDir)
	reversed["provisioners/second.provisioners.yaml"] = []byte(second)
	reversed["bundle.yaml"] = []byte(strings.Replace(string(reversed["bundle.yaml"]), "  - provisioners/bindings.provisioners.yaml\n", "  - provisioners/second.provisioners.yaml\n  - provisioners/bindings.provisioners.yaml\n", 1))
	r, selections := gitRenderer(t, map[string]map[string][]byte{"first": first, "reversed": reversed})
	render := func(name string) error {
		req := gitRequest(selections[name])
		req.ConfigSecretName, req.ConfigSecretKeys = "", nil
		_, err := r.Render(context.Background(), req)
		return err
	}
	// The approved bindings come from the first listed provisioner; when the
	// value-changing provisioner is listed first it wins and is rejected.
	if err := render("first"); err != nil {
		t.Fatalf("first listed provisioner must win: %v", err)
	}
	if err := render("reversed"); !errors.Is(err, renderregistry.ErrRenderFailed) {
		t.Fatalf("reordered provisioners must change the winner: %v", err)
	}
}
