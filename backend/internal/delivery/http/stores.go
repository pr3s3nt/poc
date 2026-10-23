package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	appconfig "orchestrator/internal/application/configuration"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/application/secretstores"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/secretstore"
	"orchestrator/internal/ports/persistence"
)

// secretStoreView is the public DTO: it never carries the token, the
// credential reference or raw provider results.
type secretStoreView struct {
	Key             string         `json:"key"`
	Name            string         `json:"name"`
	Provider        string         `json:"provider"`
	BackendAddress  string         `json:"backendAddress"`
	WorkloadAddress string         `json:"workloadAddress"`
	Mount           string         `json:"mount"`
	AuthMount       string         `json:"authMount"`
	TLSCustomCA     bool           `json:"tlsCustomCa"`
	Status          string         `json:"status"`
	Verification    map[string]any `json:"verification"`
}

var publicStoreVerification = []string{"verified", "kvVersion", "capabilities", "probe", "kubernetesAuth"}

func publicSecretStore(store secretstore.Store) secretStoreView {
	return secretStoreView{
		Key: store.Key, Name: store.Name, Provider: string(store.Provider), BackendAddress: store.BackendAddress,
		WorkloadAddress: store.WorkloadAddress, Mount: store.Mount, AuthMount: store.AuthMount, TLSCustomCA: store.TLSCAPEM != "",
		Status: string(store.Status), Verification: pick(store.Verification, publicStoreVerification),
	}
}

// storeChoiceView is the Developer projection: name and key only.
type storeChoiceView struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Status   string `json:"status"`
}

func (s *Server) handleListSecretStores(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	stores, err := s.secretStores.List(r.Context(), org)
	if err != nil {
		s.writeError(w, err)
		return
	}
	views := make([]secretStoreView, 0, len(stores))
	for _, store := range stores {
		views = append(views, publicSecretStore(store))
	}
	writeJSON(w, http.StatusOK, map[string]any{"secretStores": views})
}

func (s *Server) handleSecretStoreChoices(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.sessionIdentity(r)
	if !ok {
		writeCode(w, codeUnauthorized, "")
		return
	}
	stores, err := s.secretStores.Choices(r.Context(), identity.OrganizationKey)
	if err != nil {
		s.writeError(w, err)
		return
	}
	views := make([]storeChoiceView, 0, len(stores))
	for _, store := range stores {
		views = append(views, storeChoiceView{Key: store.Key, Name: store.Name, Provider: string(store.Provider), Status: string(store.Status)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"secretStores": views})
}

const secretStoreBodyLimit = 256 << 10

type registerSecretStoreBody struct {
	Name            string `json:"name"`
	BackendAddress  string `json:"backendAddress"`
	WorkloadAddress string `json:"workloadAddress"`
	Mount           string `json:"mount"`
	AuthMount       string `json:"authMount"`
	TLSCAPEM        string `json:"tlsCaPem"`
	Token           string `json:"token"`
}

func (s *Server) handleRegisterSecretStore(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	var body registerSecretStoreBody
	if !decodeStrict(w, r, secretStoreBodyLimit, &body) {
		return
	}
	created, err := s.secretStores.Register(r.Context(), org, secretstores.RegisterCommand{
		Name: body.Name, BackendAddress: body.BackendAddress, WorkloadAddress: body.WorkloadAddress,
		Mount: body.Mount, AuthMount: body.AuthMount, TLSCAPEM: body.TLSCAPEM, Token: body.Token,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, publicSecretStore(created))
}

type setSecretStoreBody struct {
	SecretStoreKey        json.RawMessage `json:"secretStoreKey"`
	ExpectedVersion       json.RawMessage `json:"expectedVersion"`
	ExpectedConfigVersion json.RawMessage `json:"expectedConfigVersion"`
}

func (s *Server) handleSetSecretStore(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.sessionIdentity(r)
	if !ok {
		writeCode(w, codeUnauthorized, "")
		return
	}
	var req setSecretStoreBody
	if err := decodeSingleObject(r, &req); err != nil {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	var key string
	if req.SecretStoreKey == nil || json.Unmarshal(req.SecretStoreKey, &key) != nil || key == "" {
		writeCode(w, codeInvalidRequest, "secretStoreKey")
		return
	}
	var version, configVersion int64
	if req.ExpectedVersion == nil || json.Unmarshal(req.ExpectedVersion, &version) != nil || version <= 0 {
		writeCode(w, codeInvalidRequest, "expectedVersion")
		return
	}
	if req.ExpectedConfigVersion == nil || json.Unmarshal(req.ExpectedConfigVersion, &configVersion) != nil || configVersion < 0 {
		writeCode(w, codeInvalidRequest, "expectedConfigVersion")
		return
	}
	appKey, envKey := r.PathValue("id"), r.PathValue("env")
	app, err := s.store.GetApplication(r.Context(), appKey)
	if err != nil || app.OrganizationKey != identity.OrganizationKey || (envKey != "staging" && envKey != "production") {
		writeCode(w, codeNotFound, "")
		return
	}
	result, err := s.configurations.SetSecretStore(r.Context(), appconfig.SwitchCommand{
		OrganizationKey: identity.OrganizationKey, ApplicationKey: appKey, EnvironmentKey: envKey, StoreKey: key,
		ExpectedVersion: version, ExpectedConfigVersion: configVersion,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"environment": s.environmentViewOf(r.Context(), identity.OrganizationKey, result.Environment),
		"changed":     result.Changed, "copiedSecrets": result.Copied,
	})
}

// operationView is the safe projection of an Environment operation claim.
type operationView struct {
	ID              string    `json:"id"`
	ReviewRequestID string    `json:"reviewRequestId,omitempty"`
	Kind            string    `json:"kind"`
	Status          string    `json:"status"`
	Stage           string    `json:"stage,omitempty"`
	StartedAt       time.Time `json:"startedAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
	HeartbeatAt     time.Time `json:"heartbeatAt"`
	Failure         string    `json:"failure,omitempty"`
	Recoverable     bool      `json:"recoverable"`
}

func operationViewOf(op environment.Operation) operationView {
	return operationView{
		ID: op.ID, ReviewRequestID: op.ReviewRequestID, Kind: string(op.Kind), Status: string(op.Status), Stage: op.Stage, StartedAt: op.StartedAt, UpdatedAt: op.UpdatedAt,
		HeartbeatAt: op.HeartbeatAt, Failure: op.Failure, Recoverable: op.Status == environment.OpInterrupted,
	}
}

// busyExtra adds the visible owner of a busy Environment to the response: the
// safe operation projection, never details.
func (s *Server) busyExtra(err error) map[string]any {
	var held *envops.ErrBusy
	var keyed *persistence.BusyError
	switch {
	case errors.As(err, &held):
		return map[string]any{"operation": operationViewOf(held.Operation)}
	case errors.As(err, &keyed) && s != nil && s.store != nil:
		if op, ok, getErr := s.store.ActiveOperation(context.Background(), keyed.ApplicationKey, keyed.EnvironmentKey); getErr == nil && ok {
			return map[string]any{"operation": operationViewOf(op)}
		}
	}
	return nil
}
