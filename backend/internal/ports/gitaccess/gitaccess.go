// Package gitaccess defines organization-scoped, transient Git transport access.
package gitaccess

import "context"

type Access struct {
	Username string
	Token    string
	CAPEM    string
}

type Resolver interface {
	ResolveGitAccess(ctx context.Context, organization, repository string) (Access, error)
}

type organizationKey struct{}

// WithOrganization is set by authenticated application services, never a source draft.
func WithOrganization(ctx context.Context, organization string) context.Context {
	return context.WithValue(ctx, organizationKey{}, organization)
}
func Organization(ctx context.Context) string {
	value, _ := ctx.Value(organizationKey{}).(string)
	return value
}
