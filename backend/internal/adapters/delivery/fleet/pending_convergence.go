package fleet

import (
	"context"
	"errors"
	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"strings"
)

// ConvergePending never weakens Observe's exact chain. Only the active writer
// can wake its receipted shared Git rendering, once per admitted intent/commit.
func (p *Provider) ConvergePending(ctx context.Context, i port.Intent, r port.Receipt) error {
	if i.Revision.Phase == domain.Job || r.Handle["layout"] != applicationLayout {
		return nil
	}
	if r.IntentID != i.ID || r.RevisionID != i.Revision.ID || r.PayloadDigest != i.Revision.PayloadDigest {
		return ErrOwnership
	}
	if p.Manager == nil || p.Manager.Guard == nil {
		return ErrOwnership
	}
	if err := p.Manager.Guard(ctx, i); err != nil {
		return err
	}
	cli, close, cfg, err := p.Manager.open(ctx, i)
	if err != nil {
		return err
	}
	defer close()
	if cfg.ApplicationID == "" || cfg.GitRepoMode == "EXISTING" {
		return ErrOwnership
	}
	for attempt := 0; attempt < 3; attempt++ {
		repo, found, err := cli.Get(ctx, cfg.Workspace, "gitrepos.fleet.cattle.io", r.Handle["gitrepo"])
		if err != nil || !found {
			return ErrController
		}
		md, _ := repo["metadata"].(map[string]any)
		pins, err := readUnitPins(repo)
		if err != nil || !applicationOwns(repo, i, cfg) || md["uid"] != r.Handle["gitrepoUID"] || md["deletionTimestamp"] != nil || !sharedReceiptMatches(pins, i, r) {
			return ErrOwnership
		}
		for _, pin := range pins {
			if !pin.Removed && strings.HasSuffix(pin.Path, "/job") {
				return nil
			}
		}
		spec, _ := repo["spec"].(map[string]any)
		status, _ := repo["status"].(map[string]any)
		if spec["revision"] != r.Handle["commit"] {
			return ErrOwnership
		}
		if status["commit"] != spec["revision"] {
			return nil
		}
		annotations, _ := md["annotations"].(map[string]any)
		if annotations[gitRenderRecoveryAnnotation] == i.ID+"/"+r.Handle["commit"] {
			return nil
		}
		if err := p.verifySharedTrees(ctx, i, r.Handle["commit"], pins); err != nil {
			return err
		}
		manager := *p.Manager
		manager.RecoveryGuard = p.Manager.Guard
		active := *p
		active.Manager = &manager
		_, err = active.wakeSharedGitRender(ctx, cli, cfg, i, r, repo, pins)
		if err == nil {
			return nil
		}
		// Ambiguous patch responses are re-observed via the same immutable marker;
		// the CAS and marker can advance generation at most once, even after a lost ack.
		if !errors.Is(err, ErrController) || ctx.Err() != nil {
			return err
		}
	}
	return ErrController
}
