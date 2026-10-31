package registry

import (
	"context"
	"errors"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"golang.org/x/mod/semver"
	"orchestrator/internal/domain/catalogrelease"
	"orchestrator/internal/ports/catalogsource"
	"orchestrator/internal/ports/gitaccess"
)

const maxCatalogTags = 256

// Discover captures the branch and tag observations before proving ancestry.
// No shallow fetch is used: a shallow boundary cannot prove non-ancestry.
func (g *GitCLI) Discover(ctx context.Context, source catalogrelease.Source) (catalogsource.Observation, error) {
	if source.Validate() != nil {
		return catalogsource.Observation{}, catalogrelease.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(gitaccess.WithOrganization(ctx, source.Organization), 2*time.Minute)
	defer cancel()
	dir, err := os.MkdirTemp(g.TempDir, "catalog-")
	if err != nil {
		return catalogsource.Observation{}, catalogrelease.ErrUnavailable
	}
	snap, err := g.snapshot(ctx, GitSource{URL: source.Repository}, dir, "")
	if err != nil {
		_ = os.RemoveAll(dir)
		return catalogsource.Observation{}, catalogrelease.ErrUnavailable
	}
	defer snap.Close()
	snap.outputLimit = 1 << 20
	observation, err := snap.discoverCatalog(source)
	if err != nil {
		return catalogsource.Observation{}, catalogrelease.ErrUnavailable
	}
	return observation, nil
}
func (s *gitSnapshot) discoverCatalog(source catalogrelease.Source) (catalogsource.Observation, error) {
	out, err := s.git("ls-remote", "--", source.Repository, "refs/heads/"+source.Branch, "refs/tags/"+source.TagPattern)
	if err != nil {
		return catalogsource.Observation{}, err
	}
	refs := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 || !catalogrelease.ValidCommit(f[0]) {
			return catalogsource.Observation{}, ErrFetch
		}
		refs[f[1]] = f[0]
	}
	tip := refs["refs/heads/"+source.Branch]
	if !catalogrelease.ValidCommit(tip) {
		return catalogsource.Observation{}, ErrFetch
	}
	tags := []catalogsource.Tag{}
	for ref, commit := range refs {
		if !strings.HasPrefix(ref, "refs/tags/") || strings.HasSuffix(ref, "^{}") {
			continue
		}
		tag := strings.TrimPrefix(ref, "refs/tags/")
		matched, err := path.Match(source.TagPattern, tag)
		if err != nil || !matched || !validGitReference(tag) {
			continue
		}
		// The initial contract accepts SemVer releases, with optional prereleases.
		version := tag
		if !strings.HasPrefix(version, "v") {
			version = "v" + version
		}
		if !semver.IsValid(version) || (!source.IncludePrereleases && semver.Prerelease(version) != "") {
			continue
		}
		if peeled := refs[ref+"^{}"]; peeled != "" {
			commit = peeled
		}
		tags = append(tags, catalogsource.Tag{Name: tag, Commit: commit})
	}
	if len(tags) > maxCatalogTags {
		return catalogsource.Observation{}, ErrFetch
	}
	sort.Slice(tags, func(i, j int) bool {
		a, b := tags[i].Name, tags[j].Name
		if !strings.HasPrefix(a, "v") {
			a = "v" + a
		}
		if !strings.HasPrefix(b, "v") {
			b = "v" + b
		}
		if cmp := semver.Compare(a, b); cmp != 0 {
			return cmp > 0
		}
		return tags[i].Name < tags[j].Name
	})
	if _, err = s.git("init", "-q", "--bare", "--object-format="+objectFormat(tip), "."); err != nil {
		return catalogsource.Observation{}, err
	}
	if _, err = s.git("fetch", "-q", "--no-tags", "--no-recurse-submodules", "--no-write-fetch-head", source.Repository, tip+":refs/catalog-branch"); err != nil {
		return catalogsource.Observation{}, err
	}
	for i := range tags {
		t := &tags[i]
		if _, err = s.git("fetch", "-q", "--no-tags", "--no-recurse-submodules", "--no-write-fetch-head", source.Repository, t.Commit); err != nil {
			return catalogsource.Observation{}, err
		}
		// Verify it is a commit, including lightweight tags that point to blobs.
		resolved, err := s.git("rev-parse", "--verify", "--end-of-options", t.Commit+"^{commit}")
		if err != nil || strings.TrimSpace(string(resolved)) != t.Commit {
			return catalogsource.Observation{}, ErrFetch
		}
		_, err = s.git("merge-base", "--is-ancestor", t.Commit, tip)
		if err == nil {
			t.InBranch = true
			continue
		}
		// Exit 1 proves not ancestor. All other failures leave ancestry unknown.
		var status interface{ ExitCode() int }
		if !errors.As(err, &status) || status.ExitCode() != 1 {
			return catalogsource.Observation{}, ErrFetch
		}
	}
	return catalogsource.Observation{BranchTip: tip, Tags: tags, ObservedAt: time.Now().UTC()}, nil
}

var _ catalogsource.Discovery = (*GitCLI)(nil)
