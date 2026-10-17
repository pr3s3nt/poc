package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"

	connectionapp "orchestrator/internal/application/connection"
	"orchestrator/internal/domain/application"
)

// V01 infrastructure account API (UC-04 AC-01..12, ADR-015). Every route
// derives the Organization from the session and requires Platform Engineer or
// Admin; a key of another Organization is a scoped 404. The Account DTO is a
// fixed whitelist: it carries no secret reference, credential, raw
// configuration or provider error.

const (
	accountSmallBodyLimit = 4 << 10
	accountPathVersion    = "/api/v1/driver-accounts"
)

type accountConfig struct {
	AccountID           string   `json:"accountId,omitempty"`
	PrincipalARN        string   `json:"principalArn,omitempty"`
	VerificationRegions []string `json:"verificationRegions,omitempty"`
	Cluster             string   `json:"cluster,omitempty"`
	KubeContext         string   `json:"kubeContext,omitempty"`
	Endpoint            string   `json:"endpoint,omitempty"`
	TLSVerification     string   `json:"tlsVerification,omitempty"`
	TLSCustomCA         *bool    `json:"tlsCustomCA,omitempty"`
}

type accountVerification struct {
	Verified        bool     `json:"verified"`
	VerifiedAt      string   `json:"verifiedAt,omitempty"`
	AccountID       string   `json:"accountId,omitempty"`
	PrincipalARN    string   `json:"principalArn,omitempty"`
	Regions         []string `json:"regions,omitempty"`
	Capabilities    []string `json:"capabilities,omitempty"`
	Identity        string   `json:"identity,omitempty"`
	RegionCheck     string   `json:"regionCheck,omitempty"`
	CapabilityCheck string   `json:"capabilityCheck,omitempty"`
	Cluster         string   `json:"cluster,omitempty"`
	Endpoint        string   `json:"endpoint,omitempty"`
	ServerVersion   string   `json:"serverVersion,omitempty"`
}

type accountView struct {
	ID                 string              `json:"id"`
	Key                string              `json:"key"`
	Name               string              `json:"name"`
	Type               string              `json:"type"`
	AuthenticationType string              `json:"authenticationType"`
	Status             string              `json:"status"`
	Version            int64               `json:"version"`
	Config             accountConfig       `json:"config"`
	Verification       accountVerification `json:"verification"`
}

func str(values map[string]any, key string) string {
	if v, ok := values[key].(string); ok {
		return v
	}
	return ""
}

func strList(values map[string]any, key string) []string {
	return application.Connection{Config: values}.ConfigStrings(key)
}

func publicAccount(conn application.Connection) accountView {
	name := conn.Name
	if name == "" {
		name = conn.Key
	}
	view := accountView{
		ID: conn.ID, Key: conn.Key, Name: name, Type: connectionapp.AccountType(conn.Kind),
		AuthenticationType: string(conn.AuthenticationType), Status: string(conn.Status), Version: conn.Version,
	}
	v := conn.Verification
	verified, _ := v["verified"].(bool)
	view.Verification = accountVerification{
		Verified: verified, VerifiedAt: str(v, "verifiedAt"),
	}
	if conn.Kind == application.ConnectionAWS {
		view.Config = accountConfig{
			AccountID: str(conn.Config, "accountId"), PrincipalARN: str(conn.Config, "principalArn"),
			VerificationRegions: connectionapp.AccountRegions(conn),
		}
		view.Verification.AccountID, view.Verification.PrincipalARN = str(v, "accountId"), str(v, "principalArn")
		view.Verification.Regions, view.Verification.Capabilities = strList(v, "regions"), strList(v, "capabilities")
		view.Verification.Identity, view.Verification.RegionCheck = str(v, "identity"), str(v, "regionCheck")
		view.Verification.CapabilityCheck = str(v, "capabilityCheck")
		return view
	}
	view.Config = accountConfig{
		Cluster: str(conn.Config, "cluster"), KubeContext: str(conn.Config, "kubeContext"), Endpoint: str(conn.Config, "endpoint"),
		TLSVerification: str(conn.Config, "tlsVerification"),
	}
	if custom, ok := conn.Config["tlsCustomCA"].(bool); ok {
		view.Config.TLSCustomCA = &custom
	}
	view.Verification.Cluster, view.Verification.Endpoint = str(v, "cluster"), str(v, "endpoint")
	view.Verification.ServerVersion = str(v, "serverVersion")
	return view
}

func (s *Server) registerDriverAccountRoutes() {
	s.mux.HandleFunc("GET "+accountPathVersion, s.handleListAccounts)
	s.mux.HandleFunc("GET "+accountPathVersion+"/{key}", s.handleGetAccount)
	s.mux.HandleFunc("POST "+accountPathVersion+"/internal/inspect", s.handleInspectKubeconfig)
	s.mux.HandleFunc("POST "+accountPathVersion+"/internal", s.handleCreateInternalAccount)
	s.mux.HandleFunc("POST "+accountPathVersion+"/aws", s.handleCreateAWSAccount)
	s.mux.HandleFunc("PUT "+accountPathVersion+"/{key}", s.handleRenameAccount)
	s.mux.HandleFunc("POST "+accountPathVersion+"/{key}/verify", s.handleVerifyAccount)
	s.mux.HandleFunc("PUT "+accountPathVersion+"/{key}/credentials", s.handleReplaceAccountCredentials)
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	accounts, err := s.connections.ListAccounts(r.Context(), org)
	if err != nil {
		s.writeError(w, err)
		return
	}
	views := make([]accountView, 0, len(accounts))
	for _, conn := range accounts {
		views = append(views, publicAccount(conn))
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": views})
}

func (s *Server) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	conn, err := s.connections.GetAccount(r.Context(), org, r.PathValue("key"))
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, publicAccount(conn))
}

func (s *Server) handleCreateInternalAccount(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	var body registerConnectionBody
	if !decodeConnectionBody(w, r, map[string]bool{"name": true, "kubeconfig": true, "context": true}, &body) {
		return
	}
	created, err := s.connections.RegisterKubeconfig(r.Context(), org, connectionapp.RegisterKubeconfigCommand{
		Name: deref(body.Name), Kubeconfig: deref(body.Kubeconfig), Context: deref(body.Context),
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, publicAccount(created))
}

// accountBody is the union of the strict account request fields; each route
// declares which of them it accepts and requires.
type accountBody struct {
	Name                *string  `json:"name"`
	Kubeconfig          *string  `json:"kubeconfig"`
	Context             *string  `json:"context"`
	AccessKeyID         *string  `json:"accessKeyId"`
	SecretAccessKey     *string  `json:"secretAccessKey"`
	VerificationRegions []string `json:"verificationRegions"`
	Version             int64    `json:"version"`
}

func (s *Server) handleCreateAWSAccount(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	var body accountBody
	spec := map[string]bodyKind{"name": kindString, "accessKeyId": kindString, "secretAccessKey": kindString, "verificationRegions": kindStringList}
	if !decodeAccountBody(w, r, awsEnvelopeLimit, spec, []string{"name", "accessKeyId", "secretAccessKey", "verificationRegions"}, &body) {
		return
	}
	created, err := s.connections.CreateAWSAccount(r.Context(), org, connectionapp.RegisterAWSCommand{
		Name: deref(body.Name), AccessKeyID: deref(body.AccessKeyID), SecretAccessKey: deref(body.SecretAccessKey), AllowedRegions: body.VerificationRegions,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, publicAccount(created))
}

func (s *Server) handleRenameAccount(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	var body accountBody
	if !decodeAccountBody(w, r, accountSmallBodyLimit, map[string]bodyKind{"name": kindString, "version": kindVersion}, []string{"name", "version"}, &body) {
		return
	}
	updated, err := s.connections.RenameAccount(r.Context(), org, r.PathValue("key"), body.Version, deref(body.Name))
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, publicAccount(updated))
}

func (s *Server) handleVerifyAccount(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	conn, err := s.connections.GetAccount(r.Context(), org, r.PathValue("key"))
	if err != nil {
		s.writeError(w, err)
		return
	}
	spec := map[string]bodyKind{"version": kindVersion}
	if conn.Kind == application.ConnectionAWS {
		spec["verificationRegions"] = kindStringList
	}
	var body accountBody
	if !decodeAccountBody(w, r, accountSmallBodyLimit, spec, []string{"version"}, &body) {
		return
	}
	updated, err := s.connections.VerifyAccount(r.Context(), org, conn.Key, body.Version, body.VerificationRegions)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, publicAccount(updated))
}

func (s *Server) handleReplaceAccountCredentials(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	conn, err := s.connections.GetAccount(r.Context(), org, r.PathValue("key"))
	if err != nil {
		s.writeError(w, err)
		return
	}
	var body accountBody
	var updated application.Connection
	if conn.Kind == application.ConnectionKubernetes {
		spec := map[string]bodyKind{"version": kindVersion, "kubeconfig": kindString, "context": kindString}
		if !decodeAccountBody(w, r, connectionEnvelopeLimit, spec, []string{"version", "kubeconfig", "context"}, &body) {
			return
		}
		updated, err = s.connections.ReplaceInternalCredentials(r.Context(), org, conn.Key, body.Version, deref(body.Kubeconfig), deref(body.Context))
	} else {
		spec := map[string]bodyKind{"version": kindVersion, "accessKeyId": kindString, "secretAccessKey": kindString, "verificationRegions": kindStringList}
		if !decodeAccountBody(w, r, awsEnvelopeLimit, spec, []string{"version", "accessKeyId", "secretAccessKey", "verificationRegions"}, &body) {
			return
		}
		updated, err = s.connections.ReplaceAWSCredentials(r.Context(), org, conn.Key, body.Version, connectionapp.RegisterAWSCommand{
			AccessKeyID: deref(body.AccessKeyID), SecretAccessKey: deref(body.SecretAccessKey), AllowedRegions: body.VerificationRegions,
		})
	}
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, publicAccount(updated))
}

type bodyKind int

const (
	kindString bodyKind = iota
	kindStringList
	kindVersion
)

// decodeAccountBody reads one bounded JSON object whose exact-case keys are
// all in spec with the declared type (no null), each at most once, with every
// required key present and nothing after the object. Errors never echo input.
func decodeAccountBody(w http.ResponseWriter, r *http.Request, limit int64, spec map[string]bodyKind, required []string, dst *accountBody) bool {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		if !writeTooLarge(w, err) {
			writeCode(w, codeInvalidRequest, "")
		}
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var object json.RawMessage
	if err := decoder.Decode(&object); err != nil {
		writeCode(w, codeInvalidRequest, "")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeCode(w, codeInvalidRequest, "")
		return false
	}
	present, ok := scanAccountObject(object, spec)
	if !ok {
		writeCode(w, codeInvalidRequest, "")
		return false
	}
	for _, key := range required {
		if !present[key] {
			writeCode(w, codeInvalidRequest, key)
			return false
		}
	}
	if err := json.Unmarshal(object, dst); err != nil {
		writeCode(w, codeInvalidRequest, "")
		return false
	}
	if present["version"] && dst.Version < 1 {
		writeCode(w, codeInvalidRequest, "version")
		return false
	}
	return true
}

func scanAccountObject(object json.RawMessage, spec map[string]bodyKind) (map[string]bool, bool) {
	decoder := json.NewDecoder(bytes.NewReader(object))
	decoder.UseNumber()
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil, false
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		key, isString := token.(string)
		kind, known := spec[key]
		if err != nil || !isString || !known || seen[key] {
			return nil, false
		}
		seen[key] = true
		switch kind {
		case kindString:
			if value, err := decoder.Token(); err != nil {
				return nil, false
			} else if _, ok := value.(string); !ok {
				return nil, false
			}
		case kindVersion:
			value, err := decoder.Token()
			number, ok := value.(json.Number)
			if err != nil || !ok {
				return nil, false
			}
			n, err := number.Int64()
			if err != nil || n < 1 || n == math.MaxInt64 {
				return nil, false
			}
		case kindStringList:
			if token, err := decoder.Token(); err != nil || token != json.Delim('[') {
				return nil, false
			}
			for decoder.More() {
				if member, err := decoder.Token(); err != nil {
					return nil, false
				} else if _, ok := member.(string); !ok {
					return nil, false
				}
			}
			if token, err := decoder.Token(); err != nil || token != json.Delim(']') {
				return nil, false
			}
		}
	}
	return seen, true
}
