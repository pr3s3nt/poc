package fleet

import (
	"errors"
	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
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
	b.Config["root"] = path.Join(policy.Root, env)
	b.Config["gitRepoMode"] = "MANAGED"
	b.Config["applicationID"] = r.ApplicationID
	// Historical allocations retain their original hashed Fleet identity.
	delete(b.Config, "applicationRepoName")
	if applicationRepositoryPattern.MatchString(r.Name) {
		b.Config["applicationRepoName"] = r.Name
	}
	b.Config["environmentKey"] = env
	delete(b.Config, "existingGitRepo")
	delete(b.Config, "existingGitRepoUID")
	return b, nil
}

func (p *Provider) ValidateDeliveryReceipt(b domain.BindingVersion, r port.Receipt) error {
	if b.Config["applicationID"] != "" && r.Handle["layout"] != applicationLayout && r.Handle["completionOnly"] != "true" {
		return ErrLegacyApplication
	}
	return nil
}

var ErrLegacyApplication = errors.New("fleet: remove the legacy test Application before deploying with the shared Application GitRepo layout")
