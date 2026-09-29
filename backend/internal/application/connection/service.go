// Package connection implements UC-04 local/kind Kubernetes connection registration.
package connection

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"orchestrator/internal/domain/application"
	"orchestrator/internal/ports/persistence"
)

var ErrInvalid = errors.New("connection: invalid registration")
var ErrDuplicate = errors.New("connection: duplicate id")
var ErrVerification = errors.New("connection: verification failed")

var connectionKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,40}$`)

type KubernetesVerification struct {
	Endpoint string
	Version  string
}

// KubernetesVerifier checks the host's named context without creating objects.
type KubernetesVerifier interface {
	Verify(ctx context.Context, kubeContext string) (KubernetesVerification, error)
}

type Service struct {
	store    persistence.Store
	verifier KubernetesVerifier
}

func NewService(store persistence.Store, verifier KubernetesVerifier) *Service {
	return &Service{store: store, verifier: verifier}
}

type RegisterKubernetesCommand struct {
	Key         string `json:"key"`
	ClusterID   string `json:"clusterId"`
	KubeContext string `json:"kubeContext"`
}

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
		return application.Connection{}, fmt.Errorf("%w: %v", ErrVerification, err)
	}
	if verified.Endpoint == "" {
		return application.Connection{}, fmt.Errorf("%w: cluster endpoint is empty", ErrVerification)
	}
	conn := application.Connection{
		Key: cmd.Key, OrganizationKey: org, Kind: application.ConnectionKubernetes,
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
		return s.store.SaveConnection(ctx, conn)
	})
	if err != nil {
		return application.Connection{}, err
	}
	return conn, nil
}
