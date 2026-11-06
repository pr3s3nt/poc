// Package fleet implements the version/schema-gated Fleet workload delivery adapter.
package fleet

import (
	"crypto/x509"
	"errors"
	"net/url"
	domain "orchestrator/internal/domain/delivery"
	"path"
	"regexp"
	"strings"
)

const Version = "v0.15.2"

var ErrConfig = errors.New("fleet: invalid binding configuration")
var label = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
var applicationRepositoryPattern = regexp.MustCompile(`^idp-[a-z0-9-]+-deploy-[a-z2-7]{6}$`)
var gitRef = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$`)

// Config contains nonsecret controller and publication topology only. The target
// is independently resolved from resource Definitions, never selected by Fleet.
type Config struct {
	ApplicationID, EnvironmentKey                                                string
	ApplicationRepoName                                                          string
	ControllerKind, ControllerVersion, AgentKind, AgentName, TargetKubeSystemUID string
	NativeServiceAccount                                                         bool
	ManagedReadSecret                                                            bool
	GitRepoMode                                                                  string
	ExistingGitRepo                                                              string
	ExistingGitRepoUID                                                           string
	Workspace                                                                    string
	ControllerNamespace                                                          string
	AgentNamespace                                                               string
	ControllerName                                                               string
	TargetCluster                                                                string
	TargetClusterUID                                                             string
	TargetAccountKey                                                             string
	RepositoryURL                                                                string
	Branch                                                                       string
	Root                                                                         string
	ReadSecret                                                                   string
	ServiceAccount                                                               string
	CABundle                                                                     string
	ReleaseNamespace                                                             string
}

func ParseConfig(raw map[string]string) (Config, error) {
	keys := map[string]bool{"applicationID": true, "environmentKey": true, "controllerKind": true, "controllerVersion": true, "agentKind": true, "agentName": true, "targetKubeSystemUID": true, "serviceAccountMode": true, "gitRepoMode": true, "existingGitRepo": true, "existingGitRepoUID": true, "agentNamespace": true, "workspace": true, "controllerNamespace": true, "controllerName": true, "targetCluster": true, "targetClusterUID": true, "targetAccountKey": true, "gitConnectionKey": true, "readSecretMode": true, "repositoryURL": true, "branch": true, "root": true, "readSecret": true, "serviceAccount": true, "caBundle": true, "releaseNamespace": true, "repositoryOwner": true, "repositoryAPI": true, "repositoryHost": true, "repositoryPrefix": true}
	for k := range raw {
		if !keys[k] && k != "applicationRepoName" {
			return Config{}, ErrConfig
		}
	}
	c := Config{ApplicationID: raw["applicationID"], EnvironmentKey: raw["environmentKey"], ControllerKind: raw["controllerKind"], ControllerVersion: raw["controllerVersion"], AgentKind: raw["agentKind"], AgentName: raw["agentName"], TargetKubeSystemUID: raw["targetKubeSystemUID"], NativeServiceAccount: raw["serviceAccountMode"] == "NATIVE", GitRepoMode: raw["gitRepoMode"], ExistingGitRepo: raw["existingGitRepo"], ExistingGitRepoUID: raw["existingGitRepoUID"], Workspace: raw["workspace"], ControllerNamespace: raw["controllerNamespace"], AgentNamespace: raw["agentNamespace"], ControllerName: raw["controllerName"], TargetCluster: raw["targetCluster"], TargetClusterUID: raw["targetClusterUID"], TargetAccountKey: raw["targetAccountKey"], RepositoryURL: raw["repositoryURL"], Branch: raw["branch"], Root: raw["root"], ReadSecret: raw["readSecret"], ServiceAccount: raw["serviceAccount"], CABundle: raw["caBundle"], ReleaseNamespace: raw["releaseNamespace"]}
	if (c.ApplicationID == "") != (c.EnvironmentKey == "") || (c.ApplicationID != "" && (!label.MatchString(c.ApplicationID) || (c.EnvironmentKey != "staging" && c.EnvironmentKey != "production") || c.GitRepoMode != "MANAGED")) {
		return Config{}, ErrConfig
	}
	c.ApplicationRepoName = raw["applicationRepoName"]
	if c.ApplicationRepoName != "" && (c.ApplicationID == "" || !label.MatchString(c.ApplicationRepoName) || !applicationRepositoryPattern.MatchString(c.ApplicationRepoName)) {
		return Config{}, ErrConfig
	}
	if c.ApplicationID != "" && (c.Root == "metadata" || strings.HasPrefix(c.Root, "metadata/")) {
		return Config{}, ErrConfig
	}
	if c.ControllerKind == "" {
		c.ControllerKind = "Deployment"
	}
	if (c.ControllerKind != "Deployment" && c.ControllerKind != "StatefulSet") || (c.AgentKind != "" && c.AgentKind != "Deployment" && c.AgentKind != "StatefulSet") || (c.AgentName != "" && !label.MatchString(c.AgentName)) || (c.ControllerVersion != "" && !fleetVersion.MatchString(c.ControllerVersion)) {
		return Config{}, ErrConfig
	}
	if raw["serviceAccountMode"] != "" && raw["serviceAccountMode"] != "NATIVE" {
		return Config{}, ErrConfig
	}
	if c.NativeServiceAccount && c.ServiceAccount != "" {
		return Config{}, ErrConfig
	}
	if c.TargetKubeSystemUID != "" && (len(c.TargetKubeSystemUID) > 128 || strings.ContainsAny(c.TargetKubeSystemUID, "/\x00\r\n")) {
		return Config{}, ErrConfig
	}
	if raw["readSecretMode"] != "" && raw["readSecretMode"] != "MANAGED" {
		return Config{}, ErrConfig
	}
	c.ManagedReadSecret = raw["readSecretMode"] == "MANAGED"
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
	for _, s := range []string{c.Workspace, c.ControllerNamespace, c.AgentNamespace, c.ControllerName, c.TargetCluster, c.TargetAccountKey, c.ReleaseNamespace} {
		if !label.MatchString(s) {
			return Config{}, ErrConfig
		}
	}
	if !c.NativeServiceAccount && !label.MatchString(c.ServiceAccount) {
		return Config{}, ErrConfig
	}
	if c.TargetClusterUID == "" || len(c.TargetClusterUID) > 128 || strings.ContainsAny(c.TargetClusterUID, "/\x00\r\n") {
		return Config{}, ErrConfig
	}
	if c.ReadSecret != "" && !label.MatchString(c.ReadSecret) {
		return Config{}, ErrConfig
	}
	if c.CABundle != "" {
		pool := x509.NewCertPool()
		if len(c.CABundle) > 65536 || !pool.AppendCertsFromPEM([]byte(c.CABundle)) {
			return Config{}, ErrConfig
		}
	}
	if raw["repositoryOwner"] != "" && c.RepositoryURL == "" {
		policy := domain.RepositoryPolicy{Enabled: true, Host: "github", APIBase: raw["repositoryAPI"], CloneBase: raw["repositoryHost"], Owner: raw["repositoryOwner"], Prefix: raw["repositoryPrefix"], Root: c.Root}
		if policy.Validate() != nil || c.GitRepoMode != "MANAGED" || c.ReadSecret == "" {
			return Config{}, ErrConfig
		}
		return c, nil
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

	return c, nil
}
