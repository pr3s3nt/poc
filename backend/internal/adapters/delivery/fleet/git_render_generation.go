package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math"
	"strings"

	k8s "orchestrator/internal/adapters/kubernetes"
	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
)

func currentGitRenderGeneration(spec map[string]any) (int64, error) {
	value, exists := spec["forceSyncGeneration"]
	if !exists {
		return 0, nil
	}
	var generation int64
	switch v := value.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || v != math.Trunc(v) || v < 0 || v > math.MaxInt32 {
			return 0, ErrConfig
		}
		generation = int64(v)
	case int64:
		generation = v
	case int:
		generation = int64(v)
	default:
		return 0, ErrConfig
	}
	if generation < 0 || generation >= math.MaxInt32 {
		return 0, ErrConfig
	}
	return generation, nil
}

// Fleet's Git render generation is not Helm's force flag. A new admitted
// non-Job revision must render fresh content; replaying an intent never bumps it.
func gitRenderGeneration(current map[string]any, exists bool, i port.Intent, commit string) (int64, error) {
	if !exists {
		return 0, nil
	}
	metadata, _ := current["metadata"].(map[string]any)
	annotations, _ := metadata["annotations"].(map[string]any)
	spec, _ := current["spec"].(map[string]any)
	generation, err := currentGitRenderGeneration(spec)
	if err != nil {
		return 0, err
	}
	if annotations["orchestrator.io/delivery-intent"] == i.ID && spec["revision"] == commit {
		return generation, nil
	}
	if i.Revision.Phase == domain.Job {
		return 0, domain.ErrUnsupported
	}
	return generation + 1, nil
}

const gitRenderRecoveryAnnotation = "orchestrator.io/git-render-recovery"

func (p *Provider) wakeGitRender(ctx context.Context, cli k8s.CLI, cfg Config, i port.Intent, r port.Receipt, pkg Package, repo map[string]any) error {
	status, _ := repo["status"].(map[string]any)
	if status["commit"] != r.Handle["commit"] {
		return nil
	}
	raw, err := cli.Run(ctx, nil, "get", "bundles.fleet.cattle.io", "-n", cfg.Workspace, "-l", "fleet.cattle.io/repo-name="+r.Handle["gitrepo"], "-o", "json")
	if err != nil {
		return ErrController
	}
	var bundles struct{ Items []map[string]any }
	if json.Unmarshal(raw, &bundles) != nil || len(bundles.Items) != 1 {
		return ErrController
	}
	metadata, _ := bundles.Items[0]["metadata"].(map[string]any)
	labels, _ := metadata["labels"].(map[string]any)
	if !owns(bundles.Items[0], i, r.Handle["gitrepo"]) {
		return ErrOwnership
	}
	if labels["fleet.cattle.io/commit"] == r.Handle["commit"] {
		return nil
	}
	patch, needed, err := gitRenderRecoveryPatch(repo, cfg, i, r, pkg.Publication.Path)
	if err != nil || !needed {
		return err
	}
	if err := p.Manager.RecoveryGuard(ctx, i); err != nil {
		return err
	}
	if _, err := cli.Run(ctx, nil, "patch", "gitrepos.fleet.cattle.io", r.Handle["gitrepo"], "-n", cfg.Workspace, "--type=json", "--patch", string(patch)); err != nil {
		return ErrController
	}
	return nil
}

func gitRenderRecoveryPatch(repo map[string]any, cfg Config, i port.Intent, r port.Receipt, path string) ([]byte, bool, error) {
	if i.Revision.Phase == domain.Job {
		return nil, false, domain.ErrUnsupported
	}
	metadata, _ := repo["metadata"].(map[string]any)
	annotations, _ := metadata["annotations"].(map[string]any)
	spec, _ := repo["spec"].(map[string]any)
	uid, _ := metadata["uid"].(string)
	version, _ := metadata["resourceVersion"].(string)
	paths, _ := spec["paths"].([]any)
	targets, _ := spec["targets"].([]any)
	if uid == "" || version == "" || uid != r.Handle["gitrepoUID"] || spec["revision"] != r.Handle["commit"] || spec["repo"] != cfg.RepositoryURL || len(paths) != 1 || paths[0] != path || len(targets) != 1 || spec["clientSecretName"] != cfg.ReadSecret {
		return nil, false, ErrOwnership
	}
	target, _ := targets[0].(map[string]any)
	if len(target) != 1 || target["clusterName"] != cfg.TargetCluster || annotations == nil || annotations["orchestrator.io/delivery-intent"] != i.ID || controllerRevision(repo) != i.Revision.ID {
		return nil, false, ErrOwnership
	}
	if annotations[gitRenderRecoveryAnnotation] == i.ID {
		return nil, false, nil
	}
	generation, err := currentGitRenderGeneration(spec)
	if err != nil {
		return nil, false, err
	}
	patch, err := json.Marshal([]map[string]any{
		{"op": "test", "path": "/metadata/uid", "value": uid},
		{"op": "test", "path": "/metadata/resourceVersion", "value": version},
		{"op": "test", "path": "/spec/revision", "value": r.Handle["commit"]},
		{"op": "add", "path": "/spec/forceSyncGeneration", "value": generation + 1},
		{"op": "add", "path": "/metadata/annotations/orchestrator.io~1git-render-recovery", "value": i.ID},
	})
	return patch, true, err
}

// Shared controllers must first render the admitted commit before a downstream
// Bundle can be woken. Validate all sibling trees before calling this helper.
func (p *Provider) wakeSharedGitRender(ctx context.Context, cli k8s.CLI, cfg Config, i port.Intent, r port.Receipt, repo map[string]any, pins map[string]unitPin) (bool, error) {
	log.Printf("shared Git render recovery: stage=begin")
	raw, err := cli.Run(ctx, nil, "get", "bundles.fleet.cattle.io", "-n", cfg.Workspace, "-l", "fleet.cattle.io/repo-name="+r.Handle["gitrepo"], "-o", "json")
	if err != nil {
		code := "COMMAND"
		if errors.Is(err, k8s.ErrForbidden) {
			code = "FORBIDDEN"
		}
		log.Printf("shared Git render recovery: stage=list code=%s", code)
		return false, ErrController
	}
	var bundles struct{ Items []map[string]any }
	if json.Unmarshal(raw, &bundles) != nil {
		return false, ErrController
	}
	matched := selectRelease(bundles.Items, r.Handle["release"])
	if len(matched) > 1 {
		return false, ErrOwnership
	}
	spec, _ := repo["spec"].(map[string]any)
	commit, _ := spec["revision"].(string)
	if len(matched) == 1 {
		if !owns(matched[0], i, r.Handle["release"]) {
			return false, ErrOwnership
		}
		md, _ := matched[0]["metadata"].(map[string]any)
		labels, _ := md["labels"].(map[string]any)
		if labels["fleet.cattle.io/commit"] == commit {
			return false, nil
		}
	}
	patch, needed, err := sharedGitRenderRecoveryPatch(repo, i, r, pins)
	if err != nil {
		return false, err
	}
	if needed {
		if err := p.Manager.RecoveryGuard(ctx, i); err != nil {
			return false, err
		}
		if _, err := cli.Run(ctx, nil, "patch", "gitrepos.fleet.cattle.io", r.Handle["gitrepo"], "-n", cfg.Workspace, "--type=json", "--patch", string(patch)); err != nil {
			code := "COMMAND"
			if errors.Is(err, k8s.ErrForbidden) {
				code = "FORBIDDEN"
			}
			if errors.Is(err, k8s.ErrRequestRejected) {
				code = "REJECTED"
			}
			if errors.Is(err, k8s.ErrUnreachable) {
				code = "UNREACHABLE"
			}
			log.Printf("shared Git render recovery patch refused: code=%s", code)
			return false, ErrController
		}
	}
	return true, nil
}
func sharedGitRenderRecoveryPatch(repo map[string]any, i port.Intent, r port.Receipt, pins map[string]unitPin) ([]byte, bool, error) {
	md, _ := repo["metadata"].(map[string]any)
	spec, _ := repo["spec"].(map[string]any)
	annotations, _ := md["annotations"].(map[string]any)
	commit, _ := spec["revision"].(string)
	if md["uid"] != r.Handle["gitrepoUID"] || md["resourceVersion"] == nil || commit == "" || !sharedReceiptMatches(pins, i, r) {
		return nil, false, ErrOwnership
	}
	for _, pin := range pins {
		if !pin.Removed && strings.HasSuffix(pin.Path, "/job") {
			return nil, false, domain.ErrUnsupported
		}
	}
	marker := i.ID + "/" + commit
	if annotations[gitRenderRecoveryAnnotation] == marker {
		return nil, false, nil
	}
	generation, err := currentGitRenderGeneration(spec)
	if err != nil {
		return nil, false, err
	}
	updated := map[string]any{}
	for k, v := range annotations {
		updated[k] = v
	}
	updated[gitRenderRecoveryAnnotation] = marker
	patch, err := json.Marshal([]map[string]any{
		{"op": "test", "path": "/metadata/uid", "value": md["uid"]},
		{"op": "test", "path": "/metadata/resourceVersion", "value": md["resourceVersion"]},
		{"op": "test", "path": "/spec/revision", "value": commit},
		{"op": "add", "path": "/spec/forceSyncGeneration", "value": generation + 1},
		{"op": "add", "path": "/metadata/annotations", "value": updated},
	})
	return patch, true, err
}
