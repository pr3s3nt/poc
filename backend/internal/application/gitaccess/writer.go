package gitaccess

import (
	"context"
	"encoding/json"
	"orchestrator/internal/domain/application"
	"orchestrator/internal/ports/credentials"
	"orchestrator/internal/ports/persistence"
	"regexp"
	"strings"
)

// PlatformWriterReference selects live organization settings, never secret bytes.
const PlatformWriterReference = "platform-git"

// Writer resolves scoped platform credentials into the private writer format.
func Writer(ctx context.Context, repo persistence.GitSettingsRepository, creds credentials.Store, org, destination string) ([]byte, string, error) {
	if repo == nil || creds == nil || !creds.Durable() {
		return nil, "", ErrUnavailable
	}
	v, err := repo.GetGitSettings(ctx, org)
	if err != nil || !v.Authenticated || !Matches(v.URL, destination) {
		return nil, "", ErrInvalid
	}
	s := &Service{repo: repo, credentials: creds}
	access, err := s.credential(ctx, org, v)
	if err != nil {
		return nil, "", err
	}
	raw, err := json.Marshal(map[string]string{"username": access.Username, "password": access.Token})
	return raw, access.CAPEM, err
}

var repositoryOwnerShape = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}$`)

// ConnectionWriterPrefix marks a binding writer credential that lives in a
// reusable Git Connection (ADR-034): "connection:<connectionKey>". Credential
// bytes are resolved from the connection's scoped store on every use, so
// rotation applies to subsequent operations without editing bindings.
const ConnectionWriterPrefix = "connection:"

// ConnectionReader loads an Organization-scoped Connection by key.
type ConnectionReader interface {
	GetConnection(ctx context.Context, org, key string) (application.Connection, error)
}

// ConnectionCredential resolves the writer bytes of a READY Git Connection.
func ConnectionCredential(ctx context.Context, conns ConnectionReader, creds credentials.Store, org, ref string, destinations ...string) ([]byte, error) {
	if conns == nil || creds == nil || !creds.Durable() {
		return nil, ErrUnavailable
	}
	key := strings.TrimPrefix(ref, ConnectionWriterPrefix)
	if key == "" || key == ref {
		return nil, ErrInvalid
	}
	conn, err := conns.GetConnection(ctx, org, key)
	if err != nil || conn.Kind != application.ConnectionGit || conn.Status != application.ConnectionReady || conn.SecretRef == "" {
		return nil, ErrInvalid
	}
	scope, _ := conn.Config["url"].(string)
	for _, destination := range destinations {
		if !Matches(scope, destination) {
			return nil, ErrInvalid
		}
	}
	raw, err := creds.Get(ctx, org, conn.Key, conn.SecretRef)
	if err != nil {
		return nil, ErrUnavailable
	}
	return raw, nil
}

// ConnectionRepositoryOwner derives the eligible repository owner scope of a
// Git Connection, or "" when the stored URL is not an account/group scope.
func ConnectionRepositoryOwner(conn application.Connection) string {
	url, _ := conn.Config["url"].(string)
	return RepositoryOwner(url)
}

// RepositoryOwner accepts only an explicit account/group scope, never a host or
// repository URL. Authentication username is not an ownership decision.
func RepositoryOwner(raw string) string {
	u, err := ScopeURL(raw)
	if err != nil {
		return ""
	}
	owner := strings.Trim(u.Path, "/")
	if !repositoryOwnerShape.MatchString(owner) {
		return ""
	}
	return owner
}
