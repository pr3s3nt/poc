package postgres

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"orchestrator/internal/ports/persistence"
)

func (s *Store) GetGitSettings(ctx context.Context, org string) (persistence.GitSettings, error) {
	var v persistence.GitSettings
	err := s.q(ctx).QueryRow(ctx, `SELECT o.organization_key,g.url,g.username,g.authenticated,g.ca_pem,g.credential_ref,g.version,g.updated_at,g.updated_by FROM platform_git_settings g JOIN organizations o ON o.id=g.organization_id WHERE o.organization_key=$1`, org).Scan(&v.OrganizationKey, &v.URL, &v.Username, &v.Authenticated, &v.CAPEM, &v.CredentialRef, &v.Version, &v.UpdatedAt, &v.UpdatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, persistence.ErrNotFound
	}
	if err != nil {
		return v, fmt.Errorf("postgres: read Git settings (%s)", sqlState(err))
	}
	return v, nil
}
func (s *Store) SaveGitSettings(ctx context.Context, expected int64, v persistence.GitSettings) error {
	if expected < 0 || v.Version != expected+1 {
		return persistence.ErrVersionConflict
	}
	tag, err := s.q(ctx).Exec(ctx, `INSERT INTO platform_git_settings(organization_id,url,username,authenticated,ca_pem,credential_ref,version,updated_at,updated_by)
 SELECT o.id,$2,$3,$4,$5,$6,$7,$8,$9 FROM organizations o WHERE o.organization_key=$1 AND ($10::bigint=0 OR EXISTS(SELECT 1 FROM platform_git_settings WHERE organization_id=o.id))
 ON CONFLICT(organization_id) DO UPDATE SET url=EXCLUDED.url,username=EXCLUDED.username,authenticated=EXCLUDED.authenticated,ca_pem=EXCLUDED.ca_pem,credential_ref=EXCLUDED.credential_ref,version=EXCLUDED.version,updated_at=EXCLUDED.updated_at,updated_by=EXCLUDED.updated_by WHERE platform_git_settings.version=$10`, v.OrganizationKey, v.URL, v.Username, v.Authenticated, v.CAPEM, v.CredentialRef, v.Version, v.UpdatedAt, v.UpdatedBy, expected)
	if err != nil {
		return fmt.Errorf("postgres: save Git settings (%s)", sqlState(err))
	}
	if tag.RowsAffected() != 1 {
		return persistence.ErrVersionConflict
	}
	return nil
}
