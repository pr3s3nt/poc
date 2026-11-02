package postgres

import (
	"context"
	"errors"
	domain "orchestrator/internal/domain/delivery"
	"orchestrator/internal/ports/persistence"
)

func (s *Store) GetPlatformDeliveryDefault(ctx context.Context, org string) (out domain.PlatformDefault, err error) {
	if !validUUID(org) {
		return out, persistence.ErrNotFound
	}
	err = s.q(ctx).QueryRow(ctx, `SELECT organization_id::text,binding_id::text,binding_version,revision FROM platform_delivery_defaults WHERE organization_id=$1`, org).Scan(&out.OrganizationID, &out.BindingID, &out.BindingVersion, &out.Revision)
	return out, translate(err)
}
func (s *Store) SetPlatformDeliveryDefault(ctx context.Context, in domain.PlatformDefault, expected int64) (out domain.PlatformDefault, err error) {
	if !validUUID(in.OrganizationID) || !validUUID(in.BindingID) || in.BindingVersion < 1 || expected < 0 {
		return out, domain.ErrInvalid
	}
	err = s.Transact(ctx, func(ctx context.Context) error {
		b, err := scanBinding(s.q(ctx).QueryRow(ctx, bindingSelect+` WHERE organization_id=$1 AND id=$2 AND version=$3 FOR SHARE`, in.OrganizationID, in.BindingID, in.BindingVersion))
		if err != nil {
			return err
		}
		if b.Status != domain.BindingReady {
			return domain.ErrUnsupported
		}
		err = s.q(ctx).QueryRow(ctx, `INSERT INTO platform_delivery_defaults(organization_id,binding_id,binding_version,revision) SELECT $1::uuid,$2::uuid,$3,1 WHERE $4::bigint=0 OR EXISTS(SELECT 1 FROM platform_delivery_defaults WHERE organization_id=$1) ON CONFLICT(organization_id) DO UPDATE SET binding_id=EXCLUDED.binding_id,binding_version=EXCLUDED.binding_version,revision=platform_delivery_defaults.revision+1 WHERE platform_delivery_defaults.revision=$4 RETURNING organization_id::text,binding_id::text,binding_version,revision`, in.OrganizationID, in.BindingID, in.BindingVersion, expected).Scan(&out.OrganizationID, &out.BindingID, &out.BindingVersion, &out.Revision)
		if errors.Is(translate(err), persistence.ErrNotFound) {
			return persistence.ErrVersionConflict
		}
		return translate(err)
	})
	return
}

const appRepositorySelect = `SELECT id::text,organization_id::text,application_id::text,binding_id::text,binding_version,name,marker,status,remote_id,clone_url,branch,revision,created_at FROM application_delivery_repositories`

func scanApplicationRepository(row interface{ Scan(...any) error }) (out domain.ApplicationRepository, err error) {
	err = row.Scan(&out.ID, &out.OrganizationID, &out.ApplicationID, &out.BindingID, &out.BindingVersion, &out.Name, &out.Marker, &out.Status, &out.RemoteID, &out.CloneURL, &out.Branch, &out.Revision, &out.CreatedAt)
	if err != nil {
		return out, translate(err)
	}
	return out, out.Validate()
}
func (s *Store) GetApplicationDeliveryRepository(ctx context.Context, org, app string) (domain.ApplicationRepository, error) {
	if !validUUID(org) || !validUUID(app) {
		return domain.ApplicationRepository{}, persistence.ErrNotFound
	}
	return scanApplicationRepository(s.q(ctx).QueryRow(ctx, appRepositorySelect+` WHERE organization_id=$1 AND application_id=$2`, org, app))
}
func (s *Store) ReserveApplicationDeliveryRepository(ctx context.Context, in domain.ApplicationRepository) (out domain.ApplicationRepository, err error) {
	if in.Validate() != nil || in.Status != "PENDING" || in.Revision != 1 {
		return out, domain.ErrInvalid
	}
	err = s.Transact(ctx, func(ctx context.Context) error {
		if err := s.fenced(ctx, func(context.Context) error { return nil }); err != nil {
			return err
		}
		_, err := s.q(ctx).Exec(ctx, `INSERT INTO application_delivery_repositories(id,organization_id,application_id,binding_id,binding_version,name,marker,status,revision,created_at) SELECT $1,$2,$3,$4,$5,$6,$7,'PENDING',1,$8 WHERE EXISTS(SELECT 1 FROM applications WHERE id=$3 AND organization_id=$2 AND status='ACTIVE') ON CONFLICT(organization_id,application_id) DO NOTHING`, in.ID, in.OrganizationID, in.ApplicationID, in.BindingID, in.BindingVersion, in.Name, in.Marker, in.CreatedAt)
		if err != nil {
			return translate(err)
		}
		out, err = s.GetApplicationDeliveryRepository(ctx, in.OrganizationID, in.ApplicationID)
		if err != nil {
			return err
		}
		if out.BindingID != in.BindingID || out.BindingVersion != in.BindingVersion || out.Name != in.Name {
			return persistence.ErrVersionConflict
		}
		return nil
	})
	return
}
func (s *Store) AcknowledgeApplicationDeliveryRepository(ctx context.Context, in domain.ApplicationRepository, expected int64) (out domain.ApplicationRepository, err error) {
	if in.Validate() != nil || in.Status != "READY" {
		return out, domain.ErrInvalid
	}
	err = s.Transact(ctx, func(ctx context.Context) error {
		if err := s.fenced(ctx, func(context.Context) error { return nil }); err != nil {
			return err
		}
		prior, err := scanApplicationRepository(s.q(ctx).QueryRow(ctx, appRepositorySelect+` WHERE organization_id=$1 AND application_id=$2 FOR UPDATE`, in.OrganizationID, in.ApplicationID))
		if err != nil {
			return err
		}
		if !prior.SameAllocation(in) {
			return persistence.ErrImmutable
		}
		if prior.Status == "READY" {
			if prior.RemoteID != in.RemoteID || prior.CloneURL != in.CloneURL || prior.Branch != in.Branch {
				return persistence.ErrImmutable
			}
			out = prior
			return nil
		}
		if prior.Revision != expected {
			return persistence.ErrVersionConflict
		}
		out, err = scanApplicationRepository(s.q(ctx).QueryRow(ctx, `UPDATE application_delivery_repositories SET status='READY',remote_id=$3,clone_url=$4,branch=$5,revision=revision+1 WHERE organization_id=$1 AND application_id=$2 AND revision=$6 RETURNING id::text,organization_id::text,application_id::text,binding_id::text,binding_version,name,marker,status,remote_id,clone_url,branch,revision,created_at`, in.OrganizationID, in.ApplicationID, in.RemoteID, in.CloneURL, in.Branch, expected))
		return err
	})
	return
}
