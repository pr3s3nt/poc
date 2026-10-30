package persistence

import (
	"context"
	"time"
)

// GitSettings holds public metadata and an opaque credential reference only.
// Transport credentials are stored through credentials.Store.
type GitSettings struct {
	OrganizationKey string
	URL             string
	Username        string
	Authenticated   bool
	CAPEM           string
	CredentialRef   string
	Version         int64
	UpdatedAt       time.Time
	UpdatedBy       string
}

type GitSettingsRepository interface {
	GetGitSettings(context.Context, string) (GitSettings, error)
	SaveGitSettings(context.Context, int64, GitSettings) error
}
