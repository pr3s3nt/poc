// Package direct implements the same durable phase contract without a
// background controller. Exact rendered artifacts become future handoff proof.
package direct

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"orchestrator/internal/adapters/delivery/artifact"
	k8s "orchestrator/internal/adapters/kubernetes"
	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
)

type Provider struct {
	Deployer      execution.WorkloadDeployer
	KubectlPath   string
	Credentials   execution.KubeconfigSource
	Guard         func(context.Context, port.Intent) error
	RecoveryGuard func(context.Context, port.Intent) error
}

// RecoverSubmission validates the durable apply receipt without applying again.
// The coordinator subsequently observes the exact revision and original UIDs.
func (p *Provider) RecoverSubmission(ctx context.Context, i port.Intent, prepared port.Prepared, r port.Receipt, _ port.TransferSource) (port.Receipt, error) {
	if p.RecoveryGuard == nil {
		return r, domain.ErrUnsupported
	}
	if err := p.RecoveryGuard(ctx, i); err != nil {
		return r, err
	}
	e, err := decode(prepared.Payload)
	if err != nil {
		return r, err
	}
	sum := sha256.Sum256(prepared.Payload)
	inventory, _ := json.Marshal(refsOf(e))
	ready, _ := json.Marshal(waitRefs(e))
	if r.IntentID != i.ID || r.RevisionID != i.Revision.ID || r.PayloadDigest != i.Revision.PayloadDigest || hex.EncodeToString(sum[:]) != i.Revision.PayloadDigest || r.Handle["inventory"] != string(inventory) || r.Handle["ready"] != string(ready) {
		return r, domain.ErrInvalid
	}
	return r, nil
}

type object struct {
	APIVersion, Kind, Name, Namespace string
	Body                              map[string]any
}
type payload struct{ Objects []object }

func capabilities() domain.Capabilities {
	return domain.Capabilities{ContractVersion: "v1", ExactRevision: true, PhasedDelivery: true, RetainOnWithdraw: true, GuardedPrune: true}
}
func (p *Provider) Describe(context.Context, port.BindingVersion) (domain.Capabilities, error) {
	return capabilities(), nil
}
func (p *Provider) Verify(context.Context, port.BindingVersion) (port.Verification, error) {
	return port.Verification{Ready: true, Capabilities: capabilities(), CheckedAt: time.Now().UTC()}, nil
}
func (p *Provider) Prepare(_ context.Context, i port.Intent, manifests []execution.Manifest) (port.Prepared, error) {
	if err := i.Validate(); err != nil {
		return port.Prepared{}, err
	}
	envelope := payload{}
	refs := []execution.ObjectRef{}
	for _, m := range manifests {
		if m.Secret || m.Auxiliary || m.Kind == "Secret" || m.Namespace != i.Target.Namespace || m.Object == nil {
			return port.Prepared{}, domain.ErrInvalid
		}
		// Copy before adding delivery provenance; never alter renderer artifacts.
		raw, err := json.Marshal(m.Object)
		if err != nil {
			return port.Prepared{}, domain.ErrInvalid
		}
		var body map[string]any
		if json.Unmarshal(raw, &body) != nil {
			return port.Prepared{}, domain.ErrInvalid
		}
		md, _ := body["metadata"].(map[string]any)
		if md == nil || md["name"] != m.Name || md["namespace"] != m.Namespace || body["kind"] != m.Kind || body["apiVersion"] != m.APIVersion {
			return port.Prepared{}, domain.ErrInvalid
		}
		annotations, _ := md["annotations"].(map[string]any)
		if annotations == nil {
			annotations = map[string]any{}
		}
		annotations["orchestrator.io/delivery-revision"] = i.Revision.ID
		md["annotations"] = annotations
		envelope.Objects = append(envelope.Objects, object{m.APIVersion, m.Kind, m.Name, m.Namespace, body})
		refs = append(refs, execution.ObjectRef{APIVersion: m.APIVersion, Kind: m.Kind, Name: m.Name})
	}
	if len(envelope.Objects) == 0 {
		return port.Prepared{}, domain.ErrInvalid
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return port.Prepared{}, domain.ErrInvalid
	}
	sum := sha256.Sum256(raw)
	r := i.Revision
	r.PayloadDigest = hex.EncodeToString(sum[:])
	return port.Prepared{Revision: r, Objects: refs, Payload: raw}, nil
}
func decode(raw []byte) (payload, error) {
	var e payload
	if json.Unmarshal(raw, &e) != nil || len(e.Objects) == 0 {
		return e, domain.ErrInvalid
	}
	return e, nil
}
func refsOf(e payload) []execution.ObjectRef {
	refs := []execution.ObjectRef{}
	for _, o := range e.Objects {
		refs = append(refs, execution.ObjectRef{APIVersion: o.APIVersion, Kind: o.Kind, Name: o.Name})
	}
	return refs
}
func waitRefs(e payload) []execution.WorkloadRef {
	refs := []execution.WorkloadRef{}
	for _, o := range e.Objects {
		switch o.Kind {
		case "Deployment", "StatefulSet", "DaemonSet", "Job":
			refs = append(refs, execution.WorkloadRef{Kind: o.Kind, Name: o.Name, Namespace: o.Namespace})
		}
	}
	return refs
}
func (p *Provider) Submit(ctx context.Context, i port.Intent, prepared port.Prepared) (port.Receipt, error) {
	if p.Guard == nil || p.Deployer == nil {
		return port.Receipt{}, domain.ErrUnsupported
	}
	e, err := decode(prepared.Payload)
	if err != nil {
		return port.Receipt{}, err
	}
	sum := sha256.Sum256(prepared.Payload)
	if hex.EncodeToString(sum[:]) != i.Revision.PayloadDigest {
		return port.Receipt{}, domain.ErrInvalid
	}
	if err := p.Guard(ctx, i); err != nil {
		return port.Receipt{}, err
	}
	manifests := []execution.Manifest{}
	for _, o := range e.Objects {
		manifests = append(manifests, execution.Manifest{APIVersion: o.APIVersion, Kind: o.Kind, Name: o.Name, Namespace: o.Namespace, Object: o.Body})
	}
	if err := p.Deployer.Apply(ctx, i.Target, manifests); err != nil {
		return port.Receipt{}, err
	}
	inventory, _ := json.Marshal(refsOf(e))
	ready, _ := json.Marshal(waitRefs(e))
	return port.Receipt{IntentID: i.ID, RevisionID: i.Revision.ID, PayloadDigest: i.Revision.PayloadDigest, Handle: map[string]string{"inventory": string(inventory), "ready": string(ready)}}, nil
}
func (p *Provider) Observe(ctx context.Context, i port.Intent, r port.Receipt) (domain.Observation, error) {
	if r.Handle["completionOnly"] == "true" {
		cli, close, err := k8s.OpenCLI(ctx, p.KubectlPath, p.Credentials, i.Target)
		if err != nil {
			return domain.Observation{}, err
		}
		defer close()
		return artifact.ObserveCompletion(ctx, cli, i, r)
	}
	out := domain.Observation{Scope: i.Revision.Scope, IntendedRevision: i.Revision.ID, Sync: domain.SyncUnknown, Health: domain.HealthUnknown, Drift: domain.DriftUnsupported, ObservedAt: time.Now().UTC()}
	if r.RevisionID != i.Revision.ID || r.PayloadDigest != i.Revision.PayloadDigest {
		return out, domain.ErrInvalid
	}
	var refs []execution.ObjectRef
	var ready []execution.WorkloadRef
	if json.Unmarshal([]byte(r.Handle["inventory"]), &refs) != nil || len(refs) == 0 || json.Unmarshal([]byte(r.Handle["ready"]), &ready) != nil {
		return out, domain.ErrInvalid
	}
	if len(ready) > 0 {
		if err := p.Deployer.WaitReady(ctx, i.Target, ready); err != nil {
			return out, err
		}
	}
	cli, close, err := k8s.OpenCLI(ctx, p.KubectlPath, p.Credentials, i.Target)
	if err != nil {
		return out, err
	}
	defer close()
	objects, err := k8s.InspectDeliveryObjects(ctx, cli, i.Target, refs)
	if err != nil {
		return out, err
	}
	for _, ref := range refs {
		obj, found := objects[ref.Key()]
		if !found {
			return out, nil
		}
		md, _ := obj["metadata"].(map[string]any)
		a, _ := md["annotations"].(map[string]any)
		labels, _ := md["labels"].(map[string]any)
		uid, _ := md["uid"].(string)
		if uid == "" || a["orchestrator.io/delivery-revision"] != i.Revision.ID || labels["app.kubernetes.io/managed-by"] != "orchestrator" || labels["orchestrator.io/application"] != i.Target.Extra["application"] || labels["orchestrator.io/environment"] != i.Target.Extra["environment"] {
			return out, domain.ErrStale
		}
		out.Objects = append(out.Objects, domain.ObjectObservation{APIVersion: ref.APIVersion, Kind: ref.Kind, Name: ref.Name, Namespace: i.Target.Namespace, UID: uid})
	}
	out.ObservedRevision = i.Revision.ID
	out.PayloadDigest = i.Revision.PayloadDigest
	out.Complete = true
	out.Sync = domain.InSync
	out.Health = domain.Healthy
	// Native direct delivery proves admission and readiness, not field-level drift.
	out.ObservedAt = time.Now().UTC()
	return out, nil
}
func (p *Provider) Withdraw(ctx context.Context, i port.Intent, r port.Receipt) (port.WithdrawalProof, error) {
	if p.Guard == nil {
		return port.WithdrawalProof{}, domain.ErrUnsupported
	}
	if err := p.Guard(ctx, i); err != nil {
		return port.WithdrawalProof{}, err
	}
	return port.WithdrawalProof{Receipt: r, ManagementStopped: true}, nil
}
func (p *Provider) Reconcile(context.Context, port.Intent, port.Receipt) (port.Receipt, error) {
	return port.Receipt{}, domain.ErrUnsupported
}

func (p *Provider) SameExecution(_ context.Context, raw []byte, proposed []execution.Manifest) (bool, error) {
	old, err := decode(raw)
	if err != nil {
		return false, err
	}
	prior := []map[string]any{}
	next := []map[string]any{}
	for _, o := range old.Objects {
		if o.Kind != "Job" {
			return false, domain.ErrInvalid
		}
		prior = append(prior, o.Body)
	}
	for _, m := range proposed {
		if m.Kind != "Job" {
			return false, domain.ErrInvalid
		}
		b, err := json.Marshal(m.Object)
		if err != nil {
			return false, err
		}
		var object map[string]any
		if json.Unmarshal(b, &object) != nil {
			return false, domain.ErrInvalid
		}
		next = append(next, object)
	}
	return artifact.SameExecution(prior, next)
}
func (p *Provider) RemoveRetainedObjects(ctx context.Context, i port.Intent, r port.Receipt, objects []port.AppliedObject) error {
	if _, err := p.Withdraw(ctx, i, r); err != nil {
		return err
	}
	return p.remove(ctx, i, objects)
}
func (p *Provider) PruneReleasedObjects(ctx context.Context, i port.Intent, r port.Receipt, objects []port.AppliedObject) error {
	observed, err := p.Observe(ctx, i, r)
	if err != nil {
		return err
	}
	if !observed.Ready(i.Revision, time.Now().UTC(), 30*time.Second) {
		return domain.ErrStale
	}
	keys := map[string]bool{}
	for _, o := range observed.Objects {
		keys[o.APIVersion+"/"+o.Kind+"/"+o.Name] = true
	}
	for _, o := range objects {
		if keys[o.Key()] {
			return domain.ErrInvalid
		}
	}
	return p.remove(ctx, i, objects)
}
func (p *Provider) remove(ctx context.Context, i port.Intent, objects []port.AppliedObject) error {
	cli, close, err := k8s.OpenCLI(ctx, p.KubectlPath, p.Credentials, i.Target)
	if err != nil {
		return err
	}
	defer close()
	for _, o := range objects {
		if err := p.Guard(ctx, i); err != nil {
			return err
		}
		if err := k8s.RemoveDirectDeliveryObject(ctx, cli, i.Target, o.ObjectRef, o.UID); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provider) ExportArtifact(_ context.Context, raw []byte) ([]execution.Manifest, error) {
	envelope, err := decode(raw)
	if err != nil {
		return nil, err
	}
	objects := []map[string]any{}
	for _, o := range envelope.Objects {
		objects = append(objects, o.Body)
	}
	return artifact.Export(objects)
}
