package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"orchestrator/internal/application/preview"
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
			writeCode(w, codeRequestTooLarge, "")
			return
		}
		writeCode(w, codeInvalidRequest, "")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if !writeTooLarge(w, err) {
			writeCode(w, codeInvalidRequest, "")
		}
		return
	}
	before, err := scoreObject(req.ScoreBefore)
	if err != nil {
		writeCode(w, codeInvalidRequest, "scoreBefore")
		return
	}
	after, err := scoreObject(req.ScoreAfter)
	if err != nil {
		writeCode(w, codeInvalidRequest, "scoreAfter")
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
		s.writeError(w, err)
		return
	}
	view, err := result.Public()
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// scoreObject accepts an omitted field as absent and otherwise requires a
// JSON object; an explicit null is rejected so absence stays unambiguous.
func scoreObject(raw json.RawMessage) (map[string]any, error) {
	if raw == nil {
		return nil, nil
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, errNotScoreObject
	}
	var out map[string]any
	if err := json.Unmarshal(trimmed, &out); err != nil {
		return nil, errNotScoreObject
	}
	return out, nil
}

var errNotScoreObject = errors.New("not a Score object")
