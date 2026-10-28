package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"orchestrator/internal/domain/renderbundle"
	"orchestrator/internal/ports/persistence"
)

// Verified Git render bundle registry (render_bundles, ADR-021). Rows are
// Organization scoped and insert-only; the baseline trigger refuses UPDATE and
// DELETE and the pin constraint admits one record per Organization pin.

const renderBundleColumns = `b.id::text,o.organization_key,b.source_kind,b.source_url,b.source_path,b.source_revision,b.content_sha256,b.engine_contract,b.renderer_version,b.renderer_binary_sha256,b.contract,b.created_at`

// InsertRenderBundle implements persistence.RenderBundleRepository.
func (s *Store) InsertRenderBundle(ctx context.Context, b renderbundle.Bundle) (renderbundle.Bundle, error) {
	if err := b.Validate(); err != nil {
		return renderbundle.Bundle{}, err
	}
	contract, err := json.Marshal(b.Contract)
	if err != nil {
		return renderbundle.Bundle{}, err
	}
	created := b.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	var stored renderbundle.Bundle
	err = s.Transact(ctx, func(ctx context.Context) error {
		tag, err := s.q(ctx).Exec(ctx, `INSERT INTO render_bundles(id,organization_id,source_url,source_path,source_revision,content_sha256,engine_contract,renderer_version,renderer_binary_sha256,contract,created_at,source_kind)
SELECT $1::uuid,o.id,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12 FROM organizations o WHERE o.organization_key=$2
ON CONFLICT (organization_id,source_url,source_path,source_revision,engine_contract) DO NOTHING`,
			b.ID, b.OrganizationKey, b.Source.URL, b.Source.Path, b.Source.Revision, b.Source.ContentSHA256,
			b.EngineContract, b.RendererVersion, b.RendererBinary, contract, created, b.Source.Kind)
		if err != nil {
			return fmt.Errorf("postgres: insert render bundle (%s)", sqlState(err))
		}
		row := s.q(ctx).QueryRow(ctx, `SELECT `+renderBundleColumns+` FROM render_bundles b JOIN organizations o ON o.id=b.organization_id
WHERE o.organization_key=$1 AND b.source_url=$2 AND b.source_path=$3 AND b.source_revision=$4 AND b.engine_contract=$5`,
			b.OrganizationKey, b.Source.URL, b.Source.Path, b.Source.Revision, b.EngineContract)
		stored, err = scanRenderBundle(row)
		if errors.Is(err, pgx.ErrNoRows) {
			if tag.RowsAffected() == 0 {
				// No Organization, or another record already claims the ID.
				return fmt.Errorf("%w: organization", persistence.ErrNotFound)
			}
			return errors.New("postgres: render bundle insert was not readable")
		}
		return err
	})
	if err != nil {
		return renderbundle.Bundle{}, err
	}
	return stored, nil
}

// GetRenderBundle implements persistence.RenderBundleRepository.
func (s *Store) GetRenderBundle(ctx context.Context, organizationKey, id string) (renderbundle.Bundle, error) {
	if !validUUID(id) {
		return renderbundle.Bundle{}, persistence.ErrNotFound
	}
	b, err := scanRenderBundle(s.q(ctx).QueryRow(ctx, `SELECT `+renderBundleColumns+` FROM render_bundles b JOIN organizations o ON o.id=b.organization_id
WHERE o.organization_key=$1 AND b.id=$2::uuid`, organizationKey, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return renderbundle.Bundle{}, persistence.ErrNotFound
	}
	return b, err
}

// ListRenderBundles implements persistence.RenderBundleRepository.
func (s *Store) ListRenderBundles(ctx context.Context, organizationKey string) ([]renderbundle.Bundle, error) {
	rows, err := s.q(ctx).Query(ctx, `SELECT `+renderBundleColumns+` FROM render_bundles b JOIN organizations o ON o.id=b.organization_id
WHERE o.organization_key=$1 ORDER BY b.created_at,b.id`, organizationKey)
	if err != nil {
		return nil, fmt.Errorf("postgres: list render bundles (%s)", sqlState(err))
	}
	defer rows.Close()
	out := []renderbundle.Bundle{}
	for rows.Next() {
		b, err := scanRenderBundle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func scanRenderBundle(row pgx.Row) (renderbundle.Bundle, error) {
	var b renderbundle.Bundle
	var contract []byte
	if err := row.Scan(&b.ID, &b.OrganizationKey, &b.Source.Kind, &b.Source.URL, &b.Source.Path, &b.Source.Revision, &b.Source.ContentSHA256,
		&b.EngineContract, &b.RendererVersion, &b.RendererBinary, &contract, &b.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return renderbundle.Bundle{}, err
		}
		return renderbundle.Bundle{}, fmt.Errorf("postgres: read render bundle (%s)", sqlState(err))
	}
	if err := json.Unmarshal(contract, &b.Contract); err != nil {
		return renderbundle.Bundle{}, errors.New("postgres: render bundle contract is malformed")
	}
	b.CreatedAt = b.CreatedAt.UTC()
	if err := b.Validate(); err != nil {
		return renderbundle.Bundle{}, errors.New("postgres: stored render bundle is not valid")
	}
	return b, nil
}

var _ persistence.RenderBundleRepository = (*Store)(nil)
