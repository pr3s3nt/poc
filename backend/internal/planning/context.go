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
	// ReferenceCluster is a conformance-only mode: it restores planner-reference
	// Definition matching for the internal cluster node so the read-only
	// reference fixtures can run. Only the conformance harness sets it. The
	// product default is ADR-016 Definition placement (typed cluster/namespace
	// references and the cluster Definition's own Internal account); the
	// retired ADR-013 builtin cluster is not a product path. AWS legacy
	// Environments are handled separately (until V05).
	ReferenceCluster bool
}

// DefinitionPlaced reports whether placement comes from Definitions and
// references (ADR-016): the Environment carries no deployment Connection, so
// no profile enrichment, builtin cluster or Environment account applies.
func (c Context) DefinitionPlaced() bool {
	return !c.ReferenceCluster && c.Env.Profile != application.ProfileAWSEKS
}

// InfraName is the cloud-name stem of VPC/EKS/Aurora: `<app>-<env>` (ADR-011).
func (c Context) InfraName() string {
	return c.App.Key + "-" + c.Env.Key
}

// ResourceName is the provider-safe physical name of AWS infrastructure
// (ADR-011, superseded for the run ID by ADR-014). It is
// `orch-<app8>-<env10>-<sha256hex24>`: lowercase ASCII alphanumeric prefixes
// of the Application and Environment keys plus a digest of the full keys and
// target generation, at most 49 characters, so module suffixes such as
// `-writer` and `-cluster` stay inside the IAM (64) and Aurora (63) limits.
// The run ID is provenance only and never part of a physical name: a redeploy
// in a later run must not rename, and so replace, an existing object.
func (c Context) ResourceName() string {
	identity := c.App.Key + "\x00" + c.Env.Key
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
		// ADR-011: env.profile/env.region are the only profile/region keys.
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
