// Command diagfixture provisions the PLATFORM CONTENT the diagnostics browser
// acceptance needs, through the product's own HTTP API, and writes every call
// it makes to a public log. It is test-fixture setup, not part of the claimed
// user actions: onboarding of the account, every diagnostic run, every
// inspection of results and every repair of the owned cluster are performed
// elsewhere (through the Console forms and native kubectl).
//
//	diagfixture -base URL -account KEY -log FILE [-storage-class fast] [-application NAME]
//
// It creates the Resource Types, the existing-cluster / namespace / workload /
// PostgreSQL Definitions that place an Application on the account, one
// Application, and one workload draft that requests a PostgreSQL database. The
// draft is NEVER deployed. It prints one JSON line with the application and
// environment keys.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"

	"orchestrator/internal/adapters/terraform/registry"
)

type client struct {
	base string
	http *http.Client
	log  *os.File
}

func (c *client) call(method, path string, body any) (int, map[string]any) {
	var data []byte
	if body != nil {
		data, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(method, c.base+"/api/v1"+path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		fail("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	fmt.Fprintf(c.log, "%s /api/v1%s -> %d\n", method, path, resp.StatusCode)
	return resp.StatusCode, out
}

func (c *client) must(method, path string, body any) map[string]any {
	status, out := c.call(method, path, body)
	if status >= 300 {
		fail("%s %s: HTTP %d code=%v", method, path, status, out["code"])
	}
	return out
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "diagfixture: "+format+"\n", args...)
	os.Exit(1)
}

func (c *client) signIn(user string) {
	jar, _ := cookiejar.New(nil)
	c.http = &http.Client{Jar: jar}
	c.must("POST", "/auth/sign-in", map[string]string{"username": user, "password": "test-password"})
}

func field(name, typ string, required bool) map[string]any {
	return map[string]any{"name": name, "type": typ, "required": required}
}

func module(key, resourceType, mod string, variables map[string]any, inputs, outputs []string, criteria []any) map[string]any {
	refs, err := registry.PackagedRefs()
	if err != nil {
		fail("packaged modules: %v", err)
	}
	ref, ok := refs[mod]
	if !ok {
		fail("unknown packaged module %s", mod)
	}
	in, out := map[string]any{}, map[string]any{}
	for _, name := range inputs {
		in[name] = map[string]any{"param": name}
	}
	for _, name := range outputs {
		out[name] = name
	}
	return map[string]any{"key": key, "name": "Diagnostics fixture " + key, "resourceType": resourceType, "driverType": "terraform",
		"driverInputs": map[string]any{"values": map[string]any{"source": ref.Map(), "variables": variables, "input_mapping": in, "output_mapping": out,
			"provider_bindings": map[string]any{"kubernetes": map[string]any{"target": "resource", "resource_ref": "k8s-cluster.internal#environments.@app.@env"}}}},
		"criteria": criteria}
}

func main() {
	base := flag.String("base", "", "Console base URL")
	account := flag.String("account", "", "key of the already onboarded Internal account")
	logPath := flag.String("log", "", "public call log file")
	class := flag.String("storage-class", "fast", "StorageClass the PostgreSQL Definition requires (a literal)")
	appName := flag.String("application", "Diagnostics shop", "Application name")
	subdomain := flag.String("subdomain", "diagshop", "Application subdomain")
	flag.Parse()
	if *base == "" || *account == "" || *logPath == "" {
		fail("usage: diagfixture -base URL -account KEY -log FILE")
	}
	log, err := os.OpenFile(*logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		fail("%v", err)
	}
	defer log.Close()
	c := &client{base: strings.TrimRight(*base, "/"), log: log}

	// Developer creates the Application (as in the product).
	c.signIn("developer")
	app := c.must("POST", "/applications", map[string]string{"name": *appName, "subdomain": *subdomain})
	key := ""
	if v, ok := app["application"].(map[string]any); ok {
		key, _ = v["key"].(string)
	}
	if key == "" {
		key, _ = app["key"].(string)
	}
	if key == "" {
		fail("application key missing in the response")
	}

	c.signIn("platform-engineer")
	c.must("POST", "/resource-types", map[string]any{"key": "workload", "inputs": []any{}, "outputs": []any{}})
	c.must("POST", "/resource-types", map[string]any{"key": "k8s-namespace", "inputs": []any{}, "outputs": []any{field("name", "string", true)}})
	c.must("POST", "/resource-types", map[string]any{"key": "k8s-cluster", "inputs": []any{}, "outputs": []any{field("name", "string", true), field("endpoint", "string", false), field("cluster", "any", true), field("kubeContext", "string", true), field("kubeconfig", "string", false)}})
	c.must("POST", "/resource-types", map[string]any{"key": "postgres", "inputs": []any{field("database", "string", true), field("username", "string", true)},
		"outputs": []any{field("host", "string", true), field("port", "number", true), field("database", "string", true), field("username", "string", true), field("password", "string", true)}})
	namespaceOutput := "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"
	c.must("POST", "/resource-definitions", map[string]any{"key": "diag-cluster", "name": "Diagnostics cluster", "resourceType": "k8s-cluster", "driverType": "existing-cluster", "driverAccount": *account,
		"driverInputs": map[string]any{"values": map[string]any{"variables": map[string]any{}}}, "criteria": []any{map[string]any{"class": "internal", "app_id": key}}})
	c.must("POST", "/resource-definitions", map[string]any{"key": "diag-workload", "name": "Diagnostics workload", "resourceType": "workload", "driverType": "score-k8s",
		"driverInputs": map[string]any{"values": map[string]any{"variables": map[string]any{"render_bundle": "score-k8s-internal-v1", "namespace": namespaceOutput, "cluster": "${resources['k8s-cluster.internal#environments.@app.@env'].outputs.cluster}"}}},
		"criteria":     []any{map[string]any{}}})
	c.must("POST", "/resource-definitions", module("diag-namespace", "k8s-namespace", "k8s-namespace", map[string]any{"name": "${context.env.namespace}"}, nil, []string{"name"}, []any{map[string]any{}}))
	c.must("POST", "/resource-definitions", module("diag-postgres", "postgres", "postgres-k8s",
		map[string]any{"image": "postgres:17-alpine", "storage": "1Gi", "storage_class": fmt.Sprintf("%q", *class), "namespace": namespaceOutput},
		[]string{"database", "username"}, []string{"host", "port", "database", "username", "password"}, []any{map[string]any{}}))

	draft := map[string]any{"apiVersion": "score.dev/v1b1", "metadata": map[string]any{"name": "api"},
		"containers": map[string]any{"main": map[string]any{"image": "registry.invalid/diag/api:v1"}},
		"service":    map[string]any{"ports": map[string]any{"http": map[string]any{"port": 8080}}, "publicRoutes": []any{map[string]any{"path": "/", "port": "http"}}},
		"resources":  map[string]any{"db": map[string]any{"type": "postgres", "class": "default", "params": map[string]any{"database": "app", "username": "app"}}}}
	c.signIn("developer")
	c.must("PUT", "/applications/"+key+"/environments/staging/workloads/api", map[string]any{"score": draft, "version": 0})
	out, _ := json.Marshal(map[string]string{"application": key, "environment": "staging", "storageClass": *class})
	fmt.Println(string(out))
}
