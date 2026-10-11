package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"issueflow/internal/gh"
)

// Publisher uploads staged evidence as GitHub Release assets on a prerelease
// tag. Asset names embed the content hash, nothing is overwritten and nothing
// is deleted.
type Publisher struct {
	GH     gh.GitHub
	Prefix string
}

func (p Publisher) Tag(issue int, run string) string {
	return fmt.Sprintf("%s-issue-%d-%s", p.Prefix, issue, run)
}

func copyHash(src, dst string) (string, int64, error) {
	if src == dst {
		f, err := os.Open(src)
		if err != nil {
			return "", 0, err
		}
		defer f.Close()
		h := sha256.New()
		n, err := io.Copy(h, f)
		return hex.EncodeToString(h.Sum(nil)), n, err
	}
	in, err := os.Open(src)
	if err != nil {
		return "", 0, err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", 0, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return hex.EncodeToString(h.Sum(nil)), n, err
}

// Publish is idempotent. Every staged file is re-hashed immediately before it
// is uploaded; an existing asset must match by content-addressed name and
// size (and digest when reported) or the publish fails without overwriting.
func (p Publisher) Publish(ctx context.Context, m *Manifest, workDir string) ([]string, error) {
	tag := p.Tag(m.Issue, m.RunID)
	rel, err := p.GH.EnsureRelease(ctx, tag, fmt.Sprintf("Evidence #%d (%s)", m.Issue, m.RunID), "Evidence for task run "+m.RunID+". Manifest sha256 "+m.Hash())
	if err != nil {
		return nil, fmt.Errorf("release: %w", err)
	}
	type up struct {
		path, name, sha string
		size            int64
	}
	var ups []up
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return nil, err
	}
	mb := m.Bytes()
	msum := sha256.Sum256(mb)
	mname := "manifest-" + hex.EncodeToString(msum[:])[:16] + ".json"
	mpath := filepath.Join(workDir, mname)
	if err := os.WriteFile(mpath, mb, 0o600); err != nil {
		return nil, err
	}
	ups = append(ups, up{mpath, mname, hex.EncodeToString(msum[:]), int64(len(mb))})
	for _, f := range m.Files {
		ups = append(ups, up{f.Staged, f.AssetName, f.SHA256, f.Size})
	}
	existing, err := p.GH.ListAssets(ctx, tag)
	if err != nil {
		return nil, err
	}
	have := map[string]gh.Asset{}
	for _, a := range existing {
		have[a.Name] = a
	}
	for _, u := range ups {
		// Copy to the upload name while hashing, so the hash we verify is the
		// hash of the exact bytes handed to the uploader.
		dst := filepath.Join(workDir, u.name)
		sha, size, err := copyHash(u.path, dst)
		if err != nil {
			return nil, err
		}
		if sha != u.sha || size != u.size {
			return nil, fmt.Errorf("staged file %s changed after validation", u.name)
		}
		u.path = dst
		if a, ok := have[u.name]; ok {
			if a.Size != u.size || (a.Digest != "" && a.Digest != "sha256:"+u.sha) {
				return nil, fmt.Errorf("existing asset %s does not match its content hash; refusing to overwrite", u.name)
			}
			continue
		}
		if err := p.GH.UploadAsset(ctx, tag, u.path); err != nil {
			return nil, fmt.Errorf("upload %s: %w", u.name, err)
		}
	}
	final, err := p.GH.ListAssets(ctx, tag)
	if err != nil {
		return nil, err
	}
	got := map[string]gh.Asset{}
	for _, a := range final {
		got[a.Name] = a
	}
	links := []string{rel.URL}
	for _, u := range ups {
		a, ok := got[u.name]
		if !ok || a.Size != u.size || (a.Digest != "" && a.Digest != "sha256:"+u.sha) {
			return nil, fmt.Errorf("asset %s missing or mismatched after upload", u.name)
		}
		links = append(links, fmt.Sprintf("%s: %s", strings.TrimPrefix(u.name, ""), a.URL))
	}
	return links, nil
}
