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
	"orchestrator/internal/ports/credentials"
	"orchestrator/internal/ports/persistence"
)

// V01 infrastructure accounts (UC-04 AC-01..12) are the existing Connection
// records with an optimistic version. Every mutation is a compare-and-set on
// that version; provider and credential-store work never runs inside the
// database transaction.

var (
	// ErrAccountIncompatible means the credential or stored record does not
	// belong to the account's existing target (another AWS account, another
	// cluster/context/endpoint/trust), or the record has no verifiable target.
	ErrAccountIncompatible = errors.New("connection: the credential does not match the account target")
	// ErrOutcomeUncertain means the database commit may have happened. Any new
	// credential object is kept and nothing is retried; the caller reloads.
	ErrOutcomeUncertain = errors.New("connection: the outcome is not known; reload the account before trying again")
	// ErrAccountCredential means the account's stored credential is missing,
	// out of scope or unusable.
	ErrAccountCredential = errors.New("connection: the stored account credential is unavailable")
)

func (s *Service) timestamp() string {
	now := time.Now
	if s.now != nil {
		now = s.now
	}
	return now().UTC().Format(time.RFC3339)
}

// kubeTarget is the destination identity of an Internal account beyond its
// cluster/context/endpoint: the TLS trust the selected cluster entry declares.
type kubeTarget struct {
	Trust      string // sha256 over the trust-defining cluster fields
	Mode       string // CUSTOM_CA, INSECURE_SKIP_VERIFY or SYSTEM_ROOTS
	ServerName string
}

func (t kubeTarget) addTo(config map[string]any) {
	config["tlsTrust"], config["tlsMode"] = t.Trust, t.Mode
	if t.ServerName != "" {
		config["tlsServerName"] = t.ServerName
	}
}

// kubeconfigTarget derives the trust identity from a normalized
// selected-context document (the form held by the credential store).
func kubeconfigTarget(document []byte) (kubeTarget, error) {
	var doc struct {
		Clusters []struct {
			Cluster struct {
				Server   string `json:"server"`
				CA       string `json:"certificate-authority-data"`
				Insecure bool   `json:"insecure-skip-tls-verify"`
				Name     string `json:"tls-server-name"`
			} `json:"cluster"`
		} `json:"clusters"`
	}
	if json.Unmarshal(document, &doc) != nil || len(doc.Clusters) != 1 {
		return kubeTarget{}, ErrKubeconfigInvalid
	}
	c := doc.Clusters[0].Cluster
	mode := "SYSTEM_ROOTS"
	switch {
	case c.Insecure:
		mode = "INSECURE_SKIP_VERIFY"
	case c.CA != "":
		mode = "CUSTOM_CA"
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("v1\nserver=%s\nmode=%s\nca=%s\nname=%s", c.Server, mode, c.CA, c.Name)))
	return kubeTarget{Trust: hex.EncodeToString(sum[:]), Mode: mode, ServerName: c.Name}, nil
}

func cloneConfig(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+4)
	for k, v := range in {
		out[k] = v
	}
	return out
}

// ListAccounts returns the Organization's backing records (safe projection is
// the delivery layer's job).
func (s *Service) ListAccounts(ctx context.Context, org string) ([]application.Connection, error) {
	return s.store.ListConnections(ctx, org)
}

// GetAccount returns one record of the Organization; a foreign or missing key
// is persistence.ErrNotFound.
func (s *Service) GetAccount(ctx context.Context, org, key string) (application.Connection, error) {
	conn, err := s.store.GetConnection(ctx, org, key)
	if err != nil {
		return application.Connection{}, err
	}
	if conn.OrganizationKey != org || conn.Key != key {
		return application.Connection{}, persistence.ErrNotFound
	}
	return conn, nil
}

func (s *Service) loadAccount(ctx context.Context, org, key string, version int64) (application.Connection, error) {
	if version < 1 {
		return application.Connection{}, &FieldError{Field: "version"}
	}
	conn, err := s.GetAccount(ctx, org, key)
	if err != nil {
		return application.Connection{}, err
	}
	if conn.Version != version {
		return application.Connection{}, fmt.Errorf("%w: account version", persistence.ErrVersionConflict)
	}
	return conn, nil
}

// publish applies mutate to the freshly read record inside one local
// transaction and compare-and-sets it at expected. No external I/O happens
// here.
func (s *Service) publish(ctx context.Context, org, key string, expected int64, mutate func(cur application.Connection) application.Connection) (application.Connection, error) {
	var updated application.Connection
	err := s.store.Transact(ctx, func(ctx context.Context) error {
		cur, err := s.store.GetConnection(ctx, org, key)
		if err != nil {
			return err
		}
		if cur.Version != expected {
			return fmt.Errorf("%w: account version", persistence.ErrVersionConflict)
		}
		updated, err = s.store.UpdateConnection(ctx, org, key, expected, mutate(cur))
		return err
	})
	if errors.Is(err, persistence.ErrCommitUncertain) {
		return application.Connection{}, ErrOutcomeUncertain
	}
	return updated, err
}

// RenameAccount changes only the display name.
func (s *Service) RenameAccount(ctx context.Context, org, key string, version int64, rawName string) (application.Connection, error) {
	name, err := validName(rawName)
	if err != nil {
		return application.Connection{}, &FieldError{Field: "name"}
	}
	if _, err := s.loadAccount(ctx, org, key, version); err != nil {
		return application.Connection{}, err
	}
	return s.publish(ctx, org, key, version, func(cur application.Connection) application.Connection {
		next := cur
		next.Name = name
		return next
	})
}

// VerifyAccountCommand is the body of the explicit connection check.
// VerificationRegions is AWS-only; nil rechecks the stored evidence list.
type VerifyAccountCommand struct {
	Version             int64
	VerificationRegions []string
}

// storedAWSRegions are the evidence regions to recheck for an account.
func storedAWSRegions(conn application.Connection) []string {
	if regions := conn.ConfigStrings("verificationRegions"); len(regions) > 0 {
		return regions
	}
	return conn.AllowedRegions()
}

// readStored fetches the account's stored credential privately.
func (s *Service) readStored(ctx context.Context, conn application.Connection) ([]byte, error) {
	if s.credentials == nil {
		return nil, ErrCredentialStore
	}
	if conn.SecretRef == "" {
		return nil, ErrAccountCredential
	}
	value, err := s.credentials.Get(ctx, conn.OrganizationKey, conn.Key, conn.SecretRef)
	switch {
	case err == nil && len(value) > 0:
		return value, nil
	case err == nil, errors.Is(err, credentials.ErrNotFound), errors.Is(err, credentials.ErrInvalidReference):
		return nil, ErrAccountCredential
	}
	return nil, ErrCredentialStore
}

// VerifyAccount rechecks the stored credential read-only and, only on
// success, publishes refreshed verification and an incremented version. A
// failure leaves the committed record untouched.
func (s *Service) VerifyAccount(ctx context.Context, org, key string, cmd VerifyAccountCommand) (application.Connection, error) {
	cur, err := s.loadAccount(ctx, org, key, cmd.Version)
	if err != nil {
		return application.Connection{}, err
	}
	if cur.Kind != application.ConnectionAWS && cmd.VerificationRegions != nil {
		return application.Connection{}, &FieldError{Field: "verificationRegions"}
	}
	if _, err := s.store.GetOrganization(ctx, org); err != nil {
		return application.Connection{}, err
	}
	switch cur.Kind {
	case application.ConnectionKubernetes:
		return s.verifyKubernetes(ctx, cur)
	case application.ConnectionAWS:
		return s.verifyAWS(ctx, cur, cmd.VerificationRegions)
	}
	return application.Connection{}, ErrAccountIncompatible
}

func (s *Service) verifyKubernetes(ctx context.Context, cur application.Connection) (application.Connection, error) {
	if s.kubeconfig == nil {
		return application.Connection{}, fmt.Errorf("%w: Kubernetes verifier is unavailable", ErrVerification)
	}
	value, err := s.readStored(ctx, cur)
	if err != nil {
		return application.Connection{}, err
	}
	selected, err := NormalizeKubeconfig(string(value), cur.ConfigString("kubeContext"))
	if err != nil || selected.Context.Endpoint != cur.ConfigString("endpoint") || selected.Context.Cluster != cur.ConfigString("cluster") {
		return application.Connection{}, ErrAccountIncompatible
	}
	verified, err := s.kubeconfig.VerifyKubeconfig(ctx, selected)
	if err != nil {
		return application.Connection{}, kubeconfigVerificationError(err)
	}
	return s.publish(ctx, cur.OrganizationKey, cur.Key, cur.Version, func(c application.Connection) application.Connection {
		next := c
		next.Status = application.ConnectionReady
		next.Verification = map[string]any{"verified": true, "endpoint": selected.Context.Endpoint, "serverVersion": verified.Version, "verifiedAt": s.timestamp()}
		return next
	})
}

func (s *Service) verifyAWS(ctx context.Context, cur application.Connection, requested []string) (application.Connection, error) {
	if s.aws == nil {
		return application.Connection{}, fmt.Errorf("%w: AWS verifier is unavailable", ErrVerification)
	}
	regions := storedAWSRegions(cur)
	if requested != nil {
		var err error
		if regions, err = validAWSRegions(requested, "verificationRegions"); err != nil {
			return application.Connection{}, err
		}
	}
	if len(regions) == 0 {
		return application.Connection{}, ErrAccountIncompatible
	}
	value, err := s.readStored(ctx, cur)
	if err != nil {
		return application.Connection{}, err
	}
	stored, err := decodeAWSValue(value)
	if err != nil {
		return application.Connection{}, ErrAccountCredential
	}
	verified, err := s.aws.VerifyAWS(ctx, AWSAccessKey{AccessKeyID: stored.AccessKeyID, SecretAccessKey: stored.SecretAccessKey}, regions)
	if err != nil {
		return application.Connection{}, awsVerificationError(err)
	}
	if !completeAWSVerification(verified, regions) {
		return application.Connection{}, fmt.Errorf("%w: the verification result is incomplete", ErrVerification)
	}
	if verified.AccountID != cur.ConfigString("accountId") {
		return application.Connection{}, ErrAccountIncompatible
	}
	return s.publish(ctx, cur.OrganizationKey, cur.Key, cur.Version, func(c application.Connection) application.Connection {
		next := c
		next.Status = application.ConnectionReady
		next.Config = cloneConfig(c.Config)
		next.Config["verificationRegions"] = regions
		next.Config["principalArn"] = verified.PrincipalARN
		next.Verification = awsVerificationRecord(verified, regions, s.timestamp())
		return next
	})
}

func decodeAWSValue(raw []byte) (awsAccessKeyValue, error) {
	var value awsAccessKeyValue
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || value.AccessKeyID == "" || value.SecretAccessKey == "" {
		return awsAccessKeyValue{}, ErrAccountCredential
	}
	return value, nil
}

func awsVerificationRecord(v AWSVerification, regions []string, at string) map[string]any {
	return map[string]any{
		"verified": true, "accountId": v.AccountID, "principalArn": v.PrincipalARN,
		"region": regions[0], "regions": v.Regions,
		"identity": "VERIFIED", "regionCheck": "VERIFIED", "capabilityCheck": "VERIFIED",
		"capabilities": v.Capabilities, "verifiedAt": at,
	}
}

// ReplaceKubeconfigCommand is the Internal credential replacement body.
type ReplaceKubeconfigCommand struct {
	Version    int64
	Kubeconfig string
	Context    string
}

// ReplaceKubeconfigCredential swaps in a verified kubeconfig for the same
// cluster/context/endpoint/TLS trust.
func (s *Service) ReplaceKubeconfigCredential(ctx context.Context, org, key string, cmd ReplaceKubeconfigCommand) (application.Connection, error) {
	cur, err := s.loadAccount(ctx, org, key, cmd.Version)
	if err != nil {
		return application.Connection{}, err
	}
	if cur.Kind != application.ConnectionKubernetes {
		return application.Connection{}, ErrAccountIncompatible
	}
	selected, err := NormalizeKubeconfig(cmd.Kubeconfig, cmd.Context)
	if err != nil {
		return application.Connection{}, err
	}
	if s.credentials == nil {
		return application.Connection{}, ErrCredentialStore
	}
	if s.kubeconfig == nil {
		return application.Connection{}, fmt.Errorf("%w: Kubernetes verifier is unavailable", ErrVerification)
	}
	if _, err := s.store.GetOrganization(ctx, org); err != nil {
		return application.Connection{}, err
	}
	incoming, err := kubeconfigTarget(selected.Document)
	if err != nil {
		return application.Connection{}, err
	}
	trust := cur.ConfigString("tlsTrust")
	if trust == "" {
		// A record created before trust metadata existed: derive it from the
		// stored credential; fail closed when that is not possible.
		stored, err := s.readStored(ctx, cur)
		if err != nil {
			return application.Connection{}, ErrAccountIncompatible
		}
		current, err := kubeconfigTarget(stored)
		if err != nil {
			return application.Connection{}, ErrAccountIncompatible
		}
		trust = current.Trust
	}
	if cur.ConfigString("kubeContext") == "" || selected.Context.Name != cur.ConfigString("kubeContext") ||
		selected.Context.Cluster != cur.ConfigString("cluster") || selected.Context.Endpoint != cur.ConfigString("endpoint") ||
		incoming.Trust != trust {
		return application.Connection{}, ErrAccountIncompatible
	}
	verified, err := s.kubeconfig.VerifyKubeconfig(ctx, selected)
	if err != nil {
		return application.Connection{}, kubeconfigVerificationError(err)
	}
	return s.replaceCredential(ctx, cur, selected.Document, func(c application.Connection, ref string) application.Connection {
		next := c
		next.Config = cloneConfig(c.Config)
		incoming.addTo(next.Config)
		next.SecretRef, next.Status = ref, application.ConnectionReady
		next.Verification = map[string]any{"verified": true, "endpoint": selected.Context.Endpoint, "serverVersion": verified.Version, "verifiedAt": s.timestamp()}
		return next
	})
}

// ReplaceAWSCommand is the AWS credential replacement body.
type ReplaceAWSCommand struct {
	Version             int64
	AccessKeyID         string
	SecretAccessKey     string
	VerificationRegions []string
}

func (ReplaceAWSCommand) String() string               { return "ReplaceAWSCommand{redacted}" }
func (ReplaceAWSCommand) GoString() string             { return "ReplaceAWSCommand{redacted}" }
func (c ReplaceAWSCommand) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte(c.String())) }

// ReplaceAWSCredential swaps in a verified access key of the same AWS account.
func (s *Service) ReplaceAWSCredential(ctx context.Context, org, key string, cmd ReplaceAWSCommand) (application.Connection, error) {
	cur, err := s.loadAccount(ctx, org, key, cmd.Version)
	if err != nil {
		return application.Connection{}, err
	}
	if cur.Kind != application.ConnectionAWS {
		return application.Connection{}, ErrAccountIncompatible
	}
	if !awsAccessKeyPattern.MatchString(cmd.AccessKeyID) {
		return application.Connection{}, &FieldError{Field: "accessKeyId"}
	}
	if !awsSecretKeyPattern.MatchString(cmd.SecretAccessKey) {
		return application.Connection{}, &FieldError{Field: "secretAccessKey"}
	}
	regions, err := validAWSRegions(cmd.VerificationRegions, "verificationRegions")
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
	if cur.ConfigString("accountId") == "" {
		return application.Connection{}, ErrAccountIncompatible
	}
	// Legacy consumers still resolve this credential in every retained legacy
	// allowed region, so the new key must pass checks there too (plus the new
	// evidence regions) before anything is published. Nothing is rerouted.
	checked := append([]string(nil), regions...)
	seen := map[string]bool{}
	for _, region := range regions {
		seen[region] = true
	}
	for _, region := range cur.AllowedRegions() {
		if !seen[region] {
			checked = append(checked, region)
			seen[region] = true
		}
	}
	verified, err := s.aws.VerifyAWS(ctx, AWSAccessKey{AccessKeyID: cmd.AccessKeyID, SecretAccessKey: cmd.SecretAccessKey}, checked)
	if err != nil {
		return application.Connection{}, awsVerificationError(err)
	}
	if !completeAWSVerification(verified, checked) {
		return application.Connection{}, fmt.Errorf("%w: the verification result is incomplete", ErrVerification)
	}
	if verified.AccountID != cur.ConfigString("accountId") {
		return application.Connection{}, ErrAccountIncompatible
	}
	value, err := json.Marshal(awsAccessKeyValue{AccessKeyID: cmd.AccessKeyID, SecretAccessKey: cmd.SecretAccessKey})
	if err != nil {
		return application.Connection{}, ErrCredentialStore
	}
	return s.replaceCredential(ctx, cur, value, func(c application.Connection, ref string) application.Connection {
		next := c
		next.Config = cloneConfig(c.Config)
		next.Config["verificationRegions"] = regions
		next.Config["principalArn"] = verified.PrincipalARN
		next.SecretRef, next.Status = ref, application.ConnectionReady
		// Evidence belongs to the new key only: every region it was checked in.
		next.Verification = awsVerificationRecord(verified, checked, s.timestamp())
		return next
	})
}

// replaceCredential writes one new immutable attempt object, then publishes
// its reference with the CAS. A failed or lost publish deletes only that new
// object; the previously committed object is never touched because persisted
// targets may still pin it.
func (s *Service) replaceCredential(ctx context.Context, cur application.Connection, value []byte, build func(c application.Connection, ref string) application.Connection) (application.Connection, error) {
	ref, err := s.credentials.Put(ctx, cur.OrganizationKey, cur.Key, value)
	if err != nil {
		if ref != "" {
			if cleanupErr := s.cleanup(ctx, cur.OrganizationKey, cur.Key, ref); cleanupErr != nil {
				return application.Connection{}, cleanupErr
			}
		}
		return application.Connection{}, ErrCredentialStore
	}
	updated, err := s.publish(ctx, cur.OrganizationKey, cur.Key, cur.Version, func(c application.Connection) application.Connection {
		return build(c, ref)
	})
	if err != nil {
		if errors.Is(err, ErrOutcomeUncertain) {
			// The row may point at the new object: keep it.
			log.Printf("connection: replacement outcome uncertain; new credential object retained (organization %s, connection %s, object %s)", cur.OrganizationKey, cur.Key, objectID(ref))
			return application.Connection{}, err
		}
		if cleanupErr := s.cleanup(ctx, cur.OrganizationKey, cur.Key, ref); cleanupErr != nil {
			return application.Connection{}, cleanupErr
		}
		return application.Connection{}, err
	}
	return updated, nil
}

// SetClock replaces the verification timestamp source (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }
