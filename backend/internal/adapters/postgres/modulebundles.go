package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"orchestrator/internal/domain/tfmodule"
	"orchestrator/internal/ports/persistence"
)

// Verified Terraform module registry (terraform_module_bundles). Rows are
// insert-only; the baseline trigger refuses UPDATE and DELETE and the pin
// constraint UNIQUE(source_id,module_id,revision,subdirectory) admits one
// digest per revision path, so concurrent registrations of different content
// for one revision cannot both commit.

const bundleColumns = `id::text,source_id,module_id,revision,subdirectory,content_sha256,terraform_version,terraform_binary_sha256,provider_lock_digest,contract,provider_requirements,status,created_at`

// InsertModuleBundle implements persistence.ModuleBundleRepository. Inside a
// transaction a conflicting concurrent insert blocks until the other
// transaction ends; the read that follows returns the committed bundle.
func (s *Store) InsertModuleBundle(ctx context.Context, b tfmodule.Bundle) (tfmodule.Bundle, error) {
	if err := b.Validate(); err != nil {
		return tfmodule.Bundle{}, err
	}
	contract, err := json.Marshal(b.Contract)
	if err != nil {
		return tfmodule.Bundle{}, err
	}
	providers, err := json.Marshal(b.Providers)
	if err != nil {
		return tfmodule.Bundle{}, err
	}
	created := b.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	if _, err := s.q(ctx).Exec(ctx, `INSERT INTO terraform_module_bundles(id,source_id,module_id,revision,subdirectory,content_sha256,terraform_version,terraform_binary_sha256,provider_lock_digest,contract,provider_requirements,status,created_at,updated_at)
		VALUES($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$13) ON CONFLICT DO NOTHING`,
		b.ID, b.Ref.SourceID, b.Ref.ModuleID, b.Ref.Revision, b.Ref.Subdirectory, b.Ref.ContentSHA256,
		b.TerraformVersion, b.TerraformBinarySHA256, b.ProviderLockDigest, contract, providers, b.Status, created); err != nil {
		return tfmodule.Bundle{}, fmt.Errorf("postgres: insert module bundle (%s)", sqlState(err))
	}
	stored, err := s.GetModuleBundle(ctx, b.Ref)
	if errors.Is(err, persistence.ErrNotFound) {
		// Only possible when another bundle claims the same ID or identity
		// under a different pin; never treat it as stored.
		return tfmodule.Bundle{}, fmt.Errorf("postgres: module bundle identity conflict")
	}
	return stored, err
}

// GetModuleBundle implements persistence.ModuleBundleRepository.
func (s *Store) GetModuleBundle(ctx context.Context, ref tfmodule.SourceRef) (tfmodule.Bundle, error) {
	row := s.q(ctx).QueryRow(ctx, `SELECT `+bundleColumns+` FROM terraform_module_bundles WHERE source_id=$1 AND module_id=$2 AND revision=$3 AND subdirectory=$4`,
		ref.SourceID, ref.ModuleID, ref.Revision, ref.Subdirectory)
	b, err := scanBundle(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return tfmodule.Bundle{}, persistence.ErrNotFound
	}
	return b, err
}

// ListModuleBundles implements persistence.ModuleBundleRepository.
func (s *Store) ListModuleBundles(ctx context.Context) ([]tfmodule.Bundle, error) {
	rows, err := s.q(ctx).Query(ctx, `SELECT `+bundleColumns+` FROM terraform_module_bundles ORDER BY source_id,module_id,revision,subdirectory`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list module bundles (%s)", sqlState(err))
	}
	defer rows.Close()
	out := []tfmodule.Bundle{}
	for rows.Next() {
		b, err := scanBundle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func scanBundle(row pgx.Row) (tfmodule.Bundle, error) {
	var b tfmodule.Bundle
	var contract, providers []byte
	if err := row.Scan(&b.ID, &b.Ref.SourceID, &b.Ref.ModuleID, &b.Ref.Revision, &b.Ref.Subdirectory, &b.Ref.ContentSHA256,
		&b.TerraformVersion, &b.TerraformBinarySHA256, &b.ProviderLockDigest, &contract, &providers, &b.Status, &b.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tfmodule.Bundle{}, err
		}
		return tfmodule.Bundle{}, fmt.Errorf("postgres: read module bundle (%s)", sqlState(err))
	}
	if err := json.Unmarshal(contract, &b.Contract); err != nil {
		return tfmodule.Bundle{}, errors.New("postgres: module bundle contract is malformed")
	}
	if err := json.Unmarshal(providers, &b.Providers); err != nil {
		return tfmodule.Bundle{}, errors.New("postgres: module bundle provider requirements are malformed")
	}
	b.CreatedAt = b.CreatedAt.UTC()
	if err := b.Validate(); err != nil {
		return tfmodule.Bundle{}, errors.New("postgres: stored module bundle is not valid")
	}
	return b, nil
}
