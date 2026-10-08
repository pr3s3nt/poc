package http

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	appconfig "orchestrator/internal/application/configuration"
	"orchestrator/internal/application/envops"
	"orchestrator/internal/application/secretstores"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/domain/secretstore"
	configport "orchestrator/internal/ports/configuration"
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
	Legacy          bool           `json:"legacy,omitempty"`
	Verification    map[string]any `json:"verification"`
}

var publicStoreVerification = []string{"verified", "kvVersion", "capabilities", "probe", "kubernetesAuth", "legacy"}

func publicSecretStore(store secretstore.Store) secretStoreView {
	return secretStoreView{
		Key: store.Key, Name: store.Name, Provider: string(store.Provider), BackendAddress: store.BackendAddress,
		WorkloadAddress: store.WorkloadAddress, Mount: store.Mount, AuthMount: store.AuthMount, TLSCustomCA: store.TLSCAPEM != "",
		Status: string(store.Status), Legacy: store.Legacy, Verification: pick(store.Verification, publicStoreVerification),
	}
}

// storeChoiceView is the Developer projection: name and key only.
type storeChoiceView struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Status   string `json:"status"`
	Legacy   bool   `json:"legacy,omitempty"`
}

func (s *Server) handleListSecretStores(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	stores, err := s.secretStores.List(r.Context(), org)
	if err != nil {
		writeManagementError(w, err)
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
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	stores, err := s.secretStores.Choices(r.Context(), identity.OrganizationKey)
	if err != nil {
		writeManagementError(w, err)
		return
	}
	views := make([]storeChoiceView, 0, len(stores))
	for _, store := range stores {
		views = append(views, storeChoiceView{Key: store.Key, Name: store.Name, Provider: string(store.Provider), Status: string(store.Status), Legacy: store.Legacy})
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
		writeSecretStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, publicSecretStore(created))
}

// writeSecretStoreError maps registration failures to fixed safe guidance.
func writeSecretStoreError(w http.ResponseWriter, err error) {
	var cleanup *secretstores.CleanupError
	switch {
	case errors.Is(err, secretstores.ErrInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
	case errors.Is(err, secretstores.ErrVerification):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
	case errors.Is(err, secretstores.ErrCredentialStore):
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
	case errors.As(err, &cleanup):
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": cleanup.Error()})
	default:
		writeManagementError(w, err)
	}
}

type setSecretStoreBody struct {
	SecretStoreKey        json.RawMessage `json:"secretStoreKey"`
	ExpectedVersion       json.RawMessage `json:"expectedVersion"`
	ExpectedConfigVersion json.RawMessage `json:"expectedConfigVersion"`
}

func (s *Server) handleSetSecretStore(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.sessionIdentity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	var req setSecretStoreBody
	if err := decodeSingleObject(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "request must be one JSON object with only secretStoreKey, expectedVersion and expectedConfigVersion"})
		return
	}
	var key string
	if req.SecretStoreKey == nil || json.Unmarshal(req.SecretStoreKey, &key) != nil || key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "secretStoreKey must be a non-blank string", "field": "secretStoreKey"})
		return
	}
	var version, configVersion int64
	if req.ExpectedVersion == nil || json.Unmarshal(req.ExpectedVersion, &version) != nil || version <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "expectedVersion must be a positive integer", "field": "expectedVersion"})
		return
	}
	if req.ExpectedConfigVersion == nil || json.Unmarshal(req.ExpectedConfigVersion, &configVersion) != nil || configVersion < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "expectedConfigVersion must be a non-negative integer", "field": "expectedConfigVersion"})
		return
	}
	appKey, envKey := r.PathValue("id"), r.PathValue("env")
	app, err := s.store.GetApplication(r.Context(), appKey)
	if err != nil || app.OrganizationKey != identity.OrganizationKey || (envKey != "staging" && envKey != "production") {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
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
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Status      string    `json:"status"`
	Stage       string    `json:"stage,omitempty"`
	StartedAt   time.Time `json:"startedAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	HeartbeatAt time.Time `json:"heartbeatAt"`
	Failure     string    `json:"failure,omitempty"`
	Recoverable bool      `json:"recoverable"`
}

func operationViewOf(op environment.Operation) operationView {
	return operationView{
		ID: op.ID, Kind: string(op.Kind), Status: string(op.Status), Stage: op.Stage, StartedAt: op.StartedAt, UpdatedAt: op.UpdatedAt,
		HeartbeatAt: op.HeartbeatAt, Failure: op.Failure, Recoverable: op.Status == environment.OpInterrupted,
	}
}

// writeBusy answers 409 ENVIRONMENT_BUSY with the visible owner, never details.
func (s *Server) writeBusy(w http.ResponseWriter, err error) {
	body := map[string]any{"error": "the environment is busy with another operation; wait for it to finish or review its status", "code": "ENVIRONMENT_BUSY"}
	var held *envops.ErrBusy
	var keyed *persistence.BusyError
	switch {
	case errors.As(err, &held):
		body["operation"] = operationViewOf(held.Operation)
	case errors.As(err, &keyed) && s != nil && s.store != nil:
		if op, ok, getErr := s.store.ActiveOperation(context.Background(), keyed.ApplicationKey, keyed.EnvironmentKey); getErr == nil && ok {
			body["operation"] = operationViewOf(op)
		}
	}
	writeJSON(w, http.StatusConflict, body)
}

// writeEnvironmentError maps the ADR-012 failures shared by every route; it
// reports whether it handled err. Messages are fixed and never quote stores.
func (s *Server) writeEnvironmentError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, persistence.ErrEnvironmentBusy):
		s.writeBusy(w, err)
	case errors.Is(err, persistence.ErrVersionConflict):
		writeJSON(w, http.StatusConflict, map[string]any{"error": conflictStaleVersion, "code": "STALE_VERSION"})
	case errors.Is(err, appconfig.ErrNoStore):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "select a secret store in Environment Settings before adding a secret", "field": "secretStoreKey", "code": "SECRET_STORE_REQUIRED"})
	case errors.Is(err, configport.ErrStoreUnavailable):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "the selected secret store is not available", "field": "secretStoreKey", "code": "SECRET_STORE_UNAVAILABLE"})
	case errors.Is(err, appconfig.ErrLegacyUnreadable):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": appconfig.ErrLegacyUnreadable.Error(), "code": "LEGACY_VALUE_UNREADABLE"})
	case errors.Is(err, appconfig.ErrStoreCopy):
		log.Printf("secret store copy failed: %v", errors.Unwrap(err) != nil)
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": appconfig.ErrStoreCopy.Error(), "code": "SECRET_COPY_FAILED"})
	case errors.Is(err, persistence.ErrOperationLost):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "the operation was interrupted; review its status", "code": "OPERATION_LOST"})
	default:
		return false
	}
	return true
}
