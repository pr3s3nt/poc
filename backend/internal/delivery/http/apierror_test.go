package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appcreate "orchestrator/internal/application/application"
	"orchestrator/internal/application/authentication"
	"orchestrator/internal/application/catalog"
	appconfig "orchestrator/internal/application/configuration"
	connectionapp "orchestrator/internal/application/connection"
	"orchestrator/internal/application/pending"
	"orchestrator/internal/application/preview"
	"orchestrator/internal/application/secretstores"
	"orchestrator/internal/application/transition"
	"orchestrator/internal/application/workloadconfig"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/planning"
	"orchestrator/internal/planning/score"
	configport "orchestrator/internal/ports/configuration"
	"orchestrator/internal/ports/persistence"
)

const canary = "canary-secret-VALUE-9f3"

func capture(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previous) })
	return &logs
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("not JSON: %s", rec.Body.String())
	}
	return body
}

func TestWriteErrorCodesStatusesAndFields(t *testing.T) {
	logs := capture(t)
	cleanup := &connectionapp.CleanupError{Reference: "ref-1"}
	cases := []struct {
		name   string
		err    error
		status int
		code   string
		field  string
	}{
		{"score invalid", fmt.Errorf("score: %s bad: %w", canary, score.ErrInvalid), 400, "INVALID_SCORE", ""},
		{"removed publicPort", fmt.Errorf("score: service.publicPort: %w", environment.ErrPublicPortRemoved), 400, "INVALID_SCORE", "service.publicPort"},
		{"workload score", fmt.Errorf("%w: %s", workloadconfig.ErrInvalid, canary), 400, "INVALID_SCORE", ""},
		{"workload variable", fmt.Errorf("x %s: %w", canary, workloadconfig.ErrInvalidVariable), 400, "INVALID_VARIABLE", ""},
		{"config invalid", fmt.Errorf("%s: %w", canary, appconfig.ErrInvalid), 400, "INVALID_VARIABLE", ""},
		{"config conflict", fmt.Errorf("%s: %w", canary, appconfig.ErrConflict), 409, "DUPLICATE", ""},
		{"catalog duplicate", fmt.Errorf("%w: %q", catalog.ErrDuplicate, canary), 409, "DUPLICATE", ""},
		{"app duplicate field", &appcreate.FieldError{Field: "subdomain", Err: appcreate.ErrDuplicate}, 409, "DUPLICATE", "subdomain"},
		{"app invalid field", &appcreate.FieldError{Field: "name", Err: appcreate.ErrInvalid}, 400, "INVALID_REQUEST", "name"},
		{"app unknown field dropped", &appcreate.FieldError{Field: canary, Err: appcreate.ErrInvalid}, 400, "INVALID_REQUEST", ""},
		{"catalog id", fmt.Errorf("%w: %q", catalog.ErrInvalid, canary), 400, "INVALID_REQUEST", ""},
		{"target not ready", &appcreate.FieldError{Field: "connectionKey", Err: appcreate.ErrTargetNotReady}, 422, "CONNECTION_UNAVAILABLE", "connectionKey"},
		{"stale version", fmt.Errorf("row %s: %w", canary, persistence.ErrVersionConflict), 409, "STALE_VERSION", ""},
		{"stale preview", pending.ErrStalePreview, 409, "STALE_PREVIEW", ""},
		{"stale token", transition.ErrStaleToken, 409, "STALE_PREVIEW", ""},
		{"busy", persistence.Busy("a", "staging"), 409, "ENVIRONMENT_BUSY", ""},
		{"not found", fmt.Errorf("%s: %w", canary, persistence.ErrNotFound), 404, "NOT_FOUND", ""},
		{"unauthorized", authentication.ErrUnauthorized, 401, "UNAUTHORIZED", ""},
		{"credentials", authentication.ErrInvalidCredentials, 401, "INVALID_CREDENTIALS", ""},
		{"unconfigured", planning.ErrEnvironmentUnconfigured, 422, "ENVIRONMENT_UNCONFIGURED", "connectionKey"},
		{"no store", appconfig.ErrNoStore, 422, "SECRET_STORE_REQUIRED", "secretStoreKey"},
		{"store unavailable", configport.ErrStoreUnavailable, 422, "SECRET_STORE_UNAVAILABLE", "secretStoreKey"},
		{"copy failed", fmt.Errorf("%w: %s", appconfig.ErrStoreCopy, canary), 502, "SECRET_COPY_FAILED", ""},
		{"operation lost", persistence.ErrOperationLost, 409, "OPERATION_LOST", ""},
		{"route reconcile", pending.ErrRouteReconcile, 502, "ROUTE_RECONCILE_FAILED", ""},
		{"runtime exists", appcreate.ErrRuntimeExists, 409, "TRANSITION_REQUIRED", ""},
		{"verification generic", fmt.Errorf("%w: %s", connectionapp.ErrVerification, canary), 422, "VERIFICATION_FAILED", ""},
		{"secret store verification", fmt.Errorf("%w: %s", secretstores.ErrVerification, canary), 422, "VERIFICATION_FAILED", ""},
		{"credential store", connectionapp.ErrCredentialStore, 503, "SERVICE_UNAVAILABLE", ""},
		{"too large", connectionapp.ErrKubeconfigTooLarge, 413, "REQUEST_TOO_LARGE", ""},
		{"kubeconfig tls", connectionapp.ErrInvalidTLS, 400, "KUBECONFIG_INVALID_TLS", ""},
		{"kubeconfig context", connectionapp.ErrContextRequired, 400, "KUBECONFIG_CONTEXT_REQUIRED", ""},
		{"same destination", transition.ErrSameDestination, 422, "TRANSITION_SAME_DESTINATION", "destinationKey"},
		{"destination failed", transition.ErrDestinationFailed, 422, "DESTINATION_UNAVAILABLE", "destinationKey"},
		{"downtime", transition.ErrAcknowledge, 422, "DOWNTIME_ACK_REQUIRED", "acknowledgeDowntime"},
		{"recovery unconfirmed", persistence.ErrRecoveryUnconfirmed, 422, "RECOVERY_CONFIRMATION_REQUIRED", "priorExecutionStopped"},
		{"no runtime", transition.ErrNoRuntime, 409, "NO_RUNTIME", ""},
		{"cleanup refused", transition.ErrCleanupRefused, 409, "CLEANUP_REFUSED", ""},
		{"not recoverable", transition.ErrNotRecoverable, 409, "NOT_RECOVERABLE", ""},
		{"recovery incomplete", transition.ErrRecoveryIncomplete, 409, "RECOVERY_INCOMPLETE", ""},
		{"unsupported", &transition.UnsupportedError{Reason: canary}, 422, "TRANSITION_UNSUPPORTED", ""},
		{"matching preview field", &catalog.PreviewInvalidError{Field: "definition.criteria"}, 400, "INVALID_REQUEST", "definition.criteria"},
		{"matching preview unknown field dropped", &catalog.PreviewInvalidError{Field: canary}, 400, "INVALID_REQUEST", ""},
		{"unknown", fmt.Errorf("pq: password=%s", canary), 500, "INTERNAL_ERROR", ""},
		{"english prefix is not validation", errors.New("score: " + canary), 500, "INTERNAL_ERROR", ""},
		{"planning prefix is not validation", errors.New("planning: " + canary), 500, "INTERNAL_ERROR", ""},
		{"cleanup", cleanup, 500, "INTERNAL_ERROR", ""},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		(&Server{}).writeError(rec, tc.err)
		body := decode(t, rec)
		if rec.Code != tc.status || body["code"] != tc.code {
			t.Errorf("%s -> %d %s", tc.name, rec.Code, rec.Body.String())
		}
		if field, _ := body["field"].(string); field != tc.field {
			t.Errorf("%s field %q want %q", tc.name, field, tc.field)
		}
		if strings.Contains(rec.Body.String(), canary) {
			t.Errorf("%s leaked canary: %s", tc.name, rec.Body.String())
		}
	}
	if strings.Contains(logs.String(), canary) {
		t.Fatalf("raw cause logged: %s", logs.String())
	}
}

func TestVerificationVariantsHaveSpecificCodes(t *testing.T) {
	for want, category := range map[string]error{
		"VERIFICATION_CONTEXT_MISSING":     connectionapp.ErrContextMissing,
		"VERIFICATION_PERMISSION_DENIED":   connectionapp.ErrPermissionDenied,
		"VERIFICATION_CREDENTIAL_REJECTED": connectionapp.ErrAuthentication,
		"VERIFICATION_UNREACHABLE":         connectionapp.ErrClusterUnreachable,
	} {
		err := fmt.Errorf("%w: %w", connectionapp.ErrVerification, category)
		rec := httptest.NewRecorder()
		(&Server{}).writeError(rec, err)
		if body := decode(t, rec); rec.Code != 422 || body["code"] != want {
			t.Errorf("%s -> %d %s", want, rec.Code, rec.Body.String())
		}
	}
}

func TestPlannerStageParamsAreAllowListed(t *testing.T) {
	cases := []struct {
		err    error
		code   string
		stage  string
		reason string
	}{
		{&preview.PublicError{Kind: preview.ErrPlanningRejected, Message: canary, Stage: planning.StageCatalog}, "PLANNING_REJECTED", "catalog", ""},
		{&preview.PublicError{Kind: preview.ErrInvalidScore, Message: canary, Stage: planning.StageBefore, Reason: planning.ReasonBeforeMismatch}, "INVALID_SCORE", "before-state", "before-mismatch"},
		{&preview.PublicError{Kind: preview.ErrPlanningRejected, Message: canary, Stage: planning.Stage(canary), Reason: canary}, "PLANNING_REJECTED", "", ""},
		{&planning.StageError{Stage: planning.StageGraph, Err: errors.New(canary)}, "PLANNING_REJECTED", "graph", ""},
		{&preview.PublicError{Kind: preview.ErrInvalidRequest, Message: canary}, "INVALID_REQUEST", "", ""},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		(&Server{}).writeError(rec, tc.err)
		body := decode(t, rec)
		params, _ := body["params"].(map[string]any)
		if rec.Code != 400 || body["code"] != tc.code || params["stage"] != nilIfEmpty(tc.stage) || params["reason"] != nilIfEmpty(tc.reason) || strings.Contains(rec.Body.String(), canary) {
			t.Errorf("%v -> %d %s", tc.err, rec.Code, rec.Body.String())
		}
	}
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func TestWriteFailureFiltersParamsAndExtra(t *testing.T) {
	rec := httptest.NewRecorder()
	writeFailure(rec, failure{code: codePlanningRejected, field: canary,
		params: map[string]string{"stage": "graph", "reason": canary, "other": canary, "resourceType": "Bad_" + canary},
		extra:  map[string]any{"code": "HACKED", "error": canary, "field": canary, "params": canary, "operation": "op"}})
	body := decode(t, rec)
	params := body["params"].(map[string]any)
	if body["code"] != "PLANNING_REJECTED" || len(params) != 1 || params["stage"] != "graph" || body["operation"] != "op" || strings.Contains(rec.Body.String(), canary) {
		t.Fatalf("%s", rec.Body.String())
	}
	if _, ok := body["field"]; ok {
		t.Fatal("untrusted field echoed")
	}
}

func TestBusyKeepsOperationExtra(t *testing.T) {
	rec := httptest.NewRecorder()
	(&Server{}).writeError(rec, persistence.Busy("a", "staging"))
	if rec.Code != http.StatusConflict || decode(t, rec)["code"] != "ENVIRONMENT_BUSY" {
		t.Fatalf("%s", rec.Body.String())
	}
}

func TestBodyDecodingNeverEchoesCallerKeysOrValues(t *testing.T) {
	logs := capture(t)
	bodies := []string{
		`{"` + canary + `": 1}`,
		`{"version": "` + canary + `"}`,
		`[` + canary + `]`,
		`{"version": 1} ` + canary,
		`{"name": "` + canary,
		``,
	}
	for _, raw := range bodies {
		for name, run := range map[string]func(*httptest.ResponseRecorder, *http.Request) bool{
			"strict": func(w *httptest.ResponseRecorder, r *http.Request) bool {
				var v workloadVersionRequest
				return decodeStrict(w, r, 1<<10, &v)
			},
			"connection": func(w *httptest.ResponseRecorder, r *http.Request) bool {
				var v registerConnectionBody
				return decodeConnectionBody(w, r, map[string]bool{"name": true}, &v)
			},
			"preview": func(w *httptest.ResponseRecorder, r *http.Request) bool {
				var v catalog.MatchingPreviewQuery
				return decodePreviewBody(w, r, &v)
			},
		} {
			rec := httptest.NewRecorder()
			if run(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(raw))) {
				t.Errorf("%s accepted %q", name, raw)
				continue
			}
			body := decode(t, rec)
			if rec.Code != 400 || body["code"] != "INVALID_REQUEST" || strings.Contains(rec.Body.String(), canary) {
				t.Errorf("%s %q -> %d %s", name, raw, rec.Code, rec.Body.String())
			}
		}
	}
	rec := httptest.NewRecorder()
	var v workloadVersionRequest
	if decodeStrict(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"version":1,"x":"`+strings.Repeat("a", 2<<10)+`"}`)), 1<<10, &v) || rec.Code != 413 || decode(t, rec)["code"] != "REQUEST_TOO_LARGE" {
		t.Errorf("oversized -> %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(logs.String(), canary) {
		t.Fatalf("caller data logged: %s", logs.String())
	}
}

func TestScoreObjectRejectionIsCoded(t *testing.T) {
	if _, err := scoreObject([]byte(`null`)); err == nil {
		t.Fatal("null accepted")
	}
	if got, err := scoreObject(nil); err != nil || got != nil {
		t.Fatal("absent score must stay absent")
	}
}

func TestParamErrorNamesOnlyAValidatedRegisteredType(t *testing.T) {
	logs := capture(t)
	for _, tc := range []struct {
		typ  string
		want any
	}{
		{"postgres", "postgres"},
		{"my-cache-2", "my-cache-2"},
		{"Bad_" + canary, nil},
		{"", nil},
		{strings.Repeat("a", 64), nil},
	} {
		typed := fmt.Errorf("wrapped %s: %w", canary, &workloadconfig.ParamError{Type: tc.typ})
		rec := httptest.NewRecorder()
		(&Server{}).writeError(rec, typed)
		body := decode(t, rec)
		params, _ := body["params"].(map[string]any)
		if rec.Code != 400 || body["code"] != "INVALID_SCORE" || params["resourceType"] != tc.want || strings.Contains(rec.Body.String(), canary) {
			t.Errorf("%q -> %d %s", tc.typ, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	(&Server{}).writeError(rec, fmt.Errorf("type %s: %w", canary, workloadconfig.ErrUnknownResourceType))
	if body := decode(t, rec); rec.Code != 400 || body["field"] != "resources" || body["params"] != nil || strings.Contains(rec.Body.String(), canary) {
		t.Errorf("unknown type -> %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(logs.String(), canary) {
		t.Fatalf("canary logged: %s", logs.String())
	}
}
