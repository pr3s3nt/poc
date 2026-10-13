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
	publicConnectionConfig       = []string{"cluster", "kubeContext", "endpoint", "region", "accountId", "principalArn"}
	publicConnectionVerification = []string{"verified", "endpoint", "serverVersion", "cluster", "region", "accountId", "principalArn", "identity", "regionCheck", "capabilityCheck"}
	// publicConnectionLists are allow-listed lists of plain strings (AWS
	// regions and provisioning capability groups).
	publicConnectionConfigLists       = []string{"allowedRegions"}
	publicConnectionVerificationLists = []string{"regions", "capabilities"}
)

func publicConnection(conn application.Connection) connectionView {
	name := conn.Name
	if name == "" {
		name = conn.Key
	}
	return connectionView{
		Key: conn.Key, Name: name, Kind: string(conn.Kind), AuthenticationType: string(conn.AuthenticationType),
		Config: withLists(pick(conn.Config, publicConnectionConfig), conn.Config, publicConnectionConfigLists), Status: string(conn.Status),
		Verification: withLists(pick(conn.Verification, publicConnectionVerification), conn.Verification, publicConnectionVerificationLists),
	}
}

// withLists adds allow-listed lists whose members are all plain strings; a
// list of any other shape is dropped.
func withLists(out, values map[string]any, keys []string) map[string]any {
	for _, key := range keys {
		var list []string
		switch typed := values[key].(type) {
		case []string:
			list = typed
		case []any:
			for _, member := range typed {
				text, ok := member.(string)
				if !ok {
					list = nil
					break
				}
				list = append(list, text)
			}
		}
		if len(list) > 0 {
			out[key] = append([]string(nil), list...)
		}
	}
	return out
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

// awsEnvelopeLimit bounds the AWS registration body: a key pair, a name and a
// short region list are far below 16 KiB.
const awsEnvelopeLimit = 16 << 10

// registerAWSBody is the only accepted AWS registration shape.
type registerAWSBody struct {
	Name            *string  `json:"name"`
	AccessKeyID     *string  `json:"accessKeyId"`
	SecretAccessKey *string  `json:"secretAccessKey"`
	AllowedRegions  []string `json:"allowedRegions"`
}

func (s *Server) handleRegisterAWSConnection(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	var body registerAWSBody
	if !decodeAWSBody(w, r, &body) {
		return
	}
	created, err := s.connections.RegisterAWS(r.Context(), org, connectionapp.RegisterAWSCommand{
		Name: deref(body.Name), AccessKeyID: deref(body.AccessKeyID), SecretAccessKey: deref(body.SecretAccessKey), AllowedRegions: body.AllowedRegions,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, publicConnection(created))
}

// decodeAWSBody reads one bounded JSON object with exactly the known keys,
// each at most once: string members for name/accessKeyId/secretAccessKey and
// an array of strings for allowedRegions, nothing after the object. Errors
// never echo the body.
func decodeAWSBody(w http.ResponseWriter, r *http.Request, dst *registerAWSBody) bool {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, awsEnvelopeLimit))
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
	if !exactAWSKeys(object) {
		writeCode(w, codeInvalidRequest, "")
		return false
	}
	if err := json.Unmarshal(object, dst); err != nil {
		writeCode(w, codeInvalidRequest, "")
		return false
	}
	return true
}

func exactAWSKeys(object json.RawMessage) bool {
	decoder := json.NewDecoder(bytes.NewReader(object))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return false
	}
	allowed := map[string]bool{"name": true, "accessKeyId": true, "secretAccessKey": true, "allowedRegions": true}
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || !allowed[key] || seen[key] {
			return false
		}
		seen[key] = true
		if key == "allowedRegions" {
			if token, err := decoder.Token(); err != nil || token != json.Delim('[') {
				return false
			}
			for decoder.More() {
				if member, err := decoder.Token(); err != nil {
					return false
				} else if _, ok := member.(string); !ok {
					return false
				}
			}
			if token, err := decoder.Token(); err != nil || token != json.Delim(']') {
				return false
			}
			continue
		}
		if value, err := decoder.Token(); err != nil {
			return false
		} else if _, ok := value.(string); !ok {
			return false
		}
	}
	return true
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
