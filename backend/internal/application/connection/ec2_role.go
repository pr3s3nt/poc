package connection

import (
	"context"
	"errors"
	"regexp"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/ports/terraformexec"
)

// EC2RoleProvider uses only IMDSv2, never the ambient AWS credential chain.
type EC2RoleProvider interface {
	ResolveEC2Role(context.Context, string, string) (terraformexec.AWSConnection, error)
	VerifyEC2Role(context.Context, string, []string) (AWSVerification, error)
}

var roleARNPattern = regexp.MustCompile(`^arn:aws:iam::([0-9]{12}):role/(?:[A-Za-z0-9+=,.@_-]+/)*[A-Za-z0-9+=,.@_-]{1,64}$`)

func ValidEC2RoleARN(arn string) bool                              { return len(arn) <= 2048 && roleARNPattern.MatchString(arn) }
func (s *Service) SetEC2RoleProvider(p EC2RoleProvider)            { s.ec2 = p }
func (r *CredentialResolver) SetEC2RoleProvider(p EC2RoleProvider) { r.ec2 = p }

type RegisterEC2RoleCommand struct {
	Name                string
	RoleARN             string
	VerificationRegions []string
}

// completeRoleVerification shares capability/region checks without accepting an
// IAM user or an arbitrary STS session as the expected role identity.
func completeRoleVerification(v AWSVerification, arn string, regions []string) bool {
	m := roleARNPattern.FindStringSubmatch(arn)
	if m == nil || v.AccountID != m[1] || v.PrincipalARN != arn || v.RoleID == "" {
		return false
	}
	return completeAWSCapabilities(v, regions)
}

func (s *Service) RegisterEC2Role(ctx context.Context, org string, cmd RegisterEC2RoleCommand) (application.Connection, error) {
	name, err := validName(cmd.Name)
	if err != nil {
		return application.Connection{}, &FieldError{Field: "name"}
	}
	if !ValidEC2RoleARN(cmd.RoleARN) {
		return application.Connection{}, &FieldError{Field: "roleArn"}
	}
	regions, err := validAWSRegions(cmd.VerificationRegions, "verificationRegions")
	if err != nil {
		return application.Connection{}, err
	}
	if _, err = s.store.GetOrganization(ctx, org); err != nil {
		return application.Connection{}, err
	}
	if s.ec2 == nil {
		return application.Connection{}, ErrVerification
	}
	v, err := s.ec2.VerifyEC2Role(ctx, cmd.RoleARN, regions)
	if err != nil {
		return application.Connection{}, ec2VerificationError(err)
	}
	if !completeRoleVerification(v, cmd.RoleARN, regions) {
		return application.Connection{}, ErrAccountIncompatible
	}
	for attempt := 0; attempt < maxRegisterRetries; attempt++ {
		key, err := s.freeKey(ctx, org, keySlug(name))
		if err != nil {
			return application.Connection{}, err
		}
		conn := application.Connection{ID: ids.New(), Key: key, Name: name, OrganizationKey: org, Kind: application.ConnectionAWS, AuthenticationType: application.AuthAWSEC2Role, Status: application.ConnectionReady, Version: 1,
			Config:       map[string]any{"accountId": v.AccountID, "roleArn": cmd.RoleARN, "roleId": v.RoleID, "principalArn": v.PrincipalARN, "region": regions[0], "allowedRegions": regions, "verificationRegions": regions},
			Verification: awsVerificationRecord(v, regions, s.timestamp()),
		}
		err = s.store.Transact(ctx, func(ctx context.Context) error { return s.store.CreateConnection(ctx, conn) })
		if errors.Is(err, persistence.ErrCommitUncertain) {
			return application.Connection{}, ErrOutcomeUncertain
		}
		if err == nil {
			return conn, nil
		}
		if !errors.Is(err, persistence.ErrDuplicate) {
			return application.Connection{}, err
		}
	}
	return application.Connection{}, ErrDuplicate
}

func ec2VerificationError(err error) error {
	if errors.Is(err, ErrAccountIncompatible) {
		return ErrAccountIncompatible
	}
	// Keep only categories, never metadata bodies, tokens or signed request errors.
	for _, category := range []error{ErrPermissionDenied, ErrCapabilityUnverifiable, ErrRegionUnavailable, ErrClusterUnreachable, ErrAuthentication} {
		if errors.Is(err, category) {
			return categorize(category, ErrVerification)
		}
	}
	return ErrVerification
}

func (s *Service) verifyEC2Role(ctx context.Context, cur application.Connection, requested []string) (application.Connection, error) {
	if s.ec2 == nil || cur.SecretRef != "" || cur.ConfigString("roleId") == "" {
		return application.Connection{}, ErrAccountIncompatible
	}
	regions := storedAWSRegions(cur)
	if requested != nil {
		var err error
		regions, err = validAWSRegions(requested, "verificationRegions")
		if err != nil {
			return application.Connection{}, err
		}
	}
	if len(regions) == 0 {
		return application.Connection{}, ErrAccountIncompatible
	}
	v, err := s.ec2.VerifyEC2Role(ctx, cur.ConfigString("roleArn"), regions)
	if err != nil {
		return application.Connection{}, ec2VerificationError(err)
	}
	if !completeRoleVerification(v, cur.ConfigString("roleArn"), regions) || v.AccountID != cur.ConfigString("accountId") || v.RoleID != cur.ConfigString("roleId") {
		return application.Connection{}, ErrAccountIncompatible
	}
	return s.publish(ctx, cur.OrganizationKey, cur.Key, cur.Version, func(c application.Connection) application.Connection {
		next := c
		next.Config = cloneConfig(c.Config)
		next.Config["verificationRegions"] = regions
		next.Status = application.ConnectionReady
		next.Verification = awsVerificationRecord(v, regions, s.timestamp())
		return next
	})
}

func (r *CredentialResolver) resolveEC2Account(ctx context.Context, conn application.Connection, org, key string) (terraformexec.AWSConnection, error) {
	if r.ec2 == nil || conn.OrganizationKey != org || conn.Key != key || conn.Kind != application.ConnectionAWS || conn.Status != application.ConnectionReady || conn.SecretRef != "" || conn.ConfigString("roleId") == "" || conn.ConfigString("accountId") == "" || !ValidEC2RoleARN(conn.ConfigString("roleArn")) {
		return terraformexec.AWSConnection{}, ErrCredentialUnavailable
	}
	arn, roleID := conn.ConfigString("roleArn"), conn.ConfigString("roleId")
	fresh, err := r.ec2.ResolveEC2Role(ctx, arn, conn.ConfigString("region"))
	if err != nil || fresh.AccountID != conn.ConfigString("accountId") || fresh.RoleARN != arn || fresh.RoleID != roleID {
		return terraformexec.AWSConnection{}, ErrCredentialUnavailable
	}
	fresh.ConnectionKey = key
	fresh.Refresh = func(ctx context.Context) (terraformexec.AWSConnection, error) {
		next, err := r.ResolveAWS(ctx, org, key)
		if err != nil || next.RoleARN != arn || next.RoleID != roleID || next.AccountID != fresh.AccountID {
			return terraformexec.AWSConnection{}, ErrCredentialUnavailable
		}
		return next, nil
	}
	return fresh, nil
}
