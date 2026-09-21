package planning

import (
	"fmt"

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
}

// Values renders the flat context map used for placeholder resolution. The
// `org/app/env` names follow the Humanitec context contract; the remaining keys
// are orchestrator extensions.
func (c Context) Values() map[string]any {
	return map[string]any{
		"org.id":             c.OrganizationKey,
		"app.id":             c.App.Key,
		"env.id":             c.Env.Key,
		"org.key":            c.OrganizationKey,
		"app.key":            c.App.Key,
		"app.name":           c.App.Name,
		"app.profile":        string(c.App.Profile),
		"app.region":         c.App.Region,
		"env.key":            c.Env.Key,
		"env.name":           c.Env.Name,
		"env.type":           c.Env.Type,
		"env.namespace":      c.Env.NamespaceIdentity,
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
