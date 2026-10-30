package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	appdelivery "orchestrator/internal/application/delivery"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/ports/persistence"
)

func (s *Server) registerDeliveryRoutes() {
	s.mux.HandleFunc("POST /api/v1/applications/{id}/environments/{env}/drift-repairs/preview", s.handlePrepareDriftRepair)
	s.mux.HandleFunc("GET /api/v1/applications/{id}/environments/{env}/drift-repairs/{review}", s.handleGetDriftRepair)
	s.mux.HandleFunc("POST /api/v1/applications/{id}/environments/{env}/drift-repairs", s.handleConfirmDriftRepair)
	s.mux.HandleFunc("POST /api/v1/applications/{id}/environments/{env}/drift-scans", s.handleQueueDriftScan)
	s.mux.HandleFunc("GET /api/v1/applications/{id}/environments/{env}/drift-scans", s.handleListDriftScans)
	s.mux.HandleFunc("GET /api/v1/applications/{id}/environments/{env}/drift-scans/{scan}", s.handleGetDriftScan)
	s.mux.HandleFunc("GET /api/v1/delivery-providers", s.handleDeliveryProviders)
	s.mux.HandleFunc("POST /api/v1/delivery-providers/{provider}/inspect", s.handleInspectDeliveryProvider)
	s.mux.HandleFunc("GET /api/v1/delivery-bindings", s.handleListDeliveryBindings)
	s.mux.HandleFunc("POST /api/v1/delivery-bindings", s.handleCreateDeliveryBinding)
	s.mux.HandleFunc("GET /api/v1/delivery-bindings/{binding}", s.handleDeliveryBindingDetails)
	s.mux.HandleFunc("POST /api/v1/delivery-bindings/{binding}/versions", s.handleCreateDeliveryBinding)
	s.mux.HandleFunc("POST /api/v1/delivery-bindings/{binding}/verify", s.handleVerifyDeliveryBinding)
	s.mux.HandleFunc("GET /api/v1/applications/{id}/environments/{env}/delivery", s.handleGetDelivery)
	s.mux.HandleFunc("PUT /api/v1/applications/{id}/environments/{env}/delivery", s.handleSelectDelivery)
	s.mux.HandleFunc("GET /api/v1/applications/{id}/environments/{env}/reconciliation", s.handleReconciliation)
}

type deliveryView struct {
	Desired               domain.Selection `json:"desired"`
	Applied               domain.Selection `json:"applied"`
	EnvironmentVersion    int64            `json:"environmentVersion"`
	ConfigurationRevision int64            `json:"configurationRevision"`
	Pending               bool             `json:"pending"`
}

func (s *Server) handleGetDelivery(w http.ResponseWriter, r *http.Request) {
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	if s.delivery == nil {
		writeCode(w, codeServiceUnavailable, "")
		return
	}
	identity, _ := s.sessionIdentity(r)
	current, err := s.delivery.Selection(r.Context(), identity.OrganizationKey, app, env)
	if err != nil {
		s.writeError(w, err)
		return
	}
	desired, applied := current.DesiredDelivery.Effective(), current.AppliedDelivery.Effective()
	writeJSON(w, http.StatusOK, deliveryView{Desired: desired, Applied: applied, EnvironmentVersion: current.Version, ConfigurationRevision: current.DeliveryConfigRevision, Pending: desired != applied})
}
func (s *Server) handleSelectDelivery(w http.ResponseWriter, r *http.Request) {
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	if s.delivery == nil {
		writeCode(w, codeServiceUnavailable, "")
		return
	}
	var req struct {
		ExpectedEnvironmentVersion    *int64      `json:"expectedEnvironmentVersion"`
		ExpectedConfigurationRevision *int64      `json:"expectedConfigurationRevision"`
		Mode                          domain.Mode `json:"mode"`
		BindingID                     string      `json:"bindingId"`
		BindingVersion                int64       `json:"bindingVersion"`
		SelfHeal                      *bool       `json:"selfHeal"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&req) != nil || req.ExpectedEnvironmentVersion == nil || req.ExpectedConfigurationRevision == nil || req.SelfHeal == nil || req.Mode == "" {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	current, err := s.delivery.Select(r.Context(), org, persistence.DeliverySelectionCommand{ApplicationKey: app, EnvironmentKey: env, ExpectedEnvironmentVersion: *req.ExpectedEnvironmentVersion, ExpectedConfigurationRevision: *req.ExpectedConfigurationRevision, Selection: domain.Selection{Mode: req.Mode, BindingID: req.BindingID, BindingVersion: req.BindingVersion, SelfHeal: *req.SelfHeal}})
	if err != nil {
		if errors.Is(err, domain.ErrInvalid) || errors.Is(err, domain.ErrUnsupported) {
			writeCode(w, codeInvalidRequest, "")
			return
		}
		s.writeError(w, err)
		return
	}
	desired, applied := current.DesiredDelivery.Effective(), current.AppliedDelivery.Effective()
	writeJSON(w, http.StatusOK, deliveryView{Desired: desired, Applied: applied, EnvironmentVersion: current.Version, ConfigurationRevision: current.DeliveryConfigRevision, Pending: desired != applied})
}

type bindingChoice struct {
	ID           string               `json:"id"`
	Version      int64                `json:"version"`
	Name         string               `json:"name"`
	Provider     string               `json:"provider"`
	Status       domain.BindingStatus `json:"status"`
	Revision     int64                `json:"revision"`
	Capabilities domain.Capabilities  `json:"capabilities"`
}

func choice(b domain.BindingVersion) bindingChoice {
	return bindingChoice{ID: b.ID, Version: b.Version, Name: b.Name, Provider: b.ProviderKind, Status: b.Status, Revision: b.Revision, Capabilities: b.Capabilities}
}
func (s *Server) handleDeliveryProviders(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.sessionIdentity(r); !ok {
		writeCode(w, codeUnauthorized, "")
		return
	}
	if s.delivery == nil {
		writeCode(w, codeServiceUnavailable, "")
		return
	}
	providers, err := s.delivery.Providers(r.Context())
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": providers})
}
func (s *Server) handleListDeliveryBindings(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.sessionIdentity(r)
	if !ok {
		writeCode(w, codeUnauthorized, "")
		return
	}
	if s.delivery == nil {
		writeCode(w, codeServiceUnavailable, "")
		return
	}
	bindings, err := s.delivery.List(r.Context(), actor.OrganizationKey)
	if err != nil {
		s.writeError(w, err)
		return
	}
	out := []bindingChoice{}
	for _, b := range bindings {
		out = append(out, choice(b))
	}
	writeJSON(w, http.StatusOK, map[string]any{"bindings": out})
}
func (s *Server) handleCreateDeliveryBinding(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	if s.delivery == nil {
		writeCode(w, codeServiceUnavailable, "")
		return
	}
	var req struct {
		PreviousVersion        int64 `json:"previousVersion"`
		ExpectedRevision       int64 `json:"expectedRevision"`
		RetainWriterCredential bool  `json:"retainWriterCredential"`

		Name                     string            `json:"name"`
		Provider                 string            `json:"provider"`
		ManagementAccountKey     string            `json:"managementAccountKey"`
		ManagementAccountVersion int64             `json:"managementAccountVersion"`
		Config                   map[string]string `json:"config"`
		WriterUsername           string            `json:"writerUsername"`
		WriterPassword           string            `json:"writerPassword"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&req) != nil {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	bindingID := r.PathValue("binding")
	if (bindingID != "" && (req.PreviousVersion < 1 || req.ExpectedRevision < 1)) || (bindingID == "" && (req.PreviousVersion != 0 || req.ExpectedRevision != 0 || req.RetainWriterCredential)) {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	b, err := s.delivery.Register(r.Context(), org, appdelivery.RegisterBinding{BindingID: bindingID, PreviousVersion: req.PreviousVersion, ExpectedRevision: req.ExpectedRevision, RetainWriterCredential: req.RetainWriterCredential, Name: req.Name, ProviderKind: req.Provider, ManagementAccountKey: req.ManagementAccountKey, ManagementAccountVersion: req.ManagementAccountVersion, Config: req.Config, WriterUsername: req.WriterUsername, WriterPassword: req.WriterPassword})
	if err != nil {
		if errors.Is(err, domain.ErrInvalid) || errors.Is(err, domain.ErrUnsupported) {
			writeCode(w, codeInvalidRequest, "")
			return
		}
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, choice(b))
}

func (s *Server) handleDeliveryBindingDetails(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	if s.delivery == nil {
		writeCode(w, codeServiceUnavailable, "")
		return
	}
	version, err := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
	if err != nil || version < 1 {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	details, err := s.delivery.Details(r.Context(), org, r.PathValue("binding"), version)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"binding": choice(details.Binding), "config": details.Binding.Config, "managementAccountKey": details.ManagementAccountKey, "managementAccountVersion": details.Binding.ManagementAccountVersion, "credentialConfigured": details.CredentialConfigured, "usage": details.Usage, "verifiedAt": details.Binding.VerifiedAt, "reasonCode": details.Binding.ReasonCode})
}
func (s *Server) handleVerifyDeliveryBinding(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	if s.delivery == nil {
		writeCode(w, codeServiceUnavailable, "")
		return
	}
	var req struct {
		Version          int64 `json:"version"`
		ExpectedRevision int64 `json:"expectedRevision"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&req) != nil || req.Version < 1 || req.ExpectedRevision < 1 {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	b, err := s.delivery.Verify(r.Context(), org, r.PathValue("binding"), req.Version, req.ExpectedRevision)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, choice(b))
}

func (s *Server) handleInspectDeliveryProvider(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	if s.delivery == nil {
		writeCode(w, codeServiceUnavailable, "")
		return
	}
	var req struct {
		ManagementAccountKey     string            `json:"managementAccountKey"`
		ManagementAccountVersion int64             `json:"managementAccountVersion"`
		Config                   map[string]string `json:"config"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&req) != nil || req.ManagementAccountKey == "" || req.ManagementAccountVersion < 1 {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	out, err := s.delivery.Inspect(r.Context(), org, r.PathValue("provider"), req.ManagementAccountKey, req.ManagementAccountVersion, req.Config)
	if err != nil {
		writeCode(w, codeServiceUnavailable, "")
		return
	}
	writeJSON(w, http.StatusOK, out)
}
