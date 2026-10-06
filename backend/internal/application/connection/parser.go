package connection

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// MaxKubeconfigBytes bounds one uploaded or pasted kubeconfig document.
const MaxKubeconfigBytes = 1 << 20

// Kubeconfig rejection categories. Every message is fixed guidance: names,
// endpoints and field values from the document are never echoed in errors.
var (
	// ErrKubeconfigTooLarge is an oversized document (HTTP 413).
	ErrKubeconfigTooLarge = errors.New("connection: kubeconfig document is larger than 1 MiB")
	// ErrKubeconfigInvalid is a document that is not a supported kubeconfig.
	ErrKubeconfigInvalid = fmt.Errorf("%w: the kubeconfig is not a valid kubeconfig document", ErrInvalid)
	// ErrNoValidContext is ERR-02.
	ErrNoValidContext = fmt.Errorf("%w: the kubeconfig has no valid context; provide a kubeconfig whose context references a cluster with an HTTPS or HTTP server and a user with an embedded token or client certificate", ErrInvalid)
	// ErrContextRequired is ERR-03.
	ErrContextRequired = fmt.Errorf("%w: select a context from the kubeconfig before checking and saving", ErrInvalid)
	// ErrContextNotFound rejects a context that is not a valid context of the document.
	ErrContextNotFound = fmt.Errorf("%w: the selected context is not a valid context of this kubeconfig; inspect the kubeconfig again and select one of its contexts", ErrInvalid)
	// ErrUnsupportedAuthentication is ERR-04.
	ErrUnsupportedAuthentication = fmt.Errorf("%w: the selected context uses an unsupported authentication method; external credential files, tokenFile, exec and auth-provider plugins are not supported and are never run. Provide a kubeconfig with an embedded token or embedded client certificate and key", ErrInvalid)
	// ErrUnsupportedClusterSetting rejects external CA files, proxies and unknown cluster settings.
	ErrUnsupportedClusterSetting = fmt.Errorf("%w: the selected cluster uses an unsupported setting (an external certificate-authority file, a proxy or an unknown field); embed the CA data instead", ErrInvalid)
	// ErrInvalidServer rejects unsafe or malformed cluster server URLs.
	ErrInvalidServer = fmt.Errorf("%w: the selected cluster server must be an http or https URL without user information, query or fragment", ErrInvalid)
	// ErrInvalidTLS rejects malformed embedded TLS material.
	ErrInvalidTLS = fmt.Errorf("%w: the selected context has invalid embedded TLS data (certificate authority, client certificate or key)", ErrInvalid)
)

// KubeconfigContext is the safe metadata of one usable context.
type KubeconfigContext struct {
	Name     string `json:"name"`
	Cluster  string `json:"cluster"`
	Endpoint string `json:"endpoint"`
}

// SelectedKubeconfig is the normalized single-context kubeconfig. Document
// holds credential bytes: it lives in request memory and the credential store
// only, never in logs, errors, records or responses.
type SelectedKubeconfig struct {
	Context  KubeconfigContext
	Document []byte
}

var kubeconfigName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@/+-]{0,252}$`)

const (
	maxKubeconfigEntries = 256
	maxKubeconfigDepth   = 32
	maxServerLength      = 2048
	maxTokenLength       = 16 << 10
)

// InspectKubeconfig parses a document and returns its usable contexts. It
// writes nothing and contacts no cluster.
func InspectKubeconfig(document string) ([]KubeconfigContext, error) {
	doc, err := parseKubeconfig(document)
	if err != nil {
		return nil, err
	}
	var out []KubeconfigContext
	unsupported := false
	for _, name := range doc.contextOrder {
		selected, err := doc.selected(name)
		if err != nil {
			if errors.Is(err, ErrUnsupportedAuthentication) || errors.Is(err, ErrUnsupportedClusterSetting) {
				unsupported = true
			}
			continue
		}
		out = append(out, selected.Context)
	}
	if len(out) == 0 {
		if unsupported {
			return nil, ErrUnsupportedAuthentication
		}
		return nil, ErrNoValidContext
	}
	return out, nil
}

// NormalizeKubeconfig revalidates the document independently of any earlier
// inspection and returns only the selected context, cluster and user.
func NormalizeKubeconfig(document, contextName string) (SelectedKubeconfig, error) {
	doc, err := parseKubeconfig(document)
	if err != nil {
		return SelectedKubeconfig{}, err
	}
	if strings.TrimSpace(contextName) == "" {
		return SelectedKubeconfig{}, ErrContextRequired
	}
	if _, ok := doc.contexts[contextName]; !ok {
		return SelectedKubeconfig{}, ErrContextNotFound
	}
	return doc.selected(contextName)
}

type kubeconfigDocument struct {
	clusters     map[string]*yaml.Node
	users        map[string]*yaml.Node
	contexts     map[string]*yaml.Node
	contextOrder []string
}

func parseKubeconfig(document string) (*kubeconfigDocument, error) {
	if len(document) > MaxKubeconfigBytes {
		return nil, ErrKubeconfigTooLarge
	}
	if strings.TrimSpace(document) == "" {
		return nil, ErrKubeconfigInvalid
	}
	decoder := yaml.NewDecoder(strings.NewReader(document))
	var root yaml.Node
	if err := decoder.Decode(&root); err != nil {
		return nil, ErrKubeconfigInvalid
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		// A second document or trailing garbage is never merged or ignored.
		return nil, ErrKubeconfigInvalid
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) != 1 {
		return nil, ErrKubeconfigInvalid
	}
	count := 0
	if err := checkNode(root.Content[0], 0, &count); err != nil {
		return nil, err
	}
	top := root.Content[0]
	if top.Kind != yaml.MappingNode {
		return nil, ErrKubeconfigInvalid
	}
	doc := &kubeconfigDocument{clusters: map[string]*yaml.Node{}, users: map[string]*yaml.Node{}, contexts: map[string]*yaml.Node{}}
	// apiVersion: v1 and kind: Config are required; a document missing either
	// is not a complete supported kubeconfig.
	hasAPIVersion, hasKind := false, false
	for i := 0; i < len(top.Content); i += 2 {
		key, value := top.Content[i].Value, top.Content[i+1]
		switch key {
		case "apiVersion":
			if !isString(value) || value.Value != "v1" {
				return nil, ErrKubeconfigInvalid
			}
			hasAPIVersion = true
		case "kind":
			if !isString(value) || value.Value != "Config" {
				return nil, ErrKubeconfigInvalid
			}
			hasKind = true
		case "clusters":
			if _, err := namedEntries(value, "cluster", doc.clusters); err != nil {
				return nil, err
			}
		case "users":
			if _, err := namedEntries(value, "user", doc.users); err != nil {
				return nil, err
			}
		case "contexts":
			order, err := namedEntries(value, "context", doc.contexts)
			if err != nil {
				return nil, err
			}
			doc.contextOrder = order
		case "current-context":
			if !isString(value) && !isNull(value) {
				return nil, ErrKubeconfigInvalid
			}
		case "preferences", "extensions":
			// Dropped before storage; never passed to kubectl.
		default:
			return nil, ErrKubeconfigInvalid
		}
	}
	if !hasAPIVersion || !hasKind {
		return nil, ErrKubeconfigInvalid
	}
	if len(doc.contexts) == 0 {
		return nil, ErrNoValidContext
	}
	return doc, nil
}

// checkNode rejects aliases/anchors, custom tags, non-string or duplicate
// mapping keys and excessive nesting before any field is interpreted.
func checkNode(node *yaml.Node, depth int, count *int) error {
	*count++
	if depth > maxKubeconfigDepth || *count > 100000 {
		return ErrKubeconfigInvalid
	}
	if node.Kind == yaml.AliasNode || node.Anchor != "" {
		return ErrKubeconfigInvalid
	}
	switch node.Tag {
	case "", "!!map", "!!seq", "!!str", "!!int", "!!bool", "!!null", "!!float":
	default:
		return ErrKubeconfigInvalid
	}
	if node.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || seen[key.Value] {
				return ErrKubeconfigInvalid
			}
			seen[key.Value] = true
		}
	}
	for _, child := range node.Content {
		if err := checkNode(child, depth+1, count); err != nil {
			return err
		}
	}
	return nil
}

// namedEntries reads a kubeconfig list of {name, <field>} objects. Duplicate
// names are rejected rather than resolved by position.
func namedEntries(list *yaml.Node, field string, into map[string]*yaml.Node) ([]string, error) {
	if isNull(list) {
		return nil, nil
	}
	if list.Kind != yaml.SequenceNode || len(list.Content) > maxKubeconfigEntries {
		return nil, ErrKubeconfigInvalid
	}
	var order []string
	for _, entry := range list.Content {
		if entry.Kind != yaml.MappingNode {
			return nil, ErrKubeconfigInvalid
		}
		var name string
		var body *yaml.Node
		for i := 0; i < len(entry.Content); i += 2 {
			key, value := entry.Content[i].Value, entry.Content[i+1]
			switch key {
			case "name":
				if !isString(value) {
					return nil, ErrKubeconfigInvalid
				}
				name = value.Value
			case field:
				if isNull(value) {
					// `user:` with no fields is an empty, credential-less entry.
					value = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				}
				if value.Kind != yaml.MappingNode {
					return nil, ErrKubeconfigInvalid
				}
				body = value
			case "extensions":
			default:
				return nil, ErrKubeconfigInvalid
			}
		}
		if name == "" || body == nil {
			return nil, ErrKubeconfigInvalid
		}
		if _, dup := into[name]; dup {
			return nil, ErrKubeconfigInvalid
		}
		into[name] = body
		order = append(order, name)
	}
	return order, nil
}

func (d *kubeconfigDocument) selected(contextName string) (SelectedKubeconfig, error) {
	ctxNode := d.contexts[contextName]
	if ctxNode == nil || !kubeconfigName.MatchString(contextName) {
		return SelectedKubeconfig{}, ErrContextNotFound
	}
	var clusterName, userName string
	for i := 0; i < len(ctxNode.Content); i += 2 {
		key, value := ctxNode.Content[i].Value, ctxNode.Content[i+1]
		switch key {
		case "cluster":
			if !isString(value) {
				return SelectedKubeconfig{}, ErrNoValidContext
			}
			clusterName = value.Value
		case "user":
			if !isString(value) {
				return SelectedKubeconfig{}, ErrNoValidContext
			}
			userName = value.Value
		case "namespace", "extensions":
			// Namespace is chosen per operation by the orchestrator.
		default:
			return SelectedKubeconfig{}, ErrNoValidContext
		}
	}
	if !kubeconfigName.MatchString(clusterName) || !kubeconfigName.MatchString(userName) {
		return SelectedKubeconfig{}, ErrNoValidContext
	}
	clusterNode, userNode := d.clusters[clusterName], d.users[userName]
	if clusterNode == nil || userNode == nil {
		return SelectedKubeconfig{}, ErrNoValidContext
	}
	cluster, endpoint, err := normalizeCluster(clusterNode)
	if err != nil {
		return SelectedKubeconfig{}, err
	}
	user, err := normalizeUser(userNode)
	if err != nil {
		return SelectedKubeconfig{}, err
	}
	normalized := map[string]any{
		"apiVersion":      "v1",
		"kind":            "Config",
		"clusters":        []any{map[string]any{"name": clusterName, "cluster": cluster}},
		"users":           []any{map[string]any{"name": userName, "user": user}},
		"contexts":        []any{map[string]any{"name": contextName, "context": map[string]any{"cluster": clusterName, "user": userName}}},
		"current-context": contextName,
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return SelectedKubeconfig{}, ErrKubeconfigInvalid
	}
	return SelectedKubeconfig{Context: KubeconfigContext{Name: contextName, Cluster: clusterName, Endpoint: endpoint}, Document: encoded}, nil
}

func normalizeCluster(node *yaml.Node) (map[string]any, string, error) {
	out := map[string]any{}
	server := ""
	insecure := false
	hasCA := false
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i].Value, node.Content[i+1]
		switch key {
		case "server":
			if !isString(value) {
				return nil, "", ErrInvalidServer
			}
			server = value.Value
		case "certificate-authority-data":
			if !isString(value) || !validCA(value.Value) {
				return nil, "", ErrInvalidTLS
			}
			out[key] = value.Value
			hasCA = true
		case "insecure-skip-tls-verify":
			if value.Tag != "!!bool" || value.Decode(&insecure) != nil {
				return nil, "", ErrInvalidTLS
			}
			// The supplied TLS choice is preserved, never added or removed.
			out[key] = insecure
		case "tls-server-name":
			if !isString(value) || !kubeconfigName.MatchString(value.Value) {
				return nil, "", ErrInvalidTLS
			}
			out[key] = value.Value
		case "disable-compression":
			var disabled bool
			if value.Tag != "!!bool" || value.Decode(&disabled) != nil {
				return nil, "", ErrKubeconfigInvalid
			}
			out[key] = disabled
		case "extensions":
		default:
			// certificate-authority (external file), proxy-url and unknown fields.
			return nil, "", ErrUnsupportedClusterSetting
		}
	}
	if insecure && hasCA {
		return nil, "", ErrInvalidTLS
	}
	if err := validServer(server); err != nil {
		return nil, "", err
	}
	out["server"] = server
	return out, server, nil
}

func normalizeUser(node *yaml.Node) (map[string]any, error) {
	out := map[string]any{}
	var cert, key string
	for i := 0; i < len(node.Content); i += 2 {
		field, value := node.Content[i].Value, node.Content[i+1]
		switch field {
		case "token":
			if !isString(value) || !validToken(value.Value) {
				return nil, ErrNoValidContext
			}
			out[field] = value.Value
		case "client-certificate-data":
			if !isString(value) {
				return nil, ErrInvalidTLS
			}
			cert = value.Value
		case "client-key-data":
			if !isString(value) {
				return nil, ErrInvalidTLS
			}
			key = value.Value
		case "extensions":
		default:
			// tokenFile, client-certificate, client-key, exec, auth-provider,
			// username/password and impersonation are not supported.
			return nil, ErrUnsupportedAuthentication
		}
	}
	if cert != "" || key != "" {
		if cert == "" || key == "" || !validKeyPair(cert, key) {
			return nil, ErrInvalidTLS
		}
		out["client-certificate-data"] = cert
		out["client-key-data"] = key
	}
	if len(out) == 0 {
		return nil, ErrUnsupportedAuthentication
	}
	return out, nil
}

func validServer(server string) error {
	if server == "" || len(server) > maxServerLength || strings.IndexFunc(server, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) >= 0 {
		return ErrInvalidServer
	}
	u, err := url.Parse(server)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.Hostname() == "" ||
		u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return ErrInvalidServer
	}
	return nil
}

func validToken(token string) bool {
	if token == "" || len(token) > maxTokenLength {
		return false
	}
	for _, r := range token {
		if r <= ' ' || r > '~' {
			return false
		}
	}
	return true
}

func validCA(data string) bool {
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return false
	}
	found := false
	for {
		var block *pem.Block
		block, raw = pem.Decode(raw)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			return false
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return false
		}
		found = true
	}
	return found && len(strings.TrimSpace(string(raw))) == 0
}

func validKeyPair(certData, keyData string) bool {
	cert, err := base64.StdEncoding.DecodeString(certData)
	if err != nil {
		return false
	}
	key, err := base64.StdEncoding.DecodeString(keyData)
	if err != nil {
		return false
	}
	_, err = tls.X509KeyPair(cert, key)
	return err == nil
}

func isString(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.ScalarNode && node.Tag == "!!str"
}

func isNull(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.ScalarNode && node.Tag == "!!null"
}
