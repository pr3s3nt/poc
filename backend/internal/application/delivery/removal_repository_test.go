package delivery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"orchestrator/internal/adapters/store"
	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"testing"
)

type repositoryRemovalProvider struct{ *neutralProvider }

func (p *repositoryRemovalProvider) RepositoryPolicy(domain.BindingVersion) (domain.RepositoryPolicy, error) {
	return domain.RepositoryPolicy{Enabled: true, Host: "github", APIBase: "https://api.github.com", CloneBase: "https://github.com", Owner: "platform", Prefix: "orch-app-", Root: "environments"}, nil
}
func (p *repositoryRemovalProvider) BindApplicationRepository(b domain.BindingVersion, r domain.ApplicationRepository, env string) (domain.BindingVersion, error) {
	b = b.Clone()
	if b.Config == nil {
		b.Config = map[string]string{}
	}
	b.Config["repositoryURL"] = r.CloneURL
	b.Config["branch"] = r.Branch
	b.Config["applicationID"] = r.ApplicationID
	b.Config["root"] = "environments/" + env
	return b, nil
}
func (p *repositoryRemovalProvider) PublicationDestination(b domain.BindingVersion) (string, error) {
	if b.Config["repositoryURL"] == "" {
		return "", domain.ErrInvalid
	}
	h := sha256.Sum256([]byte(b.Config["repositoryURL"] + b.Config["branch"]))
	return hex.EncodeToString(h[:]), nil
}
func (p *repositoryRemovalProvider) RemoveRetainedObjects(ctx context.Context, i port.Intent, r port.Receipt, inventory []port.AppliedObject) error {
	if i.Binding.Config["repositoryURL"] == "" || i.Binding.Config["applicationID"] == "" || i.Binding.Config["root"] != "environments/staging" {
		return domain.ErrInvalid
	}
	return p.neutralProvider.RemoveRetainedObjects(ctx, i, r, inventory)
}
func TestWorkloadRemovalUsesItsAllocatedApplicationRepository(t *testing.T) {
	c, original, ctx, req, _ := coordinatorFixture(t)
	p := &repositoryRemovalProvider{original}
	c.Registry = port.NewRegistry()
	if err := c.Registry.Register("fleet", p); err != nil {
		t.Fatal(err)
	}
	st := c.Store.(*store.Store)
	host := &repositoryHost{}
	c.Repositories = &Repositories{Store: st, Hosts: repositoryFactory{host}}
	req.Target.Extra = map[string]string{"application": req.ApplicationKey, "environment": req.Environment.Key}
	if err := c.Deploy(ctx, req); err != nil {
		t.Fatal("deploy", err)
	}
	if err := c.RemoveWorkload(ctx, req.OrganizationKey, req.Environment.ID, req.WorkloadID); err != nil {
		t.Fatal("remove must reuse application binding", err)
	}
	if len(p.removed) != 3 || host.calls != 1 {
		t.Fatalf("phases=%v repository allocations=%d", p.removed, host.calls)
	}
}
