package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	connectionapp "orchestrator/internal/application/connection"
	"orchestrator/internal/domain/application"
)

// connectionEnvelopeLimit bounds a UC-04 kubeconfig request body. The
// document itself is limited to 1 MiB; the envelope leaves room for JSON
// escaping of that document.
const connectionEnvelopeLimit = 8 << 20

// connectionView is the public Connection DTO. It never carries the secret
// reference, credentials, kubeconfig content or raw provider results.
type connectionView struct {
	Key                string         `json:"key"`
	Name               string         `json:"name"`
	Kind               string         `json:"kind"`
	AuthenticationType string         `json:"authenticationType"`
	Config             map[string]any `json:"config"`
	Status             string         `json:"status"`
	Verification       map[string]any `json:"verification"`
}

var (
	publicConnectionConfig       = []string{"cluster", "kubeContext", "endpoint", "region", "accountId"}
	publicConnectionVerification = []string{"verified", "endpoint", "serverVersion", "cluster", "region", "accountId"}
)

func publicConnection(conn application.Connection) connectionView {
	name := conn.Name
	if name == "" {
		name = conn.Key
	}
	return connectionView{
		Key: conn.Key, Name: name, Kind: string(conn.Kind), AuthenticationType: string(conn.AuthenticationType),
		Config: pick(conn.Config, publicConnectionConfig), Status: string(conn.Status),
		Verification: pick(conn.Verification, publicConnectionVerification),
	}
}

// pick copies only allow-listed scalar fields.
func pick(values map[string]any, keys []string) map[string]any {
	out := map[string]any{}
	for _, key := range keys {
		switch value := values[key].(type) {
		case string, bool, float64, int:
			out[key] = value
		}
	}
	return out
}

func (s *Server) handleConnections(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	connections, err := s.store.ListConnections(r.Context(), org)
	if err != nil {
		s.writeError(w, err)
		return
	}
	views := make([]connectionView, 0, len(connections))
	for _, conn := range connections {
		views = append(views, publicConnection(conn))
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": views})
}

func (s *Server) handleInspectKubeconfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.platformActor(w, r); !ok {
		return
	}
	var body struct {
		Kubeconfig *string `json:"kubeconfig"`
	}
	if !decodeConnectionBody(w, r, map[string]bool{"kubeconfig": true}, &body) {
		return
	}
	if body.Kubeconfig == nil {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	contexts, err := s.connections.InspectKubeconfig(r.Context(), "", *body.Kubeconfig)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"contexts": contexts})
}

// registerConnectionBody is the only accepted registration shape: the upload
// body {name,kubeconfig,context}. Any other field (including the removed
// {key,clusterId,kubeContext} body) is rejected by decodeConnectionBody.
type registerConnectionBody struct {
	Name       *string `json:"name"`
	Kubeconfig *string `json:"kubeconfig"`
	Context    *string `json:"context"`
}

func (s *Server) handleRegisterKubernetesConnection(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	var body registerConnectionBody
	allowed := map[string]bool{"name": true, "kubeconfig": true, "context": true}
	if !decodeConnectionBody(w, r, allowed, &body) {
		return
	}
	created, err := s.connections.RegisterKubeconfig(r.Context(), org, connectionapp.RegisterKubeconfigCommand{Name: deref(body.Name), Kubeconfig: deref(body.Kubeconfig), Context: deref(body.Context)})
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, publicConnection(created))
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// decodeConnectionBody reads one bounded JSON object whose top-level keys are
// exact-case members of allowed, each at most once, with string values only
// and nothing after the object.
func decodeConnectionBody(w http.ResponseWriter, r *http.Request, allowed map[string]bool, dst any) bool {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, connectionEnvelopeLimit))
	if err != nil {
		if !writeTooLarge(w, err) {
			writeCode(w, codeInvalidRequest, "")
		}
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var object json.RawMessage
	if err := decoder.Decode(&object); err != nil {
		writeCode(w, codeInvalidRequest, "")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeCode(w, codeInvalidRequest, "")
		return false
	}
	if !exactObjectKeys(object, allowed) {
		writeCode(w, codeInvalidRequest, "")
		return false
	}
	if err := json.Unmarshal(object, dst); err != nil {
		writeCode(w, codeInvalidRequest, "")
		return false
	}
	return true
}

func exactObjectKeys(object json.RawMessage, allowed map[string]bool) bool {
	decoder := json.NewDecoder(bytes.NewReader(object))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return false
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || !allowed[key] || seen[key] {
			return false
		}
		seen[key] = true
		value, err := decoder.Token()
		if err != nil {
			return false
		}
		if _, ok := value.(string); !ok {
			return false
		}
	}
	return true
}
