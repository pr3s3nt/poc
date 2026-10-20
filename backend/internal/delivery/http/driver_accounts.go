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

// V01 infrastructure account API (UC-04 AC-01..12). The routes are a second
// projection of the Connection records, not a second catalog.

const accountSmallEnvelopeLimit = 16 << 10

func (s *Server) registerDriverAccountRoutes() {
	s.mux.HandleFunc("GET /api/v1/driver-accounts", s.handleListAccounts)
	s.mux.HandleFunc("GET /api/v1/driver-accounts/{key}", s.handleGetAccount)
	s.mux.HandleFunc("POST /api/v1/driver-accounts/internal/inspect", s.handleInspectKubeconfig)
	s.mux.HandleFunc("POST /api/v1/driver-accounts/internal", s.handleCreateInternalAccount)
	s.mux.HandleFunc("POST /api/v1/driver-accounts/aws", s.handleCreateAWSAccount)
	s.mux.HandleFunc("POST /api/v1/driver-accounts/aws/ec2-role", s.handleCreateEC2RoleAccount)
	s.mux.HandleFunc("PUT /api/v1/driver-accounts/{key}", s.handleRenameAccount)
	s.mux.HandleFunc("POST /api/v1/driver-accounts/{key}/verify", s.handleVerifyAccount)
	s.mux.HandleFunc("PUT /api/v1/driver-accounts/{key}/credentials", s.handleReplaceAccountCredentials)
}

// accountView is the fixed public Account DTO. Everything is copied from an
// allow-list; the private reference, credentials, raw verification and the
// legacy allowed-region policy never appear.
type accountView struct {
	ID                 string         `json:"id"`
	Key                string         `json:"key"`
	Name               string         `json:"name"`
	Type               string         `json:"type"`
	AuthenticationType string         `json:"authenticationType"`
	Status             string         `json:"status"`
	Version            int64          `json:"version"`
	Config             map[string]any `json:"config"`
	Verification       map[string]any `json:"verification"`
}

func accountType(kind application.ConnectionKind) string {
	if kind == application.ConnectionKubernetes {
		return "INTERNAL"
	}
	return "AWS"
}

func publicAccount(conn application.Connection) accountView {
	name := conn.Name
	if name == "" {
		name = conn.Key
	}
	config := map[string]any{}
	verification := map[string]any{}
	switch conn.Kind {
	case application.ConnectionKubernetes:
		config = pick(conn.Config, []string{"cluster", "kubeContext", "endpoint"})
		tls := pick(conn.Config, []string{"tlsMode", "tlsServerName"})
		if mode, ok := tls["tlsMode"].(string); ok {
			safe := map[string]any{"mode": mode}
			if name, ok := tls["tlsServerName"].(string); ok {
				safe["serverName"] = name
			}
			config["tls"] = safe
		}
		verification = pick(conn.Verification, []string{"verified", "endpoint", "serverVersion", "verifiedAt"})
	case application.ConnectionAWS:
		config = pick(conn.Config, []string{"accountId", "principalArn", "roleArn", "roleId"})
		regions := conn.ConfigStrings("verificationRegions")
		if len(regions) == 0 {
			regions = conn.AllowedRegions()
		}
		if len(regions) > 0 {
			config["verificationRegions"] = regions
		}
		verification = withLists(pick(conn.Verification, []string{"verified", "accountId", "principalArn", "identity", "regionCheck", "capabilityCheck", "verifiedAt"}), conn.Verification, []string{"regions", "capabilities"})
	}
	return accountView{
		ID: conn.ID, Key: conn.Key, Name: name, Type: accountType(conn.Kind), AuthenticationType: string(conn.AuthenticationType),
		Status: string(conn.Status), Version: conn.Version, Config: config, Verification: verification,
	}
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
	body, ok := decodeStrictBody(w, r, connectionEnvelopeLimit, map[string]memberKind{"name": kindString, "kubeconfig": kindString, "context": kindString})
	if !ok {
		return
	}
	created, err := s.connections.RegisterKubeconfig(r.Context(), org, connectionapp.RegisterKubeconfigCommand{Name: body.text("name"), Kubeconfig: body.text("kubeconfig"), Context: body.text("context")})
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, publicAccount(created))
}

func (s *Server) handleCreateAWSAccount(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	body, ok := decodeStrictBody(w, r, accountSmallEnvelopeLimit, map[string]memberKind{
		"name": kindString, "accessKeyId": kindString, "secretAccessKey": kindString, "verificationRegions": kindStringList,
	})
	if !ok {
		return
	}
	created, err := s.connections.RegisterAWSDriverAccount(r.Context(), org, connectionapp.RegisterAWSCommand{
		Name: body.text("name"), AccessKeyID: body.text("accessKeyId"), SecretAccessKey: body.text("secretAccessKey"), AllowedRegions: body.list("verificationRegions"),
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
	body, ok := decodeStrictBody(w, r, accountSmallEnvelopeLimit, map[string]memberKind{"name": kindString, "version": kindInteger})
	if !ok {
		return
	}
	if !body.has("name") || !body.has("version") {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	updated, err := s.connections.RenameAccount(r.Context(), org, r.PathValue("key"), body.number("version"), body.text("name"))
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
	body, ok := decodeStrictBody(w, r, accountSmallEnvelopeLimit, map[string]memberKind{"version": kindInteger, "verificationRegions": kindStringList})
	if !ok {
		return
	}
	if !body.has("version") {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	cmd := connectionapp.VerifyAccountCommand{Version: body.number("version")}
	if body.has("verificationRegions") {
		cmd.VerificationRegions = body.list("verificationRegions")
	}
	updated, err := s.connections.VerifyAccount(r.Context(), org, r.PathValue("key"), cmd)
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
	// The account kind decides the only accepted body shape; a foreign key is
	// a scoped 404 before any body is parsed.
	conn, err := s.connections.GetAccount(r.Context(), org, r.PathValue("key"))
	if err != nil {
		s.writeError(w, err)
		return
	}
	switch conn.Kind {
	case application.ConnectionKubernetes:
		body, ok := decodeStrictBody(w, r, connectionEnvelopeLimit, map[string]memberKind{"version": kindInteger, "kubeconfig": kindString, "context": kindString})
		if !ok {
			return
		}
		if !body.has("version") || !body.has("kubeconfig") || !body.has("context") {
			writeCode(w, codeInvalidRequest, "")
			return
		}
		updated, err := s.connections.ReplaceKubeconfigCredential(r.Context(), org, conn.Key, connectionapp.ReplaceKubeconfigCommand{
			Version: body.number("version"), Kubeconfig: body.text("kubeconfig"), Context: body.text("context"),
		})
		if err != nil {
			s.writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, publicAccount(updated))
	case application.ConnectionAWS:
		body, ok := decodeStrictBody(w, r, accountSmallEnvelopeLimit, map[string]memberKind{
			"version": kindInteger, "accessKeyId": kindString, "secretAccessKey": kindString, "verificationRegions": kindStringList,
		})
		if !ok {
			return
		}
		if !body.has("version") || !body.has("accessKeyId") || !body.has("secretAccessKey") || !body.has("verificationRegions") {
			writeCode(w, codeInvalidRequest, "")
			return
		}
		updated, err := s.connections.ReplaceAWSCredential(r.Context(), org, conn.Key, connectionapp.ReplaceAWSCommand{
			Version: body.number("version"), AccessKeyID: body.text("accessKeyId"), SecretAccessKey: body.text("secretAccessKey"),
			VerificationRegions: body.list("verificationRegions"),
		})
		if err != nil {
			s.writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, publicAccount(updated))
	default:
		writeCode(w, codeInvalidRequest, "")
	}
}

// memberKind is the only JSON type one body member may have.
type memberKind int

const (
	kindString memberKind = iota
	kindStringList
	kindInteger
)

// strictBody holds the validated members of one request object.
type strictBody struct {
	texts   map[string]string
	lists   map[string][]string
	numbers map[string]int64
}

func (b strictBody) has(key string) bool {
	if _, ok := b.texts[key]; ok {
		return true
	}
	if _, ok := b.lists[key]; ok {
		return true
	}
	_, ok := b.numbers[key]
	return ok
}
func (b strictBody) text(key string) string   { return b.texts[key] }
func (b strictBody) list(key string) []string { return b.lists[key] }
func (b strictBody) number(key string) int64  { return b.numbers[key] }

// decodeStrictBody reads one bounded JSON object whose members are exactly
// known keys, each at most once and with its declared type (no null, no mixed
// shape; integers must be plain positive-or-zero decimal numbers), and nothing
// after the object. Errors never echo the body.
func decodeStrictBody(w http.ResponseWriter, r *http.Request, limit int64, spec map[string]memberKind) (strictBody, bool) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		if !writeTooLarge(w, err) {
			writeCode(w, codeInvalidRequest, "")
		}
		return strictBody{}, false
	}
	body, ok := parseStrictObject(raw, spec)
	if !ok {
		writeCode(w, codeInvalidRequest, "")
		return strictBody{}, false
	}
	return body, true
}

func parseStrictObject(raw []byte, spec map[string]memberKind) (strictBody, bool) {
	out := strictBody{texts: map[string]string{}, lists: map[string][]string{}, numbers: map[string]int64{}}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return out, false
	}
	for decoder.More() {
		token, err := decoder.Token()
		key, isKey := token.(string)
		kind, known := spec[key]
		if err != nil || !isKey || !known || out.has(key) {
			return out, false
		}
		switch kind {
		case kindString:
			value, err := decoder.Token()
			text, isText := value.(string)
			if err != nil || !isText {
				return out, false
			}
			out.texts[key] = text
		case kindInteger:
			value, err := decoder.Token()
			number, isNumber := value.(json.Number)
			if err != nil || !isNumber {
				return out, false
			}
			parsed, err := number.Int64()
			if err != nil || parsed < 0 {
				return out, false
			}
			out.numbers[key] = parsed
		case kindStringList:
			if token, err := decoder.Token(); err != nil || token != json.Delim('[') {
				return out, false
			}
			list := []string{}
			for decoder.More() {
				member, err := decoder.Token()
				text, isText := member.(string)
				if err != nil || !isText {
					return out, false
				}
				list = append(list, text)
			}
			if token, err := decoder.Token(); err != nil || token != json.Delim(']') {
				return out, false
			}
			out.lists[key] = list
		}
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return out, false
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return out, false
	}
	return out, true
}

func (s *Server) handleCreateEC2RoleAccount(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	body, ok := decodeStrictBody(w, r, accountSmallEnvelopeLimit, map[string]memberKind{"name": kindString, "roleArn": kindString, "verificationRegions": kindStringList})
	if !ok {
		return
	}
	created, err := s.connections.RegisterEC2Role(r.Context(), org, connectionapp.RegisterEC2RoleCommand{Name: body.text("name"), RoleARN: body.text("roleArn"), VerificationRegions: body.list("verificationRegions")})
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, publicAccount(created))
}
