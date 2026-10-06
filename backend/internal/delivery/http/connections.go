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
		writeManagementError(w, err)
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
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "kubeconfig is required"})
		return
	}
	contexts, err := s.connections.InspectKubeconfig(r.Context(), "", *body.Kubeconfig)
	if err != nil {
		writeConnectionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"contexts": contexts})
}

// registerConnectionBody accepts exactly one of two explicit shapes: the
// upload body {name,kubeconfig,context} or the legacy host-context body
// {key,clusterId,kubeContext}. A mixed document is rejected; neither shape
// falls back to the other.
type registerConnectionBody struct {
	Name        *string `json:"name"`
	Kubeconfig  *string `json:"kubeconfig"`
	Context     *string `json:"context"`
	Key         *string `json:"key"`
	ClusterID   *string `json:"clusterId"`
	KubeContext *string `json:"kubeContext"`
}

func (s *Server) handleRegisterKubernetesConnection(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	var body registerConnectionBody
	allowed := map[string]bool{"name": true, "kubeconfig": true, "context": true, "key": true, "clusterId": true, "kubeContext": true}
	if !decodeConnectionBody(w, r, allowed, &body) {
		return
	}
	upload := body.Name != nil || body.Kubeconfig != nil || body.Context != nil
	legacy := body.Key != nil || body.ClusterID != nil || body.KubeContext != nil
	if upload && legacy {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "use either {name, kubeconfig, context} or the legacy {key, clusterId, kubeContext} body, not both"})
		return
	}
	var (
		created application.Connection
		err     error
	)
	if legacy {
		created, err = s.connections.RegisterKubernetesCluster(r.Context(), org, connectionapp.RegisterKubernetesCommand{Key: deref(body.Key), ClusterID: deref(body.ClusterID), KubeContext: deref(body.KubeContext)})
	} else {
		created, err = s.connections.RegisterKubeconfig(r.Context(), org, connectionapp.RegisterKubeconfigCommand{Name: deref(body.Name), Kubeconfig: deref(body.Kubeconfig), Context: deref(body.Context)})
	}
	if err != nil {
		writeConnectionError(w, err)
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

// writeConnectionError maps UC-04 failures. Messages are fixed guidance; raw
// parser, kubectl and Vault errors never reach the response or the log.
func writeConnectionError(w http.ResponseWriter, err error) {
	var cleanup *connectionapp.CleanupError
	switch {
	case errors.Is(err, connectionapp.ErrKubeconfigTooLarge):
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": err.Error()})
	case errors.Is(err, connectionapp.ErrCredentialStore):
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
	case errors.As(err, &cleanup):
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": cleanup.Error()})
	default:
		writeManagementError(w, err)
	}
}

// decodeConnectionBody reads one bounded JSON object whose top-level keys are
// exact-case members of allowed, each at most once, with string values only
// and nothing after the object.
func decodeConnectionBody(w http.ResponseWriter, r *http.Request, allowed map[string]bool, dst any) bool {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, connectionEnvelopeLimit))
	if err != nil {
		if !writeTooLarge(w, err) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "request body could not be read"})
		}
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var object json.RawMessage
	if err := decoder.Decode(&object); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "request body must be one JSON object"})
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "request body must contain exactly one JSON object"})
		return false
	}
	if !exactObjectKeys(object, allowed) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "request body has unknown, duplicate or non-string fields"})
		return false
	}
	if err := json.Unmarshal(object, dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "request body has unknown, duplicate or non-string fields"})
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
