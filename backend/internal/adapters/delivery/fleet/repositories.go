package fleet

import (
	domain "orchestrator/internal/domain/delivery"
	"path"
)

func (p *Provider) RepositoryPolicy(b domain.BindingVersion) (domain.RepositoryPolicy, error) {
	if b.Config["repositoryOwner"] == "" {
		return domain.RepositoryPolicy{}, nil
	}
	policy := domain.RepositoryPolicy{Enabled: true, Host: "github", APIBase: b.Config["repositoryAPI"], CloneBase: b.Config["repositoryHost"], Owner: b.Config["repositoryOwner"], Prefix: b.Config["repositoryPrefix"], Root: b.Config["root"]}
	return policy, policy.Validate()
}
func (p *Provider) BindApplicationRepository(b domain.BindingVersion, r domain.ApplicationRepository, env string) (domain.BindingVersion, error) {
	if r.Validate() != nil || r.Status != "READY" || r.OrganizationID != b.OrganizationID || r.BindingID != b.ID || r.BindingVersion != b.Version || (env != "staging" && env != "production") {
		return b, ErrConfig
	}
	policy, err := p.RepositoryPolicy(b)
	if err != nil || !policy.Enabled {
		return b, ErrConfig
	}
	b = b.Clone()
	b.Config["repositoryURL"] = r.CloneURL
	b.Config["branch"] = r.Branch
	b.Config["root"] = path.Join(policy.Root, env, "manifests")
	b.Config["gitRepoMode"] = "MANAGED"
	delete(b.Config, "existingGitRepo")
	delete(b.Config, "existingGitRepoUID")
	return b, nil
}
