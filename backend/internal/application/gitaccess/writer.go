package gitaccess

import (
	"context"
	"encoding/json"
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
