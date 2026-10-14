package v2run

import (
	"errors"
	"fmt"
	"time"
)

// Timeouts are the exact finite bounds a run records BEFORE the first create.
// Every one nests inside the next: readiness <= one Terraform invocation <= the
// Deploy lease <= the overall execution deadline; the cleanup steps sum to at
// most the cleanup budget. Nothing is unlimited and nothing is silently retried.
type Timeouts struct {
	TerraformReadiness  time.Duration
	TerraformInvocation time.Duration
	DeployLease         time.Duration
	Execution           time.Duration
	CleanupBudget       time.Duration
}

// PlanTimeouts derives and validates the bounds of a configuration.
func PlanTimeouts(c Config) (Timeouts, error) {
	t := Timeouts{Execution: c.Execution.Duration, CleanupBudget: c.Cleanup.Duration}
	switch c.Target {
	case TargetAWS:
		// EKS control plane, node group and add-ons alone take 15-25 minutes; one invocation may use 45, the Deploy lease the whole run.
		t.TerraformInvocation, t.TerraformReadiness, t.DeployLease = 45*time.Minute, 15*time.Minute, c.Execution.Duration
	default:
		t.TerraformInvocation, t.TerraformReadiness = 15*time.Minute, 5*time.Minute
		if c.Scenario == "E02" {
			t.TerraformReadiness = 3 * time.Minute
		}
		t.DeployLease = c.Execution.Duration
		if t.DeployLease > 45*time.Minute {
			t.DeployLease = 45 * time.Minute
		}
	}
	return t, t.Validate()
}

// Validate refuses a nesting that could cancel a step before its own bound or
// that exceeds the documented limits.
func (t Timeouts) Validate() error {
	switch {
	case t.TerraformReadiness <= 0 || t.TerraformInvocation <= 0 || t.DeployLease <= 0 || t.Execution <= 0 || t.CleanupBudget <= 0:
		return errors.New("timeouts: every bound must be positive and finite")
	case t.TerraformReadiness > t.TerraformInvocation:
		return fmt.Errorf("timeouts: readiness %s exceeds one Terraform invocation %s", t.TerraformReadiness, t.TerraformInvocation)
	case t.TerraformInvocation > t.DeployLease:
		return fmt.Errorf("timeouts: one Terraform invocation %s exceeds the Deploy lease %s", t.TerraformInvocation, t.DeployLease)
	case t.DeployLease > t.Execution:
		return fmt.Errorf("timeouts: the Deploy lease %s exceeds the overall execution deadline %s", t.DeployLease, t.Execution)
	case t.Execution > MaxExecutionAWS || t.CleanupBudget > MaxCleanup:
		return errors.New("timeouts: the documented execution/cleanup limits are exceeded")
	}
	return nil
}

// CleanupStepTimeouts splits the cleanup budget over the supervisor steps so
// that the original reverse destroy (the Terraform step) gets nearly all of it
// and the sum of all steps never exceeds the budget.
type CleanupStepTimeouts struct{ Delivery, Terraform, Status, Aux, Vault time.Duration }

// CleanupStepsFor divides a budget: delivery 10%, three registry status/verify
// steps and the aux/vault steps small fixed shares, Terraform the rest.
func CleanupStepsFor(budget time.Duration) CleanupStepTimeouts {
	delivery := budget / 10
	status := budget / 40
	aux := budget / 24
	vault := budget / 24
	terraform := budget - delivery - 3*status - aux - vault
	return CleanupStepTimeouts{Delivery: delivery, Terraform: terraform, Status: status, Aux: aux, Vault: vault}
}

// Sum is the total of one attempt of every step (delivery + terraform + 3 status + aux + vault).
func (s CleanupStepTimeouts) Sum() time.Duration {
	return s.Delivery + s.Terraform + 3*s.Status + s.Aux + s.Vault
}
