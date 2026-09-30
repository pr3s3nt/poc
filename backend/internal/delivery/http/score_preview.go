package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"orchestrator/internal/application/preview"
	"orchestrator/internal/ports/persistence"
)

// scorePreviewLimit bounds one standalone Preview request body.
const scorePreviewLimit = 3 << 20

// scorePreviewRequest is the strict UC-05 request. Scores stay raw until the
// handler checks that a present value is a JSON object and never null.
type scorePreviewRequest struct {
	WorkloadID  string          `json:"workloadId"`
	Action      string          `json:"action"`
	RunID       string          `json:"runId"`
	ScoreBefore json.RawMessage `json:"scoreBefore"`
	ScoreAfter  json.RawMessage `json:"scoreAfter"`
}

func (s *Server) handleScorePreview(w http.ResponseWriter, r *http.Request) {
	app, env, ok := s.authorizedConfiguration(w, r)
	if !ok {
		return
	}
	identity, _ := s.sessionIdentity(r)
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, scorePreviewLimit))
	decoder.DisallowUnknownFields()
	var req scorePreviewRequest
	if err := decoder.Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "request body is too large"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body: " + safeDecodeMessage(err)})
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "request body must contain exactly one JSON object"})
		return
	}
	before, err := scoreObject("scoreBefore", req.ScoreBefore)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	after, err := scoreObject("scoreAfter", req.ScoreAfter)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	result, err := s.previews.PreviewDeployment(r.Context(), preview.PreviewDeploymentQuery{
		OrganizationKey: identity.OrganizationKey,
		ApplicationKey:  app,
		EnvironmentKey:  env,
		WorkloadID:      req.WorkloadID,
		Action:          req.Action,
		RunID:           req.RunID,
		ScoreBefore:     before,
		ScoreAfter:      after,
	})
	if err != nil {
		writeScorePreviewError(w, err)
		return
	}
	view, err := result.Public()
	if err != nil {
		writeScorePreviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// scoreObject accepts an omitted field as absent and otherwise requires a
// JSON object; an explicit null is rejected so absence stays unambiguous.
func scoreObject(field string, raw json.RawMessage) (map[string]any, error) {
	if raw == nil {
		return nil, nil
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, errors.New(field + " must be a Score object; omit it when absent")
	}
	var out map[string]any
	if err := json.Unmarshal(trimmed, &out); err != nil {
		return nil, errors.New(field + " must be a Score object")
	}
	return out, nil
}

// safeDecodeMessage keeps decoder diagnostics that name a field or syntax
// position, without echoing request values.
func safeDecodeMessage(err error) string {
	var syntax *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.As(err, &syntax):
		return "malformed JSON"
	case errors.As(err, &typeErr):
		return typeErr.Field + " has the wrong type"
	case errors.Is(err, io.EOF):
		return "empty body"
	}
	// DisallowUnknownFields reports `json: unknown field "name"`.
	if message := err.Error(); strings.HasPrefix(message, "json: unknown field ") {
		return strings.TrimPrefix(message, "json: ")
	}
	return "malformed JSON"
}

// writeScorePreviewError maps UC-05 errors. Only *preview.PublicError
// messages are returned; they never carry catalog, module or store detail.
// Anything else is a generic retryable 500, and the raw error is not logged
// because store or adapter errors may quote configuration content.
func writeScorePreviewError(w http.ResponseWriter, err error) {
	var public *preview.PublicError
	switch {
	case errors.Is(err, persistence.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	case errors.As(err, &public):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": public.Message})
	default:
		log.Printf("score preview failed: internal error (%T)", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "preview failed; retry"})
	}
}
