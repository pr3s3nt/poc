package v2run

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Phases are the video phase marks of a scenario.
var Phases = []string{"PREPARE", "FORM", "PREVIEW", "TERRAFORM", "READY", "JOB", "CACHE", "CLEANUP"}

// Mark is one phase mark on the recording timeline.
type Mark struct {
	Phase   string  `json:"phase"`
	AtSec   float64 `json:"atSec"`
	Caption string  `json:"caption"`
}

func requiredPhases(scenario string) []string {
	switch scenario {
	case "E01":
		return []string{"PREPARE", "FORM", "PREVIEW", "CLEANUP"}
	case "E02":
		return []string{"PREPARE", "FORM", "PREVIEW", "TERRAFORM", "CLEANUP"}
	}
	return Phases
}

// ValidateMarks requires the scenario's phases, in order, with non-decreasing
// times and a Vietnamese caption each.
func ValidateMarks(scenario string, marks []Mark) error {
	idx := map[string]int{}
	for i, p := range Phases {
		idx[p] = i
	}
	seen := map[string]bool{}
	lastPhase, lastAt := -1, -1.0
	for _, m := range marks {
		i, ok := idx[m.Phase]
		switch {
		case !ok:
			return fmt.Errorf("unknown phase %q", m.Phase)
		case i < lastPhase:
			return fmt.Errorf("phase %s is out of order", m.Phase)
		case m.AtSec < lastAt:
			return fmt.Errorf("phase %s goes back in time", m.Phase)
		case strings.TrimSpace(m.Caption) == "":
			return fmt.Errorf("phase %s has no caption", m.Phase)
		}
		seen[m.Phase] = true
		lastPhase, lastAt = i, m.AtSec
	}
	var missing []string
	for _, p := range requiredPhases(scenario) {
		if !seen[p] {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing phase marks: %s", strings.Join(missing, ", "))
	}
	return nil
}

// Assertion is one recorded check.
type Assertion struct {
	ID    string `json:"id"`
	Phase string `json:"phase"`
	Pass  bool   `json:"pass"`
	// Source is "ui" (observed through the Console), "private-read" (read-only
	// technical query of the real resource) or "fake". Fake never counts.
	Source string `json:"source"`
	Detail string `json:"detail,omitempty"`
}

var happyCommon = []string{
	"ui.signin", "ui.connections.registered", "ui.types.registered", "ui.definitions.registered", "ui.environment.bound",
	"ui.workloads.three-saved", "preview.pure", "preview.target", "preview.pins", "preview.graph", "preview.score-k8s-bundle",
	"deploy.terraform-stages", "ready.workloads", "ready.single-postgres", "ready.single-redis", "ready.bindings",
	"job.completed-ui", "job.postgres-row", "cache.worker-write", "cache.backend-hit", "cache.ttl-expiry-miss", "cache.refill",
	"job.reload-same-result", "evidence.no-secret-public",
}

// awsAssertions are what the V2-A02/A03 scenario records (terraform-v2-aws.mjs) plus the
// read-only preflight verdicts of the runner. The optional ACCEPTANCE_SECRET binding is
// NOT configured on AWS: its row must show not-PASS and that is recorded explicitly
// (app.secret-row-not-configured); PGPASSWORD and REDIS_PASSWORD stay secretKeyRef.
var awsAssertions = []string{
	"aws.preflight-account-region-principal", "aws.egress-32", "aws.pod-capacity",
	"ui.signin", "ui.connections.registered", "ui.types.registered", "ui.definitions.registered", "ui.environment.bound",
	"ui.workloads.three-saved", "preview.pure", "preview.target", "preview.pins", "preview.graph", "preview.score-k8s-bundle",
	"deploy.terraform-stages", "ready.aws-assets-tagged", "ready.ownership-recorded", "ready.workloads", "ready.nodes", "ready.shared-endpoints", "ready.bindings",
	"app.rows-pass", "app.secret-row-not-configured", "technical.pods-owned-and-removed", "aws.preexisting-preserved", "job.completed-ui", "job.postgres-row", "cache.worker-write", "cache.backend-hit", "cache.ttl-expiry-miss", "cache.refill",
	"job.reload-same-result", "cleanup.supervisor-clean", "cleanup.aws-absent", "evidence.no-secret-public",
}

var v05Assertions = []string{
	"aws.preflight-account-region-principal", "aws.egress-32", "aws.pod-capacity",
	"ui.signin", "ui.account.ready", "ui.types.registered", "ui.definitions.registered", "ui.workloads.three-saved",
	"preview.pure", "preview.accounts-regions", "preview.unknown-cluster-outputs", "preview.graph", "preview.pins", "preview.score-k8s-bundle", "preview.no-environment-target",
	"deploy.terraform-stages", "deploy.cluster-ready-before-consumers", "ready.ownership-recorded", "ready.aws-assets-tagged", "ready.workloads", "ready.nodes", "ready.shared-endpoints", "ready.bindings", "ready.target-identity", "persist.no-credential",
	"app.rows-pass", "app.secret-row-not-configured", "job.completed-ui", "job.postgres-row", "job.no-duplicate", "cache.worker-write", "cache.backend-hit", "cache.ttl-expiry-miss", "cache.refill", "job.reload-same-result",
	"update.data-and-worker-preserved", "update.config-rollout", "update.no-duplicate-infra", "restart.operation-after", "restart.target-unchanged", "restart.job-persisted", "restart.new-job",
	"negative.tied-cluster-definitions", "negative.no-extra-cluster", "cleanup.supervisor-clean", "cleanup.aws-absent", "aws.preexisting-preserved", "evidence.no-secret-public",
}

var deleteAssertions = []string{
	"delete.workloads-removed", "delete.resources-unreferenced", "delete.developer-has-no-button", "delete.root-only-checked",
	"delete.platform-excluded", "delete.dependency-selected", "delete.unchecked-candidate-left", "delete.exact-name-confirmation",
	"delete.operation-stages", "delete.selected-absent", "delete.unchecked-retained-present", "delete.platform-present",
	"cleanup.owned-after-ui-by-safety-net", "cleanup.namespace-absent", "cleanup.sentinel-preserved",
}

var scenarioAssertions = map[string][]string{
	"E01": {"ui.signin", "ui.definitions.registered", "e01.valid-schema-invalid-param", "e01.field-error-in-ui",
		"e01.no-state-apply", "e01.no-provider-create", "e01.no-owned-resources", "evidence.no-secret-public"},
	"E02": {"ui.signin", "ui.definitions.registered", "e02.image-syntax-valid", "e02.digest-nonexistent-in-registry", "e02.namespace-created",
		"e02.statefulset-not-ready", "e02.terraform-node-failed", "e02.state-and-provenance-preserved", "e02.cleanup-created-by-run-only",
		"cleanup.namespace-absent", "cleanup.sentinel-preserved", "evidence.no-secret-public"},
}

// RequiredAssertions lists the assertion IDs a scenario must pass.
func RequiredAssertions(scenario string) []string {
	var out []string
	switch scenario {
	case "A01":
		out = append(append(out, happyCommon...), deleteAssertions...)
	case "A02", "A03":
		out = append(out, awsAssertions...)
	case "V05":
		out = append(out, v05Assertions...)
	default:
		out = append(out, scenarioAssertions[scenario]...)
	}
	sort.Strings(out)
	return out
}

// ValidateAssertions requires every assertion of the scenario, once, passing,
// from a real source, in the right phase vocabulary.
func ValidateAssertions(scenario string, got []Assertion) error {
	required := RequiredAssertions(scenario)
	if len(required) == 0 {
		return fmt.Errorf("unknown scenario %q", scenario)
	}
	seen := map[string]Assertion{}
	var errs []string
	for _, a := range got {
		if _, dup := seen[a.ID]; dup {
			errs = append(errs, "duplicate assertion "+a.ID)
		}
		seen[a.ID] = a
		switch {
		case a.Source != "ui" && a.Source != "private-read":
			errs = append(errs, fmt.Sprintf("%s: source %q does not count as live proof", a.ID, a.Source))
		case !a.Pass:
			errs = append(errs, a.ID+" failed")
		}
		valid := false
		for _, p := range Phases {
			valid = valid || p == a.Phase
		}
		if !valid {
			errs = append(errs, fmt.Sprintf("%s: unknown phase %q", a.ID, a.Phase))
		}
	}
	for _, id := range required {
		if _, ok := seen[id]; !ok {
			errs = append(errs, "missing assertion "+id)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.New(strings.Join(errs, "; "))
}
