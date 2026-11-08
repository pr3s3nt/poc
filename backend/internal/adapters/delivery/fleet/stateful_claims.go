package fleet

import (
	"context"
	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
)

// Only generated provenance is stable across revisions of an immutable claim
// template. Storage intent and business metadata still pass unchanged to native
// admission; no live resource is adopted or edited by preparation.
func preserveClaimProvenance(i port.Intent, manifests []execution.Manifest) ([]execution.Manifest, error) {
	if i.Revision.Phase != domain.Rollout || i.PriorSubmission == nil {
		return manifests, nil
	}
	prior := i.PriorSubmission
	if i.ExpectedRevision == "" || prior.Prepared.Revision.ID != i.ExpectedRevision || prior.Receipt.RevisionID != i.ExpectedRevision || prior.Receipt.PayloadDigest != prior.Prepared.Revision.PayloadDigest {
		return nil, ErrOwnership
	}
	old, err := (&Provider{}).ExportArtifact(context.Background(), prior.Prepared.Payload)
	if err != nil {
		return nil, err
	}
	source := map[string]execution.Manifest{}
	for _, m := range old {
		source[m.APIVersion+"/"+m.Kind+"/"+m.Name] = m
	}
	uids := map[string]string{}
	for _, o := range i.SourceInventory {
		uids[o.Key()] = o.UID
	}
	out := append([]execution.Manifest(nil), manifests...)
	for index, m := range out {
		if m.Kind != "StatefulSet" {
			continue
		}
		previous, found := source[m.APIVersion+"/"+m.Kind+"/"+m.Name]
		if !found {
			continue
		}
		if m.Namespace != i.Target.Namespace || previous.Namespace != i.Target.Namespace || uids[(execution.ObjectRef{APIVersion: m.APIVersion, Kind: m.Kind, Name: m.Name}).Key()] == "" {
			return nil, ErrOwnership
		}
		original := map[string]map[string]any{}
		claims, _ := obj(previous.Object["spec"])["volumeClaimTemplates"].([]any)
		for _, raw := range claims {
			c := obj(raw)
			original[stringValue(obj(c["metadata"])["name"])] = c
		}
		out[index].Object = cloneObject(m.Object)
		proposed, _ := obj(out[index].Object["spec"])["volumeClaimTemplates"].([]any)
		for _, raw := range proposed {
			claim := obj(raw)
			md := obj(claim["metadata"])
			oldClaim, ok := original[stringValue(md["name"])]
			if !ok {
				continue
			}
			labels := obj(md["labels"])
			originalID, present := obj(obj(oldClaim["metadata"])["labels"])["orchestrator.io/deployment-id"]
			if present {
				if labels == nil {
					labels = map[string]any{}
					md["labels"] = labels
				}
				labels["orchestrator.io/deployment-id"] = originalID
			} else {
				delete(labels, "orchestrator.io/deployment-id")
			}
		}
	}
	return out, nil
}
