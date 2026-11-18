package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"gopkg.in/yaml.v3"
	"io"
	"orchestrator/internal/domain/tfmodule"
	access "orchestrator/internal/ports/gitaccess"
	"path"
	"sort"
	"strings"
	"unicode/utf8"
)

// DiscoverSetup reuses the hardened Git transport. Files are never checked out
// or executed; extension alone cannot identify a product document.
func (g *GitCLI) DiscoverSetup(ctx context.Context, org, url, ref, dir string) (any, error) {
	if dir == "" {
		dir = "."
	}
	if dir != "." && checkPath(dir) != nil {
		return nil, ErrFetch
	}
	ctx = access.WithOrganization(ctx, org)
	source := GitSource{URL: url}
	commit := ref
	if !tfmodule.FullCommit(commit) {
		var err error
		commit, err = g.Resolve(ctx, source, ref)
		if err != nil {
			return nil, err
		}
	}
	snapshot, err := g.Open(ctx, source, commit)
	if err != nil {
		return nil, err
	}
	defer snapshot.Close()
	tree, err := snapshot.Tree(dir)
	if err != nil {
		return nil, err
	}
	type document struct {
		File    string         `json:"file"`
		Kind    string         `json:"kind"`
		Content map[string]any `json:"content"`
		Problem string         `json:"problem,omitempty"`
	}
	out := []document{}
	skipped := 0
	names := make([]string, 0, len(tree))
	for name := range tree {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		ext := strings.ToLower(path.Ext(name))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}
		raw := tree[name]
		if len(raw) > 1<<20 || !utf8.Valid(raw) {
			return nil, ErrFetch
		}
		var node yaml.Node
		decoder := yaml.NewDecoder(bytes.NewReader(raw))
		parseErr := decoder.Decode(&node)
		var value map[string]any
		if parseErr == nil {
			parseErr = node.Decode(&value)
		}
		if parseErr != nil || decoder.Decode(new(any)) != io.EOF {
			out = append(out, document{File: path.Join(dir, name), Kind: "invalid", Problem: "INVALID_DOCUMENT"})
			continue
		}
		if _, err := json.Marshal(value); err != nil {
			return nil, ErrFetch
		}
		if _, ok := value["key"].(string); !ok {
			skipped++
			continue
		}
		kind := "resource-types"
		if _, ok := value["resourceType"]; ok {
			kind = "resource-definitions"
		} else {
			// An empty Type contract may contain only its key. Unknown fields
			// are reported as invalid instead of sent into a strict batch API.
			known := true
			for field := range value {
				if field != "key" && field != "inputs" && field != "outputs" {
					known = false
				}
			}
			if !known {
				skipped++
				continue
			}
		}
		out = append(out, document{File: path.Join(dir, name), Kind: kind, Content: value})
		if len(out) > 100 {
			return nil, ErrFetch
		}
	}
	return map[string]any{"url": url, "revision": commit, "path": dir, "documents": out, "skipped": skipped}, nil
}
