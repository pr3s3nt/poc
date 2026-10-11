package gitx

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var scpLike = regexp.MustCompile(`^(?:[^@/]+@)?([^:/]+):([^/].*)$`)

// ParseRemote extracts "owner/name" from an origin URL without ever returning
// the URL itself, so credentials embedded in it cannot leak. ok is false for
// anything that is not recognisably host/owner/name (for example a local path).
func ParseRemote(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	var path string
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return "", false
		}
		path = u.Path
	} else if m := scpLike.FindStringSubmatch(raw); m != nil && !strings.HasPrefix(raw, "/") {
		path = m[2]
	} else {
		return "", false
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	return parts[0] + "/" + parts[1], true
}

// OriginRepo returns the owner/name that the checkout's origin points to.
// Errors never contain the remote URL.
func (g Git) OriginRepo(ctx context.Context, dir string) (string, error) {
	res, err := g.R.Run(ctx, dir, []string{"GIT_TERMINAL_PROMPT=0", "LC_ALL=C"}, "git", "remote", "get-url", "origin")
	if err != nil || res.Exit != 0 {
		return "", fmt.Errorf("cannot read the origin remote of the checkout")
	}
	id, ok := ParseRemote(res.Stdout)
	if !ok {
		return "", fmt.Errorf("the origin remote is not a recognisable owner/name repository")
	}
	return id, nil
}
