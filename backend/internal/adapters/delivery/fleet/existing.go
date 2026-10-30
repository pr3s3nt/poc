package fleet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	k8s "orchestrator/internal/adapters/kubernetes"
	port "orchestrator/internal/ports/delivery"
	gitport "orchestrator/internal/ports/gitpublication"
	"path"
	"strings"
	"time"
)

// checkExisting proves the preexisting controller incarnation and its assigned
// publication scope. This never replaces/labels/deletes the existing GitRepo.
func checkExisting(ctx context.Context, cli k8s.CLI, cfg Config) (map[string]any, error) {
	repo, found, err := cli.Get(ctx, cfg.Workspace, "gitrepos.fleet.cattle.io", cfg.ExistingGitRepo)
	if err != nil || !found {
		return nil, ErrController
	}
	md, _ := repo["metadata"].(map[string]any)
	spec, _ := repo["spec"].(map[string]any)
	if md["uid"] != cfg.ExistingGitRepoUID || spec["repo"] != cfg.RepositoryURL || spec["branch"] != cfg.Branch {
		return nil, ErrOwnership
	}
	if rev, _ := spec["revision"].(string); rev != "" {
		return nil, ErrConfig
	}
	readSecret, _ := spec["clientSecretName"].(string)
	if readSecret != cfg.ReadSecret {
		return nil, ErrConfig
	}
	targets, _ := spec["targets"].([]any)
	if len(targets) == 0 {
		if cfg.Workspace != "fleet-local" || cfg.TargetCluster != "local" {
			return nil, ErrConfig
		}
	} else {
		if len(targets) != 1 {
			return nil, ErrConfig
		}
		target, _ := targets[0].(map[string]any)
		if target["clusterName"] != cfg.TargetCluster || len(target) != 1 {
			return nil, ErrConfig
		}
	}
	watched, _ := spec["paths"].([]any)
	covered := false
	for _, raw := range watched {
		watch, ok := raw.(string)
		if !ok {
			continue
		}
		watch = strings.TrimSuffix(watch, "/")
		if watch != "" && (watch == "." || cfg.Root == watch || strings.HasPrefix(cfg.Root, watch+"/")) {
			covered = true
		}
	}
	if !covered {
		return nil, ErrConfig
	}
	return repo, nil
}
func bundleRelease(bundle map[string]any) string {
	spec, _ := bundle["spec"].(map[string]any)
	helm, _ := spec["helm"].(map[string]any)
	release, _ := helm["releaseName"].(string)
	return release
}
func (m *Manager) submitExisting(ctx context.Context, i port.Intent, p Package, git gitport.Receipt, cli k8s.CLI, cfg Config) (port.Receipt, error) {
	repo, err := checkExisting(ctx, cli, cfg)
	if err != nil {
		return port.Receipt{}, err
	}
	md, _ := repo["metadata"].(map[string]any)
	uid, _ := md["uid"].(string)
	inventory, _ := json.Marshal(pkgObjects(p))
	return port.Receipt{IntentID: i.ID, RevisionID: i.Revision.ID, PayloadDigest: git.TreeDigest, Handle: map[string]string{"gitrepo": cfg.ExistingGitRepo, "gitrepoUID": uid, "commit": git.Commit, "path": p.Publication.Path, "release": p.ReleaseName, "namespace": i.Target.Namespace, "releaseNamespace": cfg.ReleaseNamespace, "inventory": string(inventory), "gitRepoMode": "EXISTING"}}, nil
}
func (p *Provider) withdrawExistingPublication(ctx context.Context, i port.Intent, r port.Receipt, cfg Config) error {
	if p.Manager.Guard == nil || r.Handle["gitrepo"] != cfg.ExistingGitRepo || r.Handle["gitrepoUID"] != cfg.ExistingGitRepoUID || r.Handle["path"] != path.Join(cfg.Root, r.Handle["release"]) {
		return ErrOwnership
	}
	cli, close, _, err := p.Manager.open(ctx, i)
	if err != nil {
		return err
	}
	defer close()
	if _, err := checkExisting(ctx, cli, cfg); err != nil {
		return err
	}
	publisher, err := p.Publishers.ResolvePublisher(ctx, i.Binding)
	if err != nil {
		return err
	}
	dest := sha256.Sum256([]byte(cfg.RepositoryURL + "\x00" + cfg.Branch))
	_, err = publisher.Publish(ctx, gitport.Intent{ID: i.ID, DestinationKey: hex.EncodeToString(dest[:]), Branch: cfg.Branch, Path: r.Handle["path"], Files: map[string][]byte{}, TreeDigest: gitport.Digest(nil), Delete: true}, func(ctx context.Context) error { return p.Manager.Guard(ctx, i) })
	return err
}

func (m *Manager) withdrawExisting(ctx context.Context, i port.Intent, r port.Receipt, cli k8s.CLI, cfg Config) (port.WithdrawalProof, error) {
	proof := port.WithdrawalProof{Receipt: r}
	if r.Handle["gitrepoUID"] != cfg.ExistingGitRepoUID {
		return proof, ErrOwnership
	}
	if _, err := checkExisting(ctx, cli, cfg); err != nil {
		return proof, err
	}
	timeout := m.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	wait, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := m.Guard(wait, i); err != nil {
			return proof, err
		}
		raw, err := cli.Run(wait, nil, "get", "bundles.fleet.cattle.io", "-n", cfg.Workspace, "-l", "fleet.cattle.io/repo-name="+cfg.ExistingGitRepo, "-o", "json")
		var bundles struct{ Items []map[string]any }
		if err != nil || json.Unmarshal(raw, &bundles) != nil {
			return proof, ErrController
		}
		live := false
		for _, b := range bundles.Items {
			if bundleRelease(b) == r.Handle["release"] {
				live = true
			}
		}
		if !live {
			break
		}
		select {
		case <-wait.Done():
			return proof, ErrController
		case <-ticker.C:
		}
	}
	if err := m.waitReleaseGone(wait, cli, cfg, i, r); err != nil {
		return proof, err
	}

	proof.ManagementStopped = true
	return proof, nil
}
