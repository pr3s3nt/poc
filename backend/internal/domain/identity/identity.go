// Package identity holds internal-account and opaque-session aggregates.
package identity

import "time"

type Role string

const (
	RoleAdmin            Role = "ADMIN"
	RolePlatformEngineer Role = "PLATFORM_ENGINEER"
	RoleDeveloper        Role = "DEVELOPER"
)

type AccountStatus string

const (
	AccountActive   AccountStatus = "ACTIVE"
	AccountDisabled AccountStatus = "DISABLED"
)

type UserAccount struct {
	ID              string        `json:"id"`
	OrganizationKey string        `json:"organizationKey"`
	Username        string        `json:"username"`
	PasswordHash    string        `json:"passwordHash"`
	Role            Role          `json:"role"`
	Status          AccountStatus `json:"status"`
}

type Session struct {
	ID            string     `json:"id"`
	UserAccountID string     `json:"userAccountId"`
	TokenHash     string     `json:"tokenHash"`
	ExpiresAt     time.Time  `json:"expiresAt"`
	RevokedAt     *time.Time `json:"revokedAt,omitempty"`
}
