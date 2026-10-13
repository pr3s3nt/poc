package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"orchestrator/internal/application/statebackend"
	"orchestrator/internal/domain/tfresource"
	"orchestrator/internal/ports/terraformstate"
)

// TerraformStatePrefix is the internal Terraform HTTP backend route. It is
// served only by TerraformStateHandler, never by the Console API server.
const TerraformStatePrefix = "/internal/terraform/states/"

// TerraformStateHandler serves the Terraform HTTP backend protocol (OC-18) to
// executor subprocesses. It accepts only invocation capabilities presented as
// the Basic-auth password; Console sessions are not consulted. Responses carry
// state only to the authorized executor and never echo tokens, lock
// diagnostics or decryption detail. Nothing is logged.
type TerraformStateHandler struct{ states *statebackend.Service }

// NewTerraformStateHandler builds the internal state backend handler.
func NewTerraformStateHandler(states *statebackend.Service) *TerraformStateHandler {
	return &TerraformStateHandler{states: states}
}

// terraformLockInfo mirrors the Terraform lock info document.
type terraformLockInfo struct {
	ID        string `json:"ID"`
	Operation string `json:"Operation"`
	Info      string `json:"Info"`
	Who       string `json:"Who"`
	Version   string `json:"Version"`
	Created   string `json:"Created"`
	Path      string `json:"Path"`
}

func (h *TerraformStateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	// Terraform's http backend treats GET 404 as "no state yet" and would
	// then plan from empty state. Every path inside the state collection that
	// does not name a well-formed key therefore fails closed with 400; only a
	// route outside the collection is an ordinary 404.
	collection := strings.TrimSuffix(TerraformStatePrefix, "/")
	if r.URL.Path != collection && !strings.HasPrefix(r.URL.Path, collection+"/") {
		stateError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	stateKey := strings.TrimPrefix(r.URL.Path, TerraformStatePrefix)
	if !tfresource.ValidStateKey(stateKey) {
		stateError(w, http.StatusBadRequest, "INVALID_STATE_KEY")
		return
	}
	_, token, ok := r.BasicAuth()
	if !ok || token == "" {
		w.Header().Set("WWW-Authenticate", `Basic realm="orchestrator-terraform-state"`)
		stateError(w, http.StatusUnauthorized, "UNAUTHORIZED")
		return
	}
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		state, found, err := h.states.Read(ctx, token, stateKey)
		if err != nil {
			stateFailure(w, err)
			return
		}
		if !found {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(state)
	case http.MethodPost:
		body, err := io.ReadAll(io.LimitReader(r.Body, statebackend.MaxStateBytes+1))
		if err != nil {
			stateError(w, http.StatusBadRequest, "INVALID_STATE")
			return
		}
		if len(body) > statebackend.MaxStateBytes {
			stateError(w, http.StatusRequestEntityTooLarge, "STATE_TOO_LARGE")
			return
		}
		if err := h.states.Write(ctx, token, stateKey, r.URL.Query().Get("ID"), body); err != nil {
			stateFailure(w, err)
			return
		}
		w.WriteHeader(http.StatusOK)
	case "LOCK":
		info, ok := readLockInfo(w, r)
		if !ok {
			return
		}
		held, err := h.states.Lock(ctx, token, stateKey, statebackend.LockRequest{ID: info.ID, Metadata: tfresource.LockMetadata{Operation: safeLockField(info.Operation), Version: safeLockField(info.Version), Created: safeLockField(info.Created)}})
		if errors.Is(err, terraformstate.ErrLockConflict) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusLocked)
			if held != nil {
				_ = json.NewEncoder(w).Encode(terraformLockInfo{ID: held.LockID, Operation: held.Metadata.Operation, Version: held.Metadata.Version, Created: held.Metadata.Created, Who: "orchestrator-operation:" + held.OperationID})
			}
			return
		}
		if err != nil {
			stateFailure(w, err)
			return
		}
		w.WriteHeader(http.StatusOK)
	case "UNLOCK":
		info, ok := readLockInfo(w, r)
		if !ok {
			return
		}
		if err := h.states.Unlock(ctx, token, stateKey, info.ID); err != nil {
			stateFailure(w, err)
			return
		}
		w.WriteHeader(http.StatusOK)
	default:
		// DELETE included: executors have no purge permission.
		w.Header().Set("Allow", "GET, POST, LOCK, UNLOCK")
		stateError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
	}
}

func readLockInfo(w http.ResponseWriter, r *http.Request) (terraformLockInfo, bool) {
	var info terraformLockInfo
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil || json.Unmarshal(body, &info) != nil || info.ID == "" {
		stateError(w, http.StatusBadRequest, "INVALID_LOCK")
		return info, false
	}
	return info, true
}

// safeLockField keeps short printable metadata only.
func safeLockField(v string) string {
	if len(v) > 64 {
		return ""
	}
	for _, r := range v {
		if r < 0x20 || r > 0x7e {
			return ""
		}
	}
	return v
}

func stateFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, terraformstate.ErrUnauthorized):
		stateError(w, http.StatusUnauthorized, "UNAUTHORIZED")
	case errors.Is(err, terraformstate.ErrForbidden):
		stateError(w, http.StatusForbidden, "FORBIDDEN")
	case errors.Is(err, terraformstate.ErrLockMismatch):
		stateError(w, http.StatusConflict, "LOCK_MISMATCH")
	case errors.Is(err, terraformstate.ErrStateConflict):
		stateError(w, http.StatusConflict, "STATE_CONFLICT")
	case errors.Is(err, terraformstate.ErrNoAttempt):
		stateError(w, http.StatusConflict, "NO_RUNNING_ATTEMPT")
	case errors.Is(err, terraformstate.ErrInvalidState):
		stateError(w, http.StatusBadRequest, "INVALID_STATE")
	case errors.Is(err, terraformstate.ErrUndecryptable):
		stateError(w, http.StatusInternalServerError, "STATE_UNDECRYPTABLE")
	default:
		stateError(w, http.StatusInternalServerError, "INTERNAL")
	}
}

func stateError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":"` + code + `"}`))
}

// TerraformStateLoopback is a running loopback listener of the state handler.
type TerraformStateLoopback struct {
	// Address is the backend base URL for Terraform's http backend address.
	Address string
	server  *http.Server
	done    chan struct{}
}

// ServeTerraformStateLoopback serves the handler on 127.0.0.1 with an
// ephemeral port and bounded timeouts. It never binds a non-loopback address;
// serving beyond loopback needs TLS (state contract) and is not provided.
func ServeTerraformStateLoopback(handler *TerraformStateHandler) (*TerraformStateLoopback, error) {
	if handler == nil {
		return nil, errors.New("http: the Terraform state backend is not configured")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, errors.New("http: cannot listen on loopback for the Terraform state backend")
	}
	l := &TerraformStateLoopback{
		Address: "http://" + listener.Addr().String() + TerraformStatePrefix,
		server:  &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 2 * time.Minute, WriteTimeout: 2 * time.Minute, IdleTimeout: time.Minute, ErrorLog: log.New(io.Discard, "", 0)},
		done:    make(chan struct{}),
	}
	go func() {
		defer close(l.done)
		_ = l.server.Serve(listener)
	}()
	return l, nil
}

// Close stops the listener and waits for in-flight requests up to ctx.
func (l *TerraformStateLoopback) Close(ctx context.Context) error {
	err := l.server.Shutdown(ctx)
	<-l.done
	return err
}
