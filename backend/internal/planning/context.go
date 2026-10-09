package planning

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/planning/placeholder"
)

// Context exposes the non-secret values a plan may interpolate.
type Context struct {
	OrganizationKey string
	App             application.Application
	Env             environment.Environment
	Connection      application.Connection
	DeploymentID    string
	RunID           string
	// ReferenceCluster restores planner-reference Definition matching for the
	// internal cluster node. Only the conformance harness sets it; the product
	// default is the ADR-013 Environment-bound builtin cluster.
	ReferenceCluster bool
}

// LegacyInfrastructure reports whether the Environment keeps the
// Application-scoped VPC/EKS identity of a migrated binding.
func (c Context) LegacyInfrastructure() bool {
	return c.Env.Scope() == environment.ScopeLegacyApplication
}

// InfraName is the cloud-name stem of VPC/EKS/Aurora: the Application key for
// a legacy binding, `<app>-<env>` otherwise (ADR-011).
func (c Context) InfraName() string {
	if c.LegacyInfrastructure() {
		return c.App.Key
	}
	return c.App.Key + "-" + c.Env.Key
}

// ResourceName is the provider-safe physical name of AWS infrastructure
// (ADR-011). A LEGACY_APPLICATION binding keeps the exact `<app>-<run>` name.
// Otherwise it is `orch-<app8>-<env10>-<sha256hex24>`: lowercase ASCII
// alphanumeric prefixes of the Application and Environment keys plus a digest
// of the full keys and run ID, at most 49 characters, so module suffixes such as
// `-writer` and `-cluster` stay inside the IAM (64) and Aurora (63) limits.
func (c Context) ResourceName() string {
	if c.LegacyInfrastructure() {
		return c.App.Key + "-" + c.RunID
	}
	identity := c.App.Key + "\x00" + c.Env.Key + "\x00" + c.RunID
	if c.Env.TargetGeneration > 0 {
		// A later target generation must never reuse a provider-visible name.
		identity += "\x00g" + strconv.FormatInt(c.Env.TargetGeneration, 10)
	}
	sum := sha256.Sum256([]byte(identity))
	return "orch-" + nameStem(c.App.Key, 8, "app") + "-" + nameStem(c.Env.Key, 10, "env") + "-" + hex.EncodeToString(sum[:])[:24]
}

// nameStem keeps the leading lowercase ASCII letters and digits of key, up to
// limit characters; fallback replaces an empty result.
func nameStem(key string, limit int, fallback string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(key) {
		if b.Len() == limit {
			break
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return fallback
	}
	return b.String()
}

// InfraPath is the `@infra` identity path of AWS infrastructure.
func (c Context) InfraPath() string {
	if c.LegacyInfrastructure() {
		return PathApplications + c.App.Key
	}
	return PathEnvironments + c.App.Key + "." + c.Env.Key + environment.GenerationScopeSuffix(c.Env.TargetGeneration)
}

// Values renders the flat context map used for placeholder resolution. The
// `org/app/env` names follow the Humanitec context contract; the remaining keys
// are orchestrator extensions.
func (c Context) Values() map[string]any {
	return map[string]any{
		"org.id":   c.OrganizationKey,
		"app.id":   c.App.Key,
		"env.id":   c.Env.Key,
		"org.key":  c.OrganizationKey,
		"app.key":  c.App.Key,
		"app.name": c.App.Name,
		// ADR-011: env.profile/env.region are canonical; the app.* keys are
		// aliases of the selected Environment for existing Definitions.
		"app.profile":        string(c.Env.Profile),
		"app.region":         c.Env.Region,
		"env.profile":        string(c.Env.Profile),
		"env.region":         c.Env.Region,
		"env.connection.key": c.Env.ConnectionKey,
		"infra.name":         c.InfraName(),
		"infra.resourceName": c.ResourceName(),
		"env.key":            c.Env.Key,
		"env.name":           c.Env.Name,
		"env.type":           c.Env.Type,
		"env.namespace":      c.Env.Namespace(),
		"connection.key":     c.Connection.Key,
		"connection.kind":    string(c.Connection.Kind),
		"connection.cluster": c.Connection.ConfigString("cluster"),
		"connection.context": c.Connection.ConfigString("kubeContext"),
		"deployment.id":      c.DeploymentID,
		"run.id":             c.RunID,
	}
}

// ResolveContext implements placeholder.Resolver for context references.
func (c Context) ResolveContext(path string) (any, error) {
	v, ok := c.Values()[path]
	if !ok {
		return nil, fmt.Errorf("planning: unknown context value %q", path)
	}
	return v, nil
}

// ResolveResource rejects resource references: planning never resolves runtime outputs.
func (c Context) ResolveResource(alias, outputKey string) (any, error) {
	return nil, fmt.Errorf("planning: resource output %s.%s is only available at execution time", alias, outputKey)
}

var _ placeholder.Resolver = Context{}
