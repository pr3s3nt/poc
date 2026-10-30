package postgres

import (
	"context"

	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformstate"
)

var _ terraformstate.RepairArtifactRepository = (*StateStore)(nil)

func (s *StateStore) PutRepairArtifact(ctx context.Context, a persistence.CipherArtifact) error {
	if !validUUID(a.ID) || !validUUID(a.OrganizationID) || !validUUID(a.OwnerID) || a.Purpose != "TERRAFORM_REVIEW" || a.AADVersion != 1 || a.KeyID == "" || len(a.Nonce) != 12 || len(a.Ciphertext) < 16 {
		return persistence.ErrImmutable
	}
	return s.Transact(ctx, func(ctx context.Context) error {
		scope, err := s.requireOwner(ctx)
		if err != nil {
			return err
		}
		instance, err := s.instanceInScope(ctx, scope, a.OwnerID, false)
		if err != nil {
			return err
		}
		if a.OrganizationID != scope.organizationID || instance.Identity.OrganizationID != a.OrganizationID {
			return persistence.ErrNotFound
		}
		_, err = s.q(ctx).Exec(ctx, `INSERT INTO reconciliation_private_artifacts(id,organization_id,owner_id,purpose,key_id,nonce,ciphertext,aad_version,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, a.ID, a.OrganizationID, a.OwnerID, a.Purpose, a.KeyID, a.Nonce, a.Ciphertext, a.AADVersion, a.CreatedAt)
		return translate(err)
	})
}
func (s *StateStore) ReadRepairArtifact(ctx context.Context, org, id string) (a persistence.CipherArtifact, err error) {
	if !validUUID(org) || !validUUID(id) {
		return a, persistence.ErrNotFound
	}
	err = s.q(ctx).QueryRow(ctx, `SELECT id::text,organization_id::text,owner_id::text,purpose,key_id,nonce,ciphertext,aad_version,created_at FROM reconciliation_private_artifacts WHERE organization_id=$1 AND id=$2 AND purpose='TERRAFORM_REVIEW'`, org, id).Scan(&a.ID, &a.OrganizationID, &a.OwnerID, &a.Purpose, &a.KeyID, &a.Nonce, &a.Ciphertext, &a.AADVersion, &a.CreatedAt)
	return a, translate(err)
}
