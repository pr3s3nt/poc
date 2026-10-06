// Package connection implements UC-04 Connection registration: the kubeconfig
// upload flow with a scoped credential store, and the explicit legacy
// host-context compatibility flow.
package connection

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/ports/credentials"
	"orchestrator/internal/ports/persistence"
)

var ErrInvalid = errors.New("connection: invalid registration")
var ErrDuplicate = errors.New("connection: duplicate id")
var ErrVerification = errors.New("connection: verification failed")

// ErrCredentialStore is ERR-07: no durable credential store is available or
// the credential could not be stored. Nothing READY was saved; retry is safe.
var ErrCredentialStore = errors.New("connection: the credential could not be stored; no connection was saved, retry later")

// CleanupError is ERR-08 when rollback of the attempt's own credential also
// failed. Reference is a safe operational identifier, never a credential.
type CleanupError struct{ Reference string }

func (e *CleanupError) Error() string {
	return "connection: registration failed and its credential cleanup did not complete (reference " + e.Reference + "); contact an operator"
}

// Verification failure categories a verifier may wrap. Only their fixed
// guidance reaches the caller; raw kubectl/provider output never does.
var (
	ErrContextMissing     = errors.New("kube context is not configured on the backend host")
	ErrClusterUnreachable = errors.New("cluster API is not reachable with this context")
	ErrPermissionDenied   = errors.New("context lacks a required permission")
	ErrAuthentication     = errors.New("cluster rejected the supplied credential")
)

const permissionGuidance = "create namespaces, and deployments, statefulsets, services and secrets in all namespaces"

func verificationError(err error) error {
	switch {
	case errors.Is(err, ErrContextMissing):
		return fmt.Errorf("%w: the kube context is not configured on the backend host; configure it there first", ErrVerification)
	case errors.Is(err, ErrPermissionDenied):
		return fmt.Errorf("%w: the context lacks a required permission (%s)", ErrVerification, permissionGuidance)
	case errors.Is(err, ErrClusterUnreachable):
		return fmt.Errorf("%w: the cluster API could not be reached with this context", ErrVerification)
	}
	return fmt.Errorf("%w: check that the context exists on the backend host, its API is reachable and it has the required permissions", ErrVerification)
}

// kubeconfigVerificationError maps upload verification failures (ERR-05/06).
func kubeconfigVerificationError(err error) error {
	switch {
	case errors.Is(err, ErrPermissionDenied):
		return fmt.Errorf("%w: the selected context lacks a required permission (%s)", ErrVerification, permissionGuidance)
	case errors.Is(err, ErrAuthentication):
		return fmt.Errorf("%w: the cluster rejected the credential of the selected context; check that the token or client certificate is current", ErrVerification)
	case errors.Is(err, ErrClusterUnreachable):
		return fmt.Errorf("%w: the cluster API could not be reached at the selected endpoint; check the server address, network access and TLS settings", ErrVerification)
	}
	return fmt.Errorf("%w: check that the selected endpoint is reachable, the credential is accepted and it has the required permissions", ErrVerification)
}

var connectionKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,40}$`)

const (
	maxNameLength      = 100
	maxKeySlugLength   = 32
	maxRegisterRetries = 5
	cleanupTimeout     = 15 * time.Second
)

type KubernetesVerification struct {
	Endpoint string
	Version  string
}

// KubernetesVerifier checks the host's named context without creating objects.
type KubernetesVerifier interface {
	Verify(ctx context.Context, kubeContext string) (KubernetesVerification, error)
}

// KubeconfigVerifier checks the API and RBAC of a normalized selected-context
// kubeconfig without creating objects and without host credentials.
type KubeconfigVerifier interface {
	VerifyKubeconfig(ctx context.Context, selected SelectedKubeconfig) (KubernetesVerification, error)
}

type Service struct {
	store       persistence.Store
	verifier    KubernetesVerifier
	kubeconfig  KubeconfigVerifier
	credentials credentials.Store
}

// NewService wires the legacy host-context verifier. Upload registration also
// needs SetKubeconfigRegistration.
func NewService(store persistence.Store, verifier KubernetesVerifier) *Service {
	return &Service{store: store, verifier: verifier}
}

// SetKubeconfigRegistration wires the upload verifier and credential store.
// A nil store makes upload registration fail with ErrCredentialStore.
func (s *Service) SetKubeconfigRegistration(verifier KubeconfigVerifier, store credentials.Store) {
	s.kubeconfig, s.credentials = verifier, store
}

// RegisterKubernetesCommand is the explicit legacy host-context body.
type RegisterKubernetesCommand struct {
	Key         string `json:"key"`
	ClusterID   string `json:"clusterId"`
	KubeContext string `json:"kubeContext"`
}

// RegisterKubeconfigCommand is the upload body (UC-04 VAR-01).
type RegisterKubeconfigCommand struct {
	Name       string `json:"name"`
	Kubeconfig string `json:"kubeconfig"`
	Context    string `json:"context"`
}

// InspectKubeconfig returns safe context/cluster/endpoint metadata. It stores
// nothing and contacts no cluster.
func (s *Service) InspectKubeconfig(_ context.Context, _ string, document string) ([]KubeconfigContext, error) {
	return InspectKubeconfig(document)
}

// RegisterKubeconfig revalidates the document, verifies the selected context,
// writes its credential to the scoped store and inserts a READY Connection.
// Insert failure deletes only this attempt's credential.
func (s *Service) RegisterKubeconfig(ctx context.Context, org string, cmd RegisterKubeconfigCommand) (application.Connection, error) {
	name, err := validName(cmd.Name)
	if err != nil {
		return application.Connection{}, err
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
	verified, err := s.kubeconfig.VerifyKubeconfig(ctx, selected)
	if err != nil {
		return application.Connection{}, kubeconfigVerificationError(err)
	}
	base := keySlug(name)
	for attempt := 0; attempt < maxRegisterRetries; attempt++ {
		key, err := s.freeKey(ctx, org, base)
		if err != nil {
			return application.Connection{}, err
		}
		ref, err := s.credentials.Put(ctx, org, key, selected.Document)
		if err != nil {
			if ref != "" {
				if cleanupErr := s.cleanup(ctx, org, key, ref); cleanupErr != nil {
					return application.Connection{}, cleanupErr
				}
			}
			return application.Connection{}, ErrCredentialStore
		}
		conn := application.Connection{
			Key: key, Name: name, OrganizationKey: org,
			Kind: application.ConnectionKubernetes, AuthenticationType: application.AuthKubeconfig,
			Config:    map[string]any{"cluster": selected.Context.Cluster, "kubeContext": selected.Context.Name, "endpoint": selected.Context.Endpoint},
			SecretRef: ref, Status: application.ConnectionReady,
			Verification: map[string]any{"verified": true, "endpoint": selected.Context.Endpoint, "serverVersion": verified.Version},
		}
		err = s.store.Transact(ctx, func(ctx context.Context) error {
			if _, err := s.store.GetOrganization(ctx, org); err != nil {
				return err
			}
			// Insert-only: the unique (organization, key) constraint is the
			// final concurrent guard; nothing existing is ever updated.
			return s.store.CreateConnection(ctx, conn)
		})
		if err == nil {
			return conn, nil
		}
		if cleanupErr := s.cleanup(ctx, org, key, ref); cleanupErr != nil {
			return application.Connection{}, cleanupErr
		}
		if !errors.Is(err, persistence.ErrDuplicate) {
			return application.Connection{}, err
		}
	}
	return application.Connection{}, fmt.Errorf("connection: could not allocate a unique connection ID; retry")
}

// cleanup deletes one attempt's credential with a bounded context that a
// cancelled request does not abort immediately.
func (s *Service) cleanup(ctx context.Context, org, key, ref string) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	if err := s.credentials.Delete(cleanupCtx, org, key, ref); err != nil {
		reference := ids.New()
		// Operational record: safe identifiers only, never the credential.
		log.Printf("connection: credential cleanup failed (reference %s, organization %s, connection %s, object %s)", reference, org, key, objectID(ref))
		return &CleanupError{Reference: reference}
	}
	return nil
}

func objectID(ref string) string {
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		return ref[i+1:]
	}
	return ""
}

// freeKey returns the readable slug or the first free collision suffix. It
// never selects a key that an existing Connection holds.
func (s *Service) freeKey(ctx context.Context, org, base string) (string, error) {
	existing, err := s.store.ListConnections(ctx, org)
	if err != nil {
		return "", err
	}
	taken := map[string]bool{}
	for _, conn := range existing {
		taken[conn.Key] = true
	}
	if !taken[base] {
		return base, nil
	}
	for n := 2; n < 100; n++ {
		candidate := fmt.Sprintf("%s-%d", base, n)
		if !taken[candidate] {
			return candidate, nil
		}
	}
	for {
		candidate := base + "-" + strings.ReplaceAll(ids.New(), "-", "")[:6]
		if !taken[candidate] {
			return candidate, nil
		}
	}
}

func validName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || utf8.RuneCountInString(name) > maxNameLength || !utf8.ValidString(name) ||
		strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("%w: connection name is required (at most %d characters, no control characters)", ErrInvalid, maxNameLength)
	}
	return name, nil
}

// keySlug derives a bounded readable key from the display name. Accents are
// folded, other characters become hyphens.
func keySlug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range norm.NFD.String(strings.ToLower(name)) {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case r == 'đ':
			r = 'd'
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > maxKeySlugLength {
		slug = strings.TrimRight(slug[:maxKeySlugLength], "-")
	}
	if slug == "" {
		slug = "connection"
	}
	return slug
}

// RegisterKubernetesCluster is the explicit legacy host-context registration.
// It is never a fallback for upload failures.
func (s *Service) RegisterKubernetesCluster(ctx context.Context, org string, cmd RegisterKubernetesCommand) (application.Connection, error) {
	cmd.Key = strings.TrimSpace(cmd.Key)
	cmd.ClusterID = strings.TrimSpace(cmd.ClusterID)
	cmd.KubeContext = strings.TrimSpace(cmd.KubeContext)
	if !connectionKeyPattern.MatchString(cmd.Key) || !connectionKeyPattern.MatchString(cmd.ClusterID) || cmd.KubeContext == "" || strings.ContainsAny(cmd.KubeContext, "\x00\n\r") {
		return application.Connection{}, fmt.Errorf("%w: valid connection ID, cluster ID and kube context are required", ErrInvalid)
	}
	if s.verifier == nil {
		return application.Connection{}, fmt.Errorf("%w: Kubernetes verifier is unavailable", ErrVerification)
	}
	verified, err := s.verifier.Verify(ctx, cmd.KubeContext)
	if err != nil {
		return application.Connection{}, verificationError(err)
	}
	if verified.Endpoint == "" {
		return application.Connection{}, verificationError(ErrClusterUnreachable)
	}
	conn := application.Connection{
		Key: cmd.Key, Name: cmd.Key, OrganizationKey: org,
		Kind: application.ConnectionKubernetes, AuthenticationType: application.AuthHostContext,
		Config:       map[string]any{"cluster": cmd.ClusterID, "kubeContext": cmd.KubeContext, "endpoint": verified.Endpoint},
		SecretRef:    "host-kube-context://" + url.PathEscape(cmd.KubeContext),
		Status:       application.ConnectionReady,
		Verification: map[string]any{"verified": true, "endpoint": verified.Endpoint, "serverVersion": verified.Version},
	}
	err = s.store.Transact(ctx, func(ctx context.Context) error {
		if _, err := s.store.GetOrganization(ctx, org); err != nil {
			return err
		}
		connections, err := s.store.ListConnections(ctx, org)
		if err != nil {
			return err
		}
		for _, existing := range connections {
			if existing.Key == cmd.Key {
				return fmt.Errorf("%w: connection %q", ErrDuplicate, cmd.Key)
			}
		}
		// Insert-only: the repository insert is the final duplicate guard.
		return s.store.CreateConnection(ctx, conn)
	})
	if errors.Is(err, persistence.ErrDuplicate) {
		return application.Connection{}, fmt.Errorf("%w: connection %q", ErrDuplicate, cmd.Key)
	}
	if err != nil {
		return application.Connection{}, err
	}
	return conn, nil
}
