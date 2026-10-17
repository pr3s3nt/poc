package connection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"

	"orchestrator/internal/domain/application"
)

// UC-04 VAR-02: AWS access-key onboarding. The registration command carries
// the key pair only until it is verified and written to the scoped credential
// store; the Connection record keeps account/principal/region metadata and the
// opaque store reference.

// MaxAllowedRegions bounds the allowed-region list of one registration.
const MaxAllowedRegions = 10

var (
	awsAccessKeyPattern = regexp.MustCompile(`^AKIA[A-Z0-9]{16}$`)
	awsSecretKeyPattern = regexp.MustCompile(`^[A-Za-z0-9/+]{40}$`)
	awsRegionPattern    = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-[0-9]$`)
	awsAccountPattern   = regexp.MustCompile(`^[0-9]{12}$`)
	awsUserARNPattern   = regexp.MustCompile(`^arn:aws:iam::([0-9]{12}):user/[A-Za-z0-9+=,.@_/-]{1,512}$`)
)

// RequiredAWSCapabilities are the provisioning capability groups a READY AWS
// Connection must have proven, sorted. The verifier must report exactly these.
var RequiredAWSCapabilities = []string{"Aurora", "EKS", "ElastiCache", "VPC"}

// completeAWSVerification reports whether the verifier proved everything for
// exactly the requested regions: a 12-digit account, an IAM user principal of
// that same account, the requested region set in the requested order and each
// required capability group exactly once. Anything else is not READY.
func completeAWSVerification(v AWSVerification, requested []string) bool {
	if !awsAccountPattern.MatchString(v.AccountID) {
		return false
	}
	match := awsUserARNPattern.FindStringSubmatch(v.PrincipalARN)
	if match == nil || match[1] != v.AccountID {
		return false
	}
	if len(v.Regions) != len(requested) {
		return false
	}
	for i := range requested {
		if v.Regions[i] != requested[i] {
			return false
		}
	}
	if len(v.Capabilities) != len(RequiredAWSCapabilities) {
		return false
	}
	got := append([]string(nil), v.Capabilities...)
	sort.Strings(got)
	for i := range got {
		if got[i] != RequiredAWSCapabilities[i] {
			return false
		}
	}
	return true
}

// AWS verification failure categories a verifier may wrap in ErrVerification.
// Only fixed guidance reaches the caller; AWS responses never do.
var (
	// ErrRegionUnavailable: the credential works but an allowed region is
	// disabled for the account or a required service cannot be reached there.
	ErrRegionUnavailable = errors.New("an allowed region is not usable with this credential")
	// ErrCapabilityUnverifiable: provisioning capabilities could not be
	// proven read-only (for example IAM policy simulation is not allowed).
	ErrCapabilityUnverifiable = errors.New("the provisioning capabilities could not be verified")
	// ErrUnsupportedPrincipal: the credential belongs to a principal whose
	// capabilities cannot be evaluated (root account or temporary credential).
	ErrUnsupportedPrincipal = errors.New("the credential principal is not supported")
)

// AWSAccessKey is the submitted key pair. It formats as redacted so a stray
// %v, %+v or %#v cannot leak it into a log or error.
type AWSAccessKey struct {
	AccessKeyID     string
	SecretAccessKey string
}

func (AWSAccessKey) String() string               { return "AWSAccessKey{redacted}" }
func (AWSAccessKey) GoString() string             { return "AWSAccessKey{redacted}" }
func (k AWSAccessKey) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte(k.String())) }

// AWSVerification is the safe result of read-only verification. Every field
// is public metadata; none is credential material.
type AWSVerification struct {
	AccountID    string
	PrincipalARN string
	// Regions are the regions whose reachability was proven.
	Regions []string
	// Capabilities are the provisioning groups proven allowed (for example
	// "VPC", "EKS", "ElastiCache", "Aurora").
	Capabilities []string
}

// AWSVerifier proves identity, regions and provisioning capabilities of an
// access key with read-only calls and without host credentials. STS success
// alone must never satisfy it: a result is returned only when the account is
// known, every region is usable and every capability group is allowed.
type AWSVerifier interface {
	VerifyAWS(ctx context.Context, key AWSAccessKey, regions []string) (AWSVerification, error)
}

// RegisterAWSCommand is the AWS body {name,accessKeyId,secretAccessKey,allowedRegions}.
type RegisterAWSCommand struct {
	Name            string
	AccessKeyID     string
	SecretAccessKey string
	AllowedRegions  []string
}

func (RegisterAWSCommand) String() string               { return "RegisterAWSCommand{redacted}" }
func (RegisterAWSCommand) GoString() string             { return "RegisterAWSCommand{redacted}" }
func (c RegisterAWSCommand) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte(c.String())) }

// FieldError is ErrInvalid with the request field to correct. It never holds
// the rejected value.
type FieldError struct{ Field string }

func (e *FieldError) Error() string { return "connection: the " + e.Field + " field is not valid" }
func (e *FieldError) Unwrap() error { return ErrInvalid }

// SetAWSRegistration wires the AWS verifier. Without it (or without a
// credential store) AWS registration fails closed.
func (s *Service) SetAWSRegistration(verifier AWSVerifier) { s.aws = verifier }

func validAWSRegions(raw []string, field string) ([]string, error) {
	if len(raw) == 0 || len(raw) > MaxAllowedRegions {
		return nil, &FieldError{Field: field}
	}
	seen := map[string]bool{}
	for _, region := range raw {
		if !awsRegionPattern.MatchString(region) || len(region) > 30 || seen[region] {
			return nil, &FieldError{Field: field}
		}
		seen[region] = true
	}
	return append([]string(nil), raw...), nil
}

// RegisterAWS verifies the access key read-only, writes it to the scoped
// credential store under an attempt-owned object and inserts a READY
// Connection. A failed verification or store write saves nothing; a failed
// insert (including a concurrent winner of the same key) deletes only this
// attempt's credential object.
func (s *Service) RegisterAWS(ctx context.Context, org string, cmd RegisterAWSCommand) (application.Connection, error) {
	return s.registerAWS(ctx, org, cmd, "allowedRegions")
}

// RegisterAWSDriverAccount is the V01 account creation. The regions are
// verification evidence (field verificationRegions); the backing record keeps
// the legacy allowed-region metadata for the staged legacy consumers only.
func (s *Service) RegisterAWSDriverAccount(ctx context.Context, org string, cmd RegisterAWSCommand) (application.Connection, error) {
	return s.registerAWS(ctx, org, cmd, "verificationRegions")
}

func (s *Service) registerAWS(ctx context.Context, org string, cmd RegisterAWSCommand, regionField string) (application.Connection, error) {
	name, err := validName(cmd.Name)
	if err != nil {
		return application.Connection{}, &FieldError{Field: "name"}
	}
	if !awsAccessKeyPattern.MatchString(cmd.AccessKeyID) {
		return application.Connection{}, &FieldError{Field: "accessKeyId"}
	}
	if !awsSecretKeyPattern.MatchString(cmd.SecretAccessKey) {
		return application.Connection{}, &FieldError{Field: "secretAccessKey"}
	}
	regions, err := validAWSRegions(cmd.AllowedRegions, regionField)
	if err != nil {
		return application.Connection{}, err
	}
	if s.credentials == nil {
		return application.Connection{}, ErrCredentialStore
	}
	if s.aws == nil {
		return application.Connection{}, fmt.Errorf("%w: AWS verifier is unavailable", ErrVerification)
	}
	if _, err := s.store.GetOrganization(ctx, org); err != nil {
		return application.Connection{}, err
	}
	key := AWSAccessKey{AccessKeyID: cmd.AccessKeyID, SecretAccessKey: cmd.SecretAccessKey}
	verified, err := s.aws.VerifyAWS(ctx, key, regions)
	if err != nil {
		return application.Connection{}, awsVerificationError(err)
	}
	if !completeAWSVerification(verified, regions) {
		// A verifier that did not prove everything is a failed verification,
		// never a READY Connection.
		return application.Connection{}, fmt.Errorf("%w: the verification result is incomplete", ErrVerification)
	}
	value, err := json.Marshal(awsAccessKeyValue{AccessKeyID: cmd.AccessKeyID, SecretAccessKey: cmd.SecretAccessKey})
	if err != nil {
		return application.Connection{}, ErrCredentialStore
	}
	return s.persist(ctx, org, name, value, func(key, ref string) application.Connection {
		return application.Connection{
			Key: key, Name: name, OrganizationKey: org,
			Kind: application.ConnectionAWS, AuthenticationType: application.AuthAWSAccessKey,
			// region is the default (first allowed) region older consumers
			// read; allowedRegions is the full verified list.
			Config: map[string]any{
				"accountId": verified.AccountID, "region": regions[0], "allowedRegions": regions,
				"principalArn": verified.PrincipalARN, "verificationRegions": regions,
			},
			SecretRef: ref, Status: application.ConnectionReady, Version: 1,
			Verification: map[string]any{
				"verified": true, "accountId": verified.AccountID, "principalArn": verified.PrincipalARN,
				"region": regions[0], "regions": verified.Regions,
				"identity": "VERIFIED", "regionCheck": "VERIFIED", "capabilityCheck": "VERIFIED",
				"capabilities": verified.Capabilities, "verifiedAt": s.timestamp(),
			},
		}
	})
}

// awsVerificationError maps a verifier failure to fixed guidance (ERR-05/06).
func awsVerificationError(err error) error {
	switch {
	case errors.Is(err, ErrAuthentication):
		return categorize(ErrAuthentication, fmt.Errorf("%w: AWS rejected the access key; check the access key ID and secret access key", ErrVerification))
	case errors.Is(err, ErrPermissionDenied):
		return categorize(ErrPermissionDenied, fmt.Errorf("%w: the access key lacks a permission required to provision VPC, EKS, ElastiCache or Aurora", ErrVerification))
	case errors.Is(err, ErrRegionUnavailable):
		return categorize(ErrRegionUnavailable, fmt.Errorf("%w: an allowed region is disabled for the account or a required service is not reachable there", ErrVerification))
	case errors.Is(err, ErrCapabilityUnverifiable):
		return categorize(ErrCapabilityUnverifiable, fmt.Errorf("%w: provisioning permissions could not be verified read-only; allow iam:SimulatePrincipalPolicy for this user", ErrVerification))
	case errors.Is(err, ErrUnsupportedPrincipal):
		return categorize(ErrUnsupportedPrincipal, fmt.Errorf("%w: use the long-term access key of a dedicated IAM user, not the root account or a temporary credential", ErrVerification))
	case errors.Is(err, ErrClusterUnreachable):
		return categorize(ErrClusterUnreachable, fmt.Errorf("%w: AWS could not be reached; retry later", ErrVerification))
	}
	return fmt.Errorf("%w: check the access key, the allowed regions and the permissions", ErrVerification)
}
