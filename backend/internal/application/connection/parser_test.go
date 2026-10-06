package connection_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"orchestrator/internal/application/connection"
	ct "orchestrator/internal/application/connection/connectiontest"
)

const secretToken = "sekret-token-value-0123456789"

func TestInspectKubeconfig_ListsSafeMetadataOfValidContexts(t *testing.T) {
	doc := ct.Document(
		ct.TokenContext("kind-a", "https://127.0.0.1:6443", secretToken),
		ct.CertificateContext("prod", "https://prod.example:443/k8s/clusters/c-1"),
		ct.Context{Name: "plugin", Cluster: "plugin-cluster", User: "plugin-user", Server: "https://plugin.example", UserExtra: []string{"exec:", "  command: /bin/steal", "  apiVersion: client.authentication.k8s.io/v1"}},
	)
	contexts, err := connection.InspectKubeconfig(doc)
	if err != nil {
		t.Fatal(err)
	}
	want := []connection.KubeconfigContext{
		{Name: "kind-a", Cluster: "kind-a-cluster", Endpoint: "https://127.0.0.1:6443"},
		{Name: "prod", Cluster: "prod-cluster", Endpoint: "https://prod.example:443/k8s/clusters/c-1"},
	}
	if len(contexts) != len(want) {
		t.Fatalf("contexts: %#v", contexts)
	}
	for i := range want {
		if contexts[i] != want[i] {
			t.Fatalf("context %d: %#v", i, contexts[i])
		}
	}
	encoded, _ := json.Marshal(contexts)
	ca, cert, key := ct.TLSMaterial()
	for _, secret := range []string{secretToken, ca, cert, key, "steal"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("inspect leaked %q", secret)
		}
	}
}

func TestNormalizeKubeconfig_KeepsOnlySelectedContextAndTLSChoice(t *testing.T) {
	selected := ct.TokenContext("lab", "https://lab.example:6443", secretToken)
	selected.ClusterExtra = []string{"insecure-skip-tls-verify: true", "extensions:", "- name: x", "  extension: {a: b}"}
	selected.UserExtra = append(selected.UserExtra, "extensions:", "- name: y", "  extension: {c: d}")
	doc := ct.Document(selected, ct.TokenContext("other", "https://other.example", "other-token-value"))
	got, err := connection.NormalizeKubeconfig(doc, "lab")
	if err != nil {
		t.Fatal(err)
	}
	if got.Context != (connection.KubeconfigContext{Name: "lab", Cluster: "lab-cluster", Endpoint: "https://lab.example:6443"}) {
		t.Fatalf("context: %#v", got.Context)
	}
	var config struct {
		Clusters []struct {
			Name    string         `json:"name"`
			Cluster map[string]any `json:"cluster"`
		} `json:"clusters"`
		Users []struct {
			User map[string]any `json:"user"`
		} `json:"users"`
		Contexts       []map[string]any `json:"contexts"`
		CurrentContext string           `json:"current-context"`
	}
	if err := json.Unmarshal(got.Document, &config); err != nil {
		t.Fatal(err)
	}
	if len(config.Clusters) != 1 || len(config.Users) != 1 || len(config.Contexts) != 1 || config.CurrentContext != "lab" {
		t.Fatalf("not single-context: %s", got.Document)
	}
	if config.Clusters[0].Cluster["insecure-skip-tls-verify"] != true || config.Users[0].User["token"] != secretToken {
		t.Fatalf("TLS choice or token not preserved: %s", got.Document)
	}
	for _, dropped := range []string{"other", "extension"} {
		if strings.Contains(string(got.Document), dropped) {
			t.Fatalf("normalized config kept %q: %s", dropped, got.Document)
		}
	}
	cert, err := connection.NormalizeKubeconfig(ct.Document(ct.CertificateContext("c", "https://c.example")), "c")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cert.Document), "client-key-data") || !strings.Contains(string(cert.Document), "certificate-authority-data") {
		t.Fatalf("certificate material dropped: %s", cert.Document)
	}
	// JSON documents are accepted as well.
	if _, err := connection.NormalizeKubeconfig(string(got.Document), "lab"); err != nil {
		t.Fatalf("normalized JSON is not reusable: %v", err)
	}
}

func TestNormalizeKubeconfig_RejectionsAreSafeAndCategorized(t *testing.T) {
	base := func(mutate func(*ct.Context)) string {
		c := ct.TokenContext("ctx", "https://cluster.example", secretToken)
		mutate(&c)
		return ct.Document(c)
	}
	tokenAsName := ct.Document(ct.Context{Name: secretToken + "\x01", Cluster: "c", User: "u", Server: "https://c.example", UserExtra: []string{"token: abc"}})
	cases := map[string]struct {
		doc     string
		context string
		want    error
	}{
		"exec":             {base(func(c *ct.Context) { c.UserExtra = []string{"exec:", "  command: aws"} }), "ctx", connection.ErrUnsupportedAuthentication},
		"auth-provider":    {base(func(c *ct.Context) { c.UserExtra = []string{"auth-provider:", "  name: gcp"} }), "ctx", connection.ErrUnsupportedAuthentication},
		"token file":       {base(func(c *ct.Context) { c.UserExtra = []string{"tokenFile: /var/run/token"} }), "ctx", connection.ErrUnsupportedAuthentication},
		"client cert file": {base(func(c *ct.Context) { c.UserExtra = []string{"client-certificate: /tmp/c", "client-key: /tmp/k"} }), "ctx", connection.ErrUnsupportedAuthentication},
		"basic auth":       {base(func(c *ct.Context) { c.UserExtra = []string{"username: admin", "password: " + secretToken} }), "ctx", connection.ErrUnsupportedAuthentication},
		"no credential":    {base(func(c *ct.Context) { c.UserExtra = nil }), "ctx", connection.ErrUnsupportedAuthentication},
		"ca file":          {base(func(c *ct.Context) { c.ClusterExtra = []string{"certificate-authority: /etc/ca.crt"} }), "ctx", connection.ErrUnsupportedClusterSetting},
		"proxy":            {base(func(c *ct.Context) { c.ClusterExtra = []string{"proxy-url: http://proxy"} }), "ctx", connection.ErrUnsupportedClusterSetting},
		"userinfo":         {base(func(c *ct.Context) { c.Server = "https://admin:" + secretToken + "@cluster.example" }), "ctx", connection.ErrInvalidServer},
		"scheme":           {base(func(c *ct.Context) { c.Server = "file:///etc/passwd" }), "ctx", connection.ErrInvalidServer},
		"query":            {base(func(c *ct.Context) { c.Server = "https://cluster.example/?token=" + secretToken }), "ctx", connection.ErrInvalidServer},
		"bad CA":           {base(func(c *ct.Context) { c.ClusterExtra = []string{"certificate-authority-data: bm90LWEtY2VydA=="} }), "ctx", connection.ErrInvalidTLS},
		"half key pair": {base(func(c *ct.Context) {
			_, cert, _ := ct.TLSMaterial()
			c.UserExtra = []string{"client-certificate-data: " + cert}
		}), "ctx", connection.ErrInvalidTLS},
		"missing context":    {base(func(*ct.Context) {}), "", connection.ErrContextRequired},
		"unknown context":    {base(func(*ct.Context) {}), "nope", connection.ErrContextNotFound},
		"dangling user":      {strings.Replace(base(func(*ct.Context) {}), "    user: ctx-user", "    user: ghost", 1), "ctx", connection.ErrNoValidContext},
		"duplicate key":      {strings.Replace(base(func(*ct.Context) {}), "kind: Config\n", "kind: Config\nkind: Config\n", 1), "ctx", connection.ErrKubeconfigInvalid},
		"duplicate name":     {ct.Document(ct.TokenContext("ctx", "https://a.example", "a"), ct.TokenContext("ctx", "https://b.example", "b")), "ctx", connection.ErrKubeconfigInvalid},
		"alias":              {"apiVersion: v1\nkind: Config\nclusters:\n- &c {name: c, cluster: {server: 'https://c.example'}}\n- *c\n", "ctx", connection.ErrKubeconfigInvalid},
		"second document":    {base(func(*ct.Context) {}) + "---\napiVersion: v1\n", "ctx", connection.ErrKubeconfigInvalid},
		"custom tag":         {"apiVersion: !!binary djE=\nkind: Config\n", "ctx", connection.ErrKubeconfigInvalid},
		"unknown top field":  {base(func(*ct.Context) {}) + "evil: true\n", "ctx", connection.ErrKubeconfigInvalid},
		"wrong api version":  {strings.Replace(base(func(*ct.Context) {}), "apiVersion: v1", "apiVersion: v2", 1), "ctx", connection.ErrKubeconfigInvalid},
		"not yaml":           {"{{{" + secretToken, "ctx", connection.ErrKubeconfigInvalid},
		"empty":              {"   ", "ctx", connection.ErrKubeconfigInvalid},
		"credential in name": {tokenAsName, secretToken + "\x01", connection.ErrKubeconfigInvalid},
		"insecure with CA": {base(func(c *ct.Context) {
			ca, _, _ := ct.TLSMaterial()
			c.ClusterExtra = []string{"insecure-skip-tls-verify: true", "certificate-authority-data: " + ca}
		}), "ctx", connection.ErrInvalidTLS},
		"oversized document": {strings.Repeat("#", connection.MaxKubeconfigBytes+1), "ctx", connection.ErrKubeconfigTooLarge},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := connection.NormalizeKubeconfig(tc.doc, tc.context)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if strings.Contains(err.Error(), secretToken) || strings.Contains(err.Error(), "/var/run") || strings.Contains(err.Error(), "admin") {
				t.Fatalf("error echoes document content: %v", err)
			}
		})
	}
}

func TestInspectKubeconfig_NoUsableContext(t *testing.T) {
	onlyExec := ct.Document(ct.Context{Name: "eks", Cluster: "c", User: "u", Server: "https://eks.example", UserExtra: []string{"exec:", "  command: aws"}})
	if _, err := connection.InspectKubeconfig(onlyExec); !errors.Is(err, connection.ErrUnsupportedAuthentication) {
		t.Fatalf("exec-only kubeconfig: %v", err)
	}
	if _, err := connection.InspectKubeconfig("apiVersion: v1\nkind: Config\nclusters: []\nusers: []\ncontexts: []\n"); !errors.Is(err, connection.ErrNoValidContext) {
		t.Fatalf("no contexts: %v", err)
	}
}

// apiVersion: v1 and kind: Config are mandatory. A document whose clusters,
// users and contexts are otherwise valid is still rejected when either field
// is missing or null, both on inspection and on normalization.
func TestKubeconfig_RequiresAPIVersionAndKind(t *testing.T) {
	valid := ct.Single("ctx", "https://cluster.example", secretToken)
	if _, err := connection.InspectKubeconfig(valid); err != nil {
		t.Fatalf("baseline document rejected: %v", err)
	}
	cases := map[string]string{
		"apiVersion missing": strings.Replace(valid, "apiVersion: v1\n", "", 1),
		"apiVersion null":    strings.Replace(valid, "apiVersion: v1\n", "apiVersion: null\n", 1),
		"apiVersion tilde":   strings.Replace(valid, "apiVersion: v1\n", "apiVersion: ~\n", 1),
		"apiVersion empty":   strings.Replace(valid, "apiVersion: v1\n", "apiVersion:\n", 1),
		"kind missing":       strings.Replace(valid, "kind: Config\n", "", 1),
		"kind null":          strings.Replace(valid, "kind: Config\n", "kind: null\n", 1),
		"kind wrong":         strings.Replace(valid, "kind: Config\n", "kind: Secret\n", 1),
		"both missing":       strings.Replace(strings.Replace(valid, "apiVersion: v1\n", "", 1), "kind: Config\n", "", 1),
		"both null":          strings.Replace(strings.Replace(valid, "apiVersion: v1\n", "apiVersion: null\n", 1), "kind: Config\n", "kind: null\n", 1),
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			if doc == valid {
				t.Fatal("fixture did not change the document")
			}
			_, inspectErr := connection.InspectKubeconfig(doc)
			_, normalizeErr := connection.NormalizeKubeconfig(doc, "ctx")
			for _, err := range []error{inspectErr, normalizeErr} {
				if !errors.Is(err, connection.ErrKubeconfigInvalid) {
					t.Fatalf("got %v, want ErrKubeconfigInvalid", err)
				}
				if strings.Contains(err.Error(), secretToken) || strings.Contains(err.Error(), "cluster.example") {
					t.Fatalf("error echoes document content: %v", err)
				}
			}
		})
	}
}
