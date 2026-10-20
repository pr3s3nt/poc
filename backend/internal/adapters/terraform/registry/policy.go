package registry

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"orchestrator/internal/domain/tfmodule"
)

// Policy is legacy reviewed platform source configuration loaded at startup.
// Those pins retain their source/module restrictions. Direct HTTPS inspection
// uses its own verified pins without requiring entries here (UC-03 GIT-01).
type Policy struct {
	Sources []GitSource `json:"sources"`
}

// GitSource is one allowlisted HTTPS Git repository. CAFile optionally
// names a PEM bundle trusted for this host (for example a private CA); the
// URL never carries credentials.
type GitSource struct {
	ID      string      `json:"id"`
	Kind    string      `json:"kind"`
	URL     string      `json:"url"`
	CAFile  string      `json:"caFile,omitempty"`
	Modules []GitModule `json:"modules"`
}

// GitModule allows one module directory of a Git source and pairs it with
// the Resource Types it may implement.
type GitModule struct {
	ID            string   `json:"id"`
	Subdirectory  string   `json:"subdirectory"`
	ResourceTypes []string `json:"resourceTypes"`
}

// Clone returns a deep copy, so the allowlist the registry validated cannot
// be changed through the caller's value.
func (p Policy) Clone() Policy {
	out := Policy{Sources: make([]GitSource, len(p.Sources))}
	for i, s := range p.Sources {
		modules := make([]GitModule, len(s.Modules))
		for j, m := range s.Modules {
			m.ResourceTypes = append([]string(nil), m.ResourceTypes...)
			modules[j] = m
		}
		s.Modules = modules
		out.Sources[i] = s
	}
	return out
}

// LoadPolicy reads and validates an Admin policy file. An empty path is the
// empty policy (packaged release modules only).
func LoadPolicy(path string, release Release) (Policy, error) {
	if path == "" {
		return Policy{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, errors.New("registry: read the module source policy file")
	}
	var p Policy
	if err := strictDecode(data, &p); err != nil {
		return Policy{}, fmt.Errorf("registry: module source policy: %w", err)
	}
	if err := p.Validate(release); err != nil {
		return Policy{}, err
	}
	return p, nil
}

// Validate checks the policy. Errors name the offending source position, not
// URLs or paths.
func (p Policy) Validate(release Release) error {
	ids := map[string]bool{release.Packaged.SourceID: true}
	for i, source := range p.Sources {
		fail := func(reason string) error {
			return fmt.Errorf("registry: module source policy entry %d: %s", i, reason)
		}
		if !tfmodule.ValidIdentifier(source.ID) || ids[source.ID] {
			return fail("source IDs must be unique lowercase identifiers distinct from the packaged source")
		}
		ids[source.ID] = true
		if source.Kind != tfmodule.SourceKindGit {
			// Packaged modules come only from the platform release.
			return fail("only git sources can be allowlisted")
		}
		if err := validateGitURL(source.URL); err != nil {
			return fail(err.Error())
		}
		if source.CAFile != "" && !strings.HasPrefix(source.CAFile, "/") {
			return fail("caFile must be an absolute path")
		}
		modules := map[string]bool{}
		for _, m := range source.Modules {
			if !tfmodule.ValidIdentifier(m.ID) || modules[m.ID] || !tfmodule.ValidSubdirectory(m.Subdirectory) || len(m.ResourceTypes) == 0 {
				return fail("modules need unique IDs, clean subdirectories and resource types")
			}
			for _, typ := range m.ResourceTypes {
				if typ == "" {
					return fail("resource types must be named")
				}
			}
			modules[m.ID] = true
		}
		if len(modules) == 0 {
			return fail("a source must allow at least one module")
		}
	}
	return nil
}

// validateGitURL accepts only an https:// URL with a host and path and no
// credentials, query, fragment, whitespace or control characters.
func validateGitURL(raw string) error {
	for _, r := range raw {
		if r <= 0x20 || r == 0x7f {
			return errors.New("the URL must not contain whitespace or control characters")
		}
	}
	u, err := url.Parse(raw)
	switch {
	case err != nil:
		return errors.New("the URL is malformed")
	case u.Scheme != "https":
		return errors.New("only https Git URLs are allowed")
	case u.User != nil:
		return errors.New("credentials in the URL are not allowed")
	case u.Host == "" || u.Hostname() == "":
		return errors.New("the URL needs a host")
	case u.RawQuery != "" || u.Fragment != "" || u.Opaque != "":
		return errors.New("the URL must not have a query or fragment")
	case u.Path == "" || u.Path == "/":
		return errors.New("the URL needs a repository path")
	}
	return nil
}

func (p Policy) source(id string) (GitSource, bool) {
	for _, s := range p.Sources {
		if s.ID == id {
			return s, true
		}
	}
	return GitSource{}, false
}

func (s GitSource) module(id string) (GitModule, bool) {
	for _, m := range s.Modules {
		if m.ID == id {
			return m, true
		}
	}
	return GitModule{}, false
}
