// Package fleet implements the Fleet v0.15.2 workload delivery adapter.
package fleet

import (
	"crypto/x509"
	"errors"
	"net/url"
	"path"
	"regexp"
	"strings"
)

const Version = "v0.15.2"

var ErrConfig = errors.New("fleet: invalid binding configuration")
var label = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
var gitRef = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$`)

// Config contains nonsecret controller and publication topology only. The target
// is independently resolved from resource Definitions, never selected by Fleet.
type Config struct {
	GitRepoMode         string
	ExistingGitRepo     string
	ExistingGitRepoUID  string
	Workspace           string
	ControllerNamespace string
	AgentNamespace      string
	ControllerName      string
	TargetCluster       string
	TargetClusterUID    string
	TargetAccountKey    string
	RepositoryURL       string
	Branch              string
	Root                string
	ReadSecret          string
	ServiceAccount      string
	CABundle            string
	ReleaseNamespace    string
}

func ParseConfig(raw map[string]string) (Config, error) {
	keys := map[string]bool{"gitRepoMode": true, "existingGitRepo": true, "existingGitRepoUID": true, "agentNamespace": true, "workspace": true, "controllerNamespace": true, "controllerName": true, "targetCluster": true, "targetClusterUID": true, "targetAccountKey": true, "repositoryURL": true, "branch": true, "root": true, "readSecret": true, "serviceAccount": true, "caBundle": true, "releaseNamespace": true}
	for k := range raw {
		if !keys[k] {
			return Config{}, ErrConfig
		}
	}
	c := Config{GitRepoMode: raw["gitRepoMode"], ExistingGitRepo: raw["existingGitRepo"], ExistingGitRepoUID: raw["existingGitRepoUID"], Workspace: raw["workspace"], ControllerNamespace: raw["controllerNamespace"], AgentNamespace: raw["agentNamespace"], ControllerName: raw["controllerName"], TargetCluster: raw["targetCluster"], TargetClusterUID: raw["targetClusterUID"], TargetAccountKey: raw["targetAccountKey"], RepositoryURL: raw["repositoryURL"], Branch: raw["branch"], Root: raw["root"], ReadSecret: raw["readSecret"], ServiceAccount: raw["serviceAccount"], CABundle: raw["caBundle"], ReleaseNamespace: raw["releaseNamespace"]}
	if c.GitRepoMode == "" {
		c.GitRepoMode = "MANAGED"
	}
	if c.GitRepoMode != "MANAGED" && c.GitRepoMode != "EXISTING" {
		return Config{}, ErrConfig
	}
	if c.GitRepoMode == "EXISTING" && (!label.MatchString(c.ExistingGitRepo) || c.ExistingGitRepoUID == "") {
		return Config{}, ErrConfig
	}
	if c.GitRepoMode == "MANAGED" && (c.ExistingGitRepo != "" || c.ExistingGitRepoUID != "") {
		return Config{}, ErrConfig
	}
	for _, s := range []string{c.Workspace, c.ControllerNamespace, c.AgentNamespace, c.ControllerName, c.TargetCluster, c.TargetAccountKey, c.ServiceAccount, c.ReleaseNamespace} {
		if !label.MatchString(s) {
			return Config{}, ErrConfig
		}
	}
	if c.TargetClusterUID == "" || len(c.TargetClusterUID) > 128 || strings.ContainsAny(c.TargetClusterUID, "/\x00\r\n") {
		return Config{}, ErrConfig
	}
	if c.ReadSecret != "" && !label.MatchString(c.ReadSecret) {
		return Config{}, ErrConfig
	}
	u, err := url.Parse(c.RepositoryURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return Config{}, ErrConfig
	}
	if !gitRef.MatchString(c.Branch) || strings.Contains(c.Branch, "..") || strings.Contains(c.Branch, "//") || strings.HasSuffix(c.Branch, "/") || strings.HasSuffix(c.Branch, ".lock") {
		return Config{}, ErrConfig
	}
	if c.Root == "" || strings.HasPrefix(c.Root, "/") || path.Clean(c.Root) != c.Root || strings.Contains(c.Root, "..") || strings.ContainsAny(c.Root, "\\\x00\r\n") {
		return Config{}, ErrConfig
	}
	if c.CABundle != "" {
		pool := x509.NewCertPool()
		if len(c.CABundle) > 65536 || !pool.AppendCertsFromPEM([]byte(c.CABundle)) {
			return Config{}, ErrConfig
		}
	}
	return c, nil
}
