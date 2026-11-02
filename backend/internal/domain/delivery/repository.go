package delivery

import (
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
)

var repositorySegment = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)

type RepositoryPolicy struct {
	Enabled                                       bool
	Host, APIBase, CloneBase, Owner, Prefix, Root string
}

func (p RepositoryPolicy) Validate() error {
	if !p.Enabled || p.Host != "github" || !repositorySegment.MatchString(p.Owner) || p.Prefix == "" || len(p.Prefix) > 40 || !repositorySegment.MatchString(p.Prefix+"app") {
		return ErrInvalid
	}
	for _, raw := range []string{p.APIBase, p.CloneBase} {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(raw, "\x00\r\n") {
			return ErrInvalid
		}
	}
	if p.Root == "" || path.IsAbs(p.Root) || path.Clean(p.Root) != p.Root || strings.Contains(p.Root, "..") || strings.ContainsAny(p.Root, "\\\x00\r\n") {
		return ErrInvalid
	}
	return nil
}

type PlatformDefault struct {
	OrganizationID, BindingID string
	BindingVersion, Revision  int64
}

func (d PlatformDefault) Validate() error {
	if d.OrganizationID == "" || d.BindingID == "" || d.BindingVersion < 1 || d.Revision < 1 {
		return ErrInvalid
	}
	return nil
}

type ApplicationRepository struct {
	ID, OrganizationID, ApplicationID, BindingID     string
	BindingVersion                                   int64
	Name, Marker, Status, RemoteID, CloneURL, Branch string
	Revision                                         int64
	CreatedAt                                        time.Time
}

func (r ApplicationRepository) Validate() error {
	if r.ID == "" || r.OrganizationID == "" || r.ApplicationID == "" || r.BindingID == "" || r.BindingVersion < 1 || !repositorySegment.MatchString(r.Name) || r.Marker == "" || r.Revision < 1 || r.CreatedAt.IsZero() {
		return ErrInvalid
	}
	if r.Status == "PENDING" {
		if r.RemoteID != "" || r.CloneURL != "" || r.Branch != "" {
			return ErrInvalid
		}
		return nil
	}
	if r.Status != "READY" || r.RemoteID == "" || r.Branch == "" {
		return ErrInvalid
	}
	u, err := url.Parse(r.CloneURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return ErrInvalid
	}
	return nil
}
func (r ApplicationRepository) SameAllocation(other ApplicationRepository) bool {
	return r.ID == other.ID && r.OrganizationID == other.OrganizationID && r.ApplicationID == other.ApplicationID && r.BindingID == other.BindingID && r.BindingVersion == other.BindingVersion && r.Name == other.Name && r.Marker == other.Marker && r.CreatedAt.Equal(other.CreatedAt)
}
