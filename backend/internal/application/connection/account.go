package connection

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/persistence"
)

// V01 infrastructure accounts (UC-04 AC-01..12, ADR-015). An account is the
// existing Connection record seen through a public AWS/INTERNAL type. Every
// mutation is a version CAS; provider and credential-store calls never run
// inside a database transaction.

// Public infrastructure account types.
const (
	AccountTypeAWS      = "AWS"
	AccountTypeInternal = "INTERNAL"
)

// AccountType projects the persisted kind onto the public account type.
func AccountType(kind application.ConnectionKind) string {
	if kind == application.ConnectionKubernetes {
		return AccountTypeInternal
	}
	return AccountTypeAWS
}

var (
	// ErrTargetMismatch means a replacement credential reaches another AWS
	// account or Internal cluster/context/endpoint/trust. The account is never
	// redirected; a new account must be registered.
	ErrTargetMismatch = errors.New("connection: the credential belongs to a different target; register a new account instead")
	// ErrWrongKind rejects an operation of the other account type.
	ErrWrongKind = fmt.Errorf("%w: the operation does not apply to this account type", ErrInvalid)
)

// SetClock replaces the time source of verification timestamps (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

func (s *Service) timestamp() string {
	now := time.Now
	if s.now != nil {
		now = s.now
	}
	return now().UTC().Format(time.RFC3339)
}

// ListAccounts returns every account of the Organization, oldest key first.
func (s *Service) ListAccounts(ctx context.Context, org string) ([]application.Connection, error) {
	return s.store.ListConnections(ctx, org)
}

// GetAccount returns one account of the Organization; a foreign or missing
// key is persistence.ErrNotFound.
func (s *Service) GetAccount(ctx context.Context, org, key string) (application.Connection, error) {
	return s.store.GetConnection(ctx, org, key)
}

// RenameAccount changes only the display name, at the expected version.
func (s *Service) RenameAccount(ctx context.Context, org, key string, version int64, rawName string) (application.Connection, error) {
	name, err := validName(rawName)
	if err != nil {
		return application.Connection{}, &FieldError{Field: "name"}
	}
	conn, err := s.guarded(ctx, org, key, version)
	if err != nil {
		return application.Connection{}, err
	}
	return s.store.UpdateConnection(ctx, updateOf(conn, version, name, conn.SecretRef, conn.Config, conn.Verification, conn.Status))
}

// guarded loads the account and rejects a stale expected version early; the
// final CAS still decides.
func (s *Service) guarded(ctx context.Context, org, key string, version int64) (application.Connection, error) {
	conn, err := s.store.GetConnection(ctx, org, key)
	if err != nil {
		return application.Connection{}, err
	}
	if version < 1 {
		return application.Connection{}, &FieldError{Field: "version"}
	}
	if conn.Version != version {
		return application.Connection{}, fmt.Errorf("%w: account %q", persistence.ErrVersionConflict, key)
	}
	return conn, nil
}

func updateOf(conn application.Connection, version int64, name, ref string, config, verification map[string]any, status application.ConnectionStatus) persistence.ConnectionUpdate {
	return persistence.ConnectionUpdate{
		OrganizationKey: conn.OrganizationKey, Key: conn.Key, ExpectedVersion: version,
		Name: name, Config: config, SecretRef: ref, Status: status, Verification: verification,
	}
}

// storedCredential reads the account's private credential for a read-only
// check. Nothing is returned to the caller beyond the verification result.
func (s *Service) storedCredential(ctx context.Context, conn application.Connection) ([]byte, error) {
	if s.credentials == nil {
		return nil, ErrCredentialStore
	}
	if conn.SecretRef == "" {
		return nil, fmt.Errorf("%w: the account has no stored credential to check", ErrVerification)
	}
	value, err := s.credentials.Get(ctx, conn.OrganizationKey, conn.Key, conn.SecretRef)
	if err != nil {
		return nil, ErrCredentialStore
	}
	if len(value) == 0 {
		return nil, fmt.Errorf("%w: the stored credential is not usable", ErrVerification)
	}
	return value, nil
}

// VerifyAccount rechecks the stored credential with read-only calls and, only
// on success, publishes the fresh verification and the next version. A failed
// check returns its safe error and leaves the committed record untouched.
// regions is AWS-only and optional (the stored evidence list is the default).
func (s *Service) VerifyAccount(ctx context.Context, org, key string, version int64, regions []string) (application.Connection, error) {
	conn, err := s.guarded(ctx, org, key, version)
	if err != nil {
		return application.Connection{}, err
	}
	if conn.Kind == application.ConnectionKubernetes && regions != nil {
		return application.Connection{}, &FieldError{Field: "verificationRegions"}
	}
	value, err := s.storedCredential(ctx, conn)
	if err != nil {
		return application.Connection{}, err
	}
	config, verification := cloneConfig(conn.Config), map[string]any{}
	switch conn.Kind {
	case application.ConnectionKubernetes:
		if s.kubeconfig == nil {
			return application.Connection{}, fmt.Errorf("%w: Kubernetes verifier is unavailable", ErrVerification)
		}
		selected, err := NormalizeKubeconfig(string(value), conn.ConfigString("kubeContext"))
		if err != nil || selected.Context.Endpoint != conn.ConfigString("endpoint") || selected.Context.Cluster != conn.ConfigString("cluster") {
			return application.Connection{}, fmt.Errorf("%w: the stored credential does not match the account target", ErrVerification)
		}
		verified, err := s.kubeconfig.VerifyKubeconfig(ctx, selected)
		if err != nil {
			return application.Connection{}, kubeconfigVerificationError(err)
		}
		verification = map[string]any{"verified": true, "endpoint": selected.Context.Endpoint, "serverVersion": verified.Version, "verifiedAt": s.timestamp()}
	case application.ConnectionAWS:
		if regions == nil {
			regions = accountRegions(conn)
		}
		regions, err = validAWSRegions(regions, "verificationRegions")
		if err != nil {
			return application.Connection{}, err
		}
		verified, err := s.verifyStoredAWS(ctx, value, regions)
		if err != nil {
			return application.Connection{}, err
		}
		if want := conn.ConfigString("accountId"); want != "" && verified.AccountID != want {
			return application.Connection{}, fmt.Errorf("%w: the stored credential does not match the account target", ErrVerification)
		}
		config["verificationRegions"], config["principalArn"] = regions, verified.PrincipalARN
		verification = s.awsVerificationRecord(verified, regions)
	default:
		return application.Connection{}, ErrWrongKind
	}
	return s.store.UpdateConnection(ctx, updateOf(conn, version, conn.Name, conn.SecretRef, config, verification, application.ConnectionReady))
}

func (s *Service) verifyStoredAWS(ctx context.Context, raw []byte, regions []string) (AWSVerification, error) {
	var value awsAccessKeyValue
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || value.AccessKeyID == "" || value.SecretAccessKey == "" {
		return AWSVerification{}, fmt.Errorf("%w: the stored credential is not usable", ErrVerification)
	}
	return s.verifyAWSKey(ctx, AWSAccessKey{AccessKeyID: value.AccessKeyID, SecretAccessKey: value.SecretAccessKey}, regions)
}

func (s *Service) verifyAWSKey(ctx context.Context, key AWSAccessKey, regions []string) (AWSVerification, error) {
	if s.aws == nil {
		return AWSVerification{}, fmt.Errorf("%w: AWS verifier is unavailable", ErrVerification)
	}
	verified, err := s.aws.VerifyAWS(ctx, key, regions)
	if err != nil {
		return AWSVerification{}, awsVerificationError(err)
	}
	if !completeAWSVerification(verified, regions) {
		return AWSVerification{}, fmt.Errorf("%w: the verification result is incomplete", ErrVerification)
	}
	return verified, nil
}

// accountRegions is the stored AWS evidence list: verificationRegions, or the
// list a legacy-registered record was verified for.
func accountRegions(conn application.Connection) []string {
	if regions := conn.ConfigStrings("verificationRegions"); len(regions) > 0 {
		return regions
	}
	return conn.AllowedRegions()
}

// AccountRegions exposes the evidence list for the public projection.
func AccountRegions(conn application.Connection) []string { return accountRegions(conn) }

func cloneConfig(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+2)
	for k, v := range in {
		out[k] = v
	}
	return out
}

// ReplaceInternalCredentials swaps the kubeconfig of an Internal account for
// one that reaches exactly the same cluster, context, endpoint and TLS trust.
func (s *Service) ReplaceInternalCredentials(ctx context.Context, org, key string, version int64, document, contextName string) (application.Connection, error) {
	selected, err := NormalizeKubeconfig(document, contextName)
	if err != nil {
		return application.Connection{}, err
	}
	conn, err := s.guarded(ctx, org, key, version)
	if err != nil {
		return application.Connection{}, err
	}
	if conn.Kind != application.ConnectionKubernetes {
		return application.Connection{}, ErrWrongKind
	}
	if s.credentials == nil {
		return application.Connection{}, ErrCredentialStore
	}
	if err := s.sameInternalTarget(ctx, conn, selected); err != nil {
		return application.Connection{}, err
	}
	if s.kubeconfig == nil {
		return application.Connection{}, fmt.Errorf("%w: Kubernetes verifier is unavailable", ErrVerification)
	}
	verified, err := s.kubeconfig.VerifyKubeconfig(ctx, selected)
	if err != nil {
		return application.Connection{}, kubeconfigVerificationError(err)
	}
	config := cloneConfig(conn.Config)
	tls := kubeTLS(selected)
	config["tlsVerification"], config["tlsCustomCA"] = tls.verification, tls.customCA
	verification := map[string]any{"verified": true, "endpoint": selected.Context.Endpoint, "serverVersion": verified.Version, "verifiedAt": s.timestamp()}
	return s.publishReplacement(ctx, conn, version, selected.Document, config, verification)
}

// sameInternalTarget proves the replacement reaches the committed target. The
// committed credential is the only source of the prior TLS trust, so a record
// whose credential cannot be read is not replaceable.
func (s *Service) sameInternalTarget(ctx context.Context, conn application.Connection, next SelectedKubeconfig) error {
	if conn.SecretRef == "" {
		return ErrTargetMismatch
	}
	if conn.ConfigString("cluster") != next.Context.Cluster || conn.ConfigString("kubeContext") != next.Context.Name ||
		conn.ConfigString("endpoint") != next.Context.Endpoint {
		return ErrTargetMismatch
	}
	prior, err := s.storedCredential(ctx, conn)
	if err != nil {
		return err
	}
	old, err := NormalizeKubeconfig(string(prior), conn.ConfigString("kubeContext"))
	if err != nil || kubeTLS(old).trust != kubeTLS(next).trust {
		return ErrTargetMismatch
	}
	return nil
}

// ReplaceAWSCredentials swaps the access key of an AWS account for another
// key of the same AWS account ID.
func (s *Service) ReplaceAWSCredentials(ctx context.Context, org, key string, version int64, cmd RegisterAWSCommand) (application.Connection, error) {
	if !awsAccessKeyPattern.MatchString(cmd.AccessKeyID) {
		return application.Connection{}, &FieldError{Field: "accessKeyId"}
	}
	if !awsSecretKeyPattern.MatchString(cmd.SecretAccessKey) {
		return application.Connection{}, &FieldError{Field: "secretAccessKey"}
	}
	regions, err := validAWSRegions(cmd.AllowedRegions, "verificationRegions")
	if err != nil {
		return application.Connection{}, err
	}
	conn, err := s.guarded(ctx, org, key, version)
	if err != nil {
		return application.Connection{}, err
	}
	if conn.Kind != application.ConnectionAWS {
		return application.Connection{}, ErrWrongKind
	}
	if s.credentials == nil {
		return application.Connection{}, ErrCredentialStore
	}
	verified, err := s.verifyAWSKey(ctx, AWSAccessKey{AccessKeyID: cmd.AccessKeyID, SecretAccessKey: cmd.SecretAccessKey}, regions)
	if err != nil {
		return application.Connection{}, err
	}
	if conn.ConfigString("accountId") == "" || verified.AccountID != conn.ConfigString("accountId") {
		return application.Connection{}, ErrTargetMismatch
	}
	value, err := json.Marshal(awsAccessKeyValue{AccessKeyID: cmd.AccessKeyID, SecretAccessKey: cmd.SecretAccessKey})
	if err != nil {
		return application.Connection{}, ErrCredentialStore
	}
	config := cloneConfig(conn.Config)
	config["verificationRegions"], config["principalArn"] = regions, verified.PrincipalARN
	return s.publishReplacement(ctx, conn, version, value, config, s.awsVerificationRecord(verified, regions))
}

// publishReplacement writes one immutable attempt object and then publishes
// ref, config, verification and the next version in a single CAS. A failed or
// lost CAS deletes only this attempt's object; the prior committed object is
// kept because persisted targets may still pin it. If the CAS outcome is
// unknown the committed record decides whether the attempt object is live.
func (s *Service) publishReplacement(ctx context.Context, conn application.Connection, version int64, value []byte, config, verification map[string]any) (application.Connection, error) {
	ref, err := s.credentials.Put(ctx, conn.OrganizationKey, conn.Key, value)
	if err != nil {
		if ref != "" && ref != conn.SecretRef {
			if cleanupErr := s.cleanup(ctx, conn.OrganizationKey, conn.Key, ref); cleanupErr != nil {
				return application.Connection{}, cleanupErr
			}
		}
		return application.Connection{}, ErrCredentialStore
	}
	updated, err := s.store.UpdateConnection(ctx, updateOf(conn, version, conn.Name, ref, config, verification, application.ConnectionReady))
	if err == nil {
		return updated, nil
	}
	if !errors.Is(err, persistence.ErrVersionConflict) && !errors.Is(err, persistence.ErrNotFound) {
		// Unknown outcome (for example a lost commit response): a committed
		// attempt object must survive.
		readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
		current, readErr := s.store.GetConnection(readCtx, conn.OrganizationKey, conn.Key)
		cancel()
		if readErr != nil {
			// Cannot prove the attempt lost: keep the object and report it.
			return application.Connection{}, s.cleanupFailure(conn, ref)
		}
		if current.SecretRef == ref {
			return current, nil
		}
	}
	if ref != conn.SecretRef {
		if cleanupErr := s.cleanup(ctx, conn.OrganizationKey, conn.Key, ref); cleanupErr != nil {
			return application.Connection{}, cleanupErr
		}
	}
	return application.Connection{}, err
}

// kubeTLSInfo is the safe TLS description of a selected kubeconfig. trust is a
// digest of the cluster trust inputs and is compared, never exposed.
type kubeTLSInfo struct {
	verification string
	customCA     bool
	trust        string
}

func kubeTLS(selected SelectedKubeconfig) kubeTLSInfo {
	var doc struct {
		Clusters []struct {
			Cluster struct {
				CA         string `json:"certificate-authority-data"`
				Insecure   bool   `json:"insecure-skip-tls-verify"`
				ServerName string `json:"tls-server-name"`
			} `json:"cluster"`
		} `json:"clusters"`
	}
	info := kubeTLSInfo{verification: "ENABLED"}
	if json.Unmarshal(selected.Document, &doc) != nil || len(doc.Clusters) != 1 {
		info.verification, info.trust = "UNKNOWN", "unknown"
		return info
	}
	c := doc.Clusters[0].Cluster
	if c.Insecure {
		info.verification = "DISABLED"
	}
	info.customCA = c.CA != ""
	sum := sha256.Sum256([]byte(fmt.Sprintf("insecure=%t\nserverName=%s\nca=%s", c.Insecure, c.ServerName, c.CA)))
	info.trust = hex.EncodeToString(sum[:])
	return info
}

// cleanupFailure records, with safe identifiers only, an attempt object whose
// fate could not be established.
func (s *Service) cleanupFailure(conn application.Connection, ref string) error {
	reference := ids.New()
	log.Printf("connection: replacement outcome unknown, object kept (reference %s, organization %s, connection %s, object %s)", reference, conn.OrganizationKey, conn.Key, objectID(ref))
	return &CleanupError{Reference: reference}
}
