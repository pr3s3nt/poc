package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformstate"
)

var ErrArtifact = errors.New("delivery: private artifact unavailable")

type Artifacts struct {
	Store  persistence.DeliveryArtifactRepository
	Cipher terraformstate.Cipher
}

func artifactAAD(a persistence.CipherArtifact) []byte {
	raw, _ := json.Marshal([]any{"reconciliation", a.OrganizationID, a.Purpose, a.OwnerID, a.ID, a.AADVersion})
	return raw
}
func (a *Artifacts) Put(ctx context.Context, org, owner string, payload []byte) (string, error) {
	if a == nil || a.Store == nil || a.Cipher == nil || org == "" || owner == "" || len(payload) == 0 {
		return "", ErrArtifact
	}
	blob := persistence.CipherArtifact{ID: ids.New(), OrganizationID: org, OwnerID: owner, Purpose: "DELIVERY_PAYLOAD", KeyID: a.Cipher.KeyID(), AADVersion: 1, CreatedAt: time.Now().UTC()}
	nonce, ciphertext, err := a.Cipher.Seal(payload, artifactAAD(blob))
	if err != nil {
		return "", ErrArtifact
	}
	blob.Nonce, blob.Ciphertext = nonce, ciphertext
	if err := a.Store.PutDeliveryArtifact(ctx, blob); err != nil {
		return "", err
	}
	return blob.ID, nil
}
func (a *Artifacts) Get(ctx context.Context, org, owner, id string) ([]byte, error) {
	if a == nil || a.Store == nil || a.Cipher == nil {
		return nil, ErrArtifact
	}
	blob, err := a.Store.GetDeliveryArtifact(ctx, org, id)
	if err != nil {
		return nil, ErrArtifact
	}
	if blob.OwnerID != owner || blob.Purpose != "DELIVERY_PAYLOAD" || blob.AADVersion != 1 {
		return nil, ErrArtifact
	}
	plain, err := a.Cipher.Open(blob.KeyID, blob.Nonce, blob.Ciphertext, artifactAAD(blob))
	if err != nil {
		return nil, ErrArtifact
	}
	return plain, nil
}
