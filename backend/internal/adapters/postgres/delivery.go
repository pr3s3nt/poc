package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"time"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/domain/delivery"
	"orchestrator/internal/domain/environment"
	"orchestrator/internal/ports/persistence"
)

var _ persistence.DeliveryBindingRepository = (*Store)(nil)

const bindingSelect = `SELECT id::text,version,organization_id::text,name,provider_kind,contract_version,management_account_id::text,management_account_version,config,writer_credential_ref,capabilities,status,revision,verified_at,reason_code,created_at FROM delivery_bindings`

func scanBinding(row interface{ Scan(...any) error }) (delivery.BindingVersion, error) {
	var b delivery.BindingVersion
	var config, caps []byte
	err := row.Scan(&b.ID, &b.Version, &b.OrganizationID, &b.Name, &b.ProviderKind, &b.ContractVersion, &b.ManagementAccountID, &b.ManagementAccountVersion, &config, &b.WriterCredentialRef, &caps, &b.Status, &b.Revision, &b.VerifiedAt, &b.ReasonCode, &b.CreatedAt)
	if err != nil {
		return b, translate(err)
	}
	if json.Unmarshal(config, &b.Config) != nil || json.Unmarshal(caps, &b.Capabilities) != nil {
		return b, delivery.ErrInvalid
	}
	return b, b.Validate()
}
func (s *Store) CreateDeliveryBinding(ctx context.Context, b delivery.BindingVersion) error {
	if err := b.Validate(); err != nil {
		return err
	}
	if b.Status != delivery.Unverified || b.Revision != 1 || b.VerifiedAt != nil {
		return delivery.ErrInvalid
	}
	if !validUUID(b.ID) || !validUUID(b.OrganizationID) || !validUUID(b.ManagementAccountID) {
		return persistence.ErrNotFound
	}
	config, _ := json.Marshal(b.Config)
	caps, _ := json.Marshal(b.Capabilities)
	tag, err := s.q(ctx).Exec(ctx, `INSERT INTO delivery_bindings(id,version,organization_id,name,provider_kind,contract_version,management_account_id,management_account_version,config,writer_credential_ref,capabilities,status,revision,verified_at,reason_code,created_at)
 SELECT $1::uuid,$2::bigint,$3::uuid,$4,$5,$6,$7::uuid,$8::bigint,$9,$10,$11,$12,$13,$14,$15,$16 WHERE EXISTS(SELECT 1 FROM connections WHERE id=$7 AND organization_id=$3 AND version=$8)
 AND ($2::bigint=1 OR EXISTS(SELECT 1 FROM delivery_bindings WHERE id=$1 AND organization_id=$3 AND version=$2::bigint-1))`, b.ID, b.Version, b.OrganizationID, b.Name, b.ProviderKind, b.ContractVersion, b.ManagementAccountID, b.ManagementAccountVersion, config, b.WriterCredentialRef, caps, b.Status, b.Revision, b.VerifiedAt, b.ReasonCode, b.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return persistence.ErrDuplicate
		}
		return translate(err)
	}
	if tag.RowsAffected() != 1 {
		return persistence.ErrNotFound
	}
	return nil
}
func (s *Store) GetDeliveryBinding(ctx context.Context, org, id string, version int64) (delivery.BindingVersion, error) {
	if !validUUID(org) || !validUUID(id) {
		return delivery.BindingVersion{}, persistence.ErrNotFound
	}
	return scanBinding(s.q(ctx).QueryRow(ctx, bindingSelect+` WHERE organization_id=$1 AND id=$2 AND version=$3`, org, id, version))
}
func (s *Store) ListDeliveryBindings(ctx context.Context, org string) ([]delivery.BindingVersion, error) {
	if !validUUID(org) {
		return nil, persistence.ErrNotFound
	}
	rows, err := s.q(ctx).Query(ctx, bindingSelect+` WHERE organization_id=$1 ORDER BY id,version`, org)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()
	out := []delivery.BindingVersion{}
	for rows.Next() {
		b, err := scanBinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
func (s *Store) VerifyDeliveryBinding(ctx context.Context, org, id string, version, expected int64, status delivery.BindingStatus, caps delivery.Capabilities, at time.Time, reason string) (out delivery.BindingVersion, err error) {
	if status != delivery.BindingReady && status != delivery.Rejected && status != delivery.Disabled {
		return out, delivery.ErrInvalid
	}
	err = s.Transact(ctx, func(ctx context.Context) error {
		b, err := s.GetDeliveryBinding(ctx, org, id, version)
		if err != nil {
			return err
		}
		if b.Revision != expected {
			return persistence.ErrVersionConflict
		}
		b.Status = status
		b.Capabilities = caps
		b.VerifiedAt = &at
		b.ReasonCode = reason
		b.Revision++
		if err := b.Validate(); err != nil {
			return err
		}
		raw, _ := json.Marshal(caps)
		tag, err := s.q(ctx).Exec(ctx, `UPDATE delivery_bindings SET status=$5,capabilities=$6,verified_at=$7,reason_code=$8,revision=revision+1 WHERE organization_id=$1 AND id=$2 AND version=$3 AND revision=$4`, org, id, version, expected, status, raw, at, reason)
		if err != nil {
			return translate(err)
		}
		if tag.RowsAffected() != 1 {
			return persistence.ErrVersionConflict
		}
		out = b
		return nil
	})
	return
}

func (s *Store) SelectDelivery(ctx context.Context, cmd persistence.DeliverySelectionCommand) (out environment.Environment, err error) {
	if err := cmd.Selection.Validate(); err != nil {
		return out, err
	}
	if !validUUID(cmd.OrganizationID) {
		return out, persistence.ErrNotFound
	}
	err = s.Transact(ctx, func(ctx context.Context) error {
		env, err := scanEnv(s.q(ctx).QueryRow(ctx, envSelect+` WHERE a.application_key=$1 AND e.environment_key=$2 AND a.organization_id=$3 FOR UPDATE OF e`, cmd.ApplicationKey, cmd.EnvironmentKey, cmd.OrganizationID))
		if err != nil {
			return translate(err)
		}
		if env.Busy() {
			return persistence.ErrEnvironmentBusy
		}
		if env.Version != cmd.ExpectedEnvironmentVersion || env.DeliveryConfigRevision != cmd.ExpectedConfigurationRevision {
			return persistence.ErrVersionConflict
		}
		selected := cmd.Selection.Effective()
		if selected.Mode == delivery.CD {
			// Lock verification facts so a concurrent disable cannot race admission.
			b, err := scanBinding(s.q(ctx).QueryRow(ctx, bindingSelect+` WHERE organization_id=$1 AND id=$2 AND version=$3 FOR SHARE`, cmd.OrganizationID, selected.BindingID, selected.BindingVersion))
			if err != nil {
				return err
			}
			if b.Status != delivery.BindingReady {
				return persistence.ErrNotFound
			}
			if err := b.Capabilities.ValidateSelection(selected); err != nil {
				return err
			}
		}
		if env.DesiredDelivery.Effective() != selected {
			raw, _ := json.Marshal(selected)
			_, err = s.q(ctx).Exec(ctx, `UPDATE environments SET desired_delivery=$2,delivery_config_revision=delivery_config_revision+1,version=version+1 WHERE id=$1`, env.ID, raw)
			if err != nil {
				return translate(err)
			}
		}
		out, err = s.GetEnvironment(ctx, cmd.ApplicationKey, cmd.EnvironmentKey)
		return err
	})
	return
}

func (s *Store) GetOrganizationByID(ctx context.Context, id string) (application.Organization, error) {
	var org application.Organization
	if !validUUID(id) {
		return org, persistence.ErrNotFound
	}
	err := s.q(ctx).QueryRow(ctx, `SELECT id::text,organization_key,name FROM organizations WHERE id=$1`, id).Scan(&org.ID, &org.Key, &org.Name)
	return org, translate(err)
}

func (s *Store) ActivateDelivery(ctx context.Context, cmd persistence.DeliveryActivation) error {
	if cmd.CheckEpoch && (cmd.ExpectedEpoch < 0 || cmd.NextEpoch != cmd.ExpectedEpoch+1) {
		return delivery.ErrInvalid
	}
	if _, ok := persistence.OwnerFrom(ctx); !ok {
		return persistence.ErrOperationLost
	}
	return s.fenced(ctx, func(ctx context.Context) error {
		raw, _ := json.Marshal(cmd.Selection.Effective())
		tag, err := s.q(ctx).Exec(ctx, `UPDATE environments e SET applied_delivery=$3,delivery_epoch=CASE WHEN $6::boolean THEN $8::bigint ELSE e.delivery_epoch END FROM applications a WHERE e.application_id=a.id AND a.application_key=$1 AND e.environment_key=$2 AND e.desired_delivery=$3::jsonb AND e.delivery_config_revision=$4 AND e.active_operation_id=$5 AND (NOT $6::boolean OR e.delivery_epoch=$7::bigint)`, cmd.ApplicationKey, cmd.EnvironmentKey, raw, cmd.ConfigurationRevision, ownerArg(ctx), cmd.CheckEpoch, cmd.ExpectedEpoch, cmd.NextEpoch)
		if err != nil {
			return translate(err)
		}
		if tag.RowsAffected() != 1 {
			return persistence.ErrVersionConflict
		}
		if cmd.CheckEpoch {
			_, err = s.q(ctx).Exec(ctx, `UPDATE workload_instances w SET target_ref=jsonb_set(w.target_ref,'{deliveryMode}',to_jsonb($3::text),true) FROM environments e JOIN applications a ON a.id=e.application_id WHERE w.environment_id=e.id AND a.application_key=$1 AND e.environment_key=$2 AND w.generation=e.target_generation AND w.status<>'REMOVED'`, cmd.ApplicationKey, cmd.EnvironmentKey, string(cmd.Selection.Effective().Mode))
			if err != nil {
				return translate(err)
			}
		}
		return nil
	})
}
