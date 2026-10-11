package gh

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Write records one mutating call for assertions.
type Write struct{ Kind, Detail string }

// Fake is an in-memory GitHub with fault injection. Never touches the network.
type Fake struct {
	mu       sync.Mutex
	Issues   map[int]*Issue
	Actors   map[string]string // "n/label" -> login
	Cmts     map[int][]Comment
	Releases map[string]*Release
	Assets   map[string][]Asset
	Writes   []Write
	nextID   int64
	Now      func() time.Time
	// FailOn makes the named operation fail while the value is >0 (decrements).
	FailOn map[string]int
	// AfterWrite runs after a mutating op took effect but before returning
	// (simulates crash between side effect and state save).
	AfterWrite func(kind string) error
}

func NewFake(now func() time.Time) *Fake {
	return &Fake{Issues: map[int]*Issue{}, Actors: map[string]string{}, Cmts: map[int][]Comment{}, Releases: map[string]*Release{},
		Assets: map[string][]Asset{}, FailOn: map[string]int{}, Now: now, nextID: 1000}
}

func (f *Fake) fail(op string) error {
	if f.FailOn[op] > 0 {
		f.FailOn[op]--
		return fmt.Errorf("injected %s failure", op)
	}
	return nil
}

func (f *Fake) after(kind string) error {
	if f.AfterWrite != nil {
		return f.AfterWrite(kind)
	}
	return nil
}

func (f *Fake) AddIssue(i Issue, labeledBy string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := i
	if cp.State == "" {
		cp.State = "OPEN"
	}
	f.Issues[i.Number] = &cp
	for _, l := range i.Labels {
		f.Actors[fmt.Sprintf("%d/%s", i.Number, l)] = labeledBy
	}
}

// AddComment simulates a human comment.
func (f *Fake) AddComment(n int, login, body string) int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	f.Cmts[n] = append(f.Cmts[n], Comment{ID: f.nextID, Login: login, Body: body, CreatedAt: f.Now()})
	return f.nextID
}

func (f *Fake) ListIssues(_ context.Context, label string) ([]Issue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("ListIssues"); err != nil {
		return nil, err
	}
	var out []Issue
	for _, i := range f.Issues {
		if i.State != "OPEN" {
			continue
		}
		for _, l := range i.Labels {
			if l == label {
				out = append(out, *i)
			}
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Number < out[b].Number })
	return out, nil
}

func (f *Fake) GetIssue(_ context.Context, n int) (Issue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("GetIssue"); err != nil {
		return Issue{}, err
	}
	i, ok := f.Issues[n]
	if !ok {
		return Issue{}, errors.New("not found")
	}
	return *i, nil
}

func (f *Fake) LabelActor(_ context.Context, n int, label string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Actors[fmt.Sprintf("%d/%s", n, label)], nil
}

func (f *Fake) Comments(_ context.Context, n int, after int64) ([]Comment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("Comments"); err != nil {
		return nil, err
	}
	var out []Comment
	for _, c := range f.Cmts[n] {
		if c.ID > after {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *Fake) FindMarker(_ context.Context, n int, marker string) (int64, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.Cmts[n] {
		if strings.Contains(c.Body, marker) {
			return c.ID, true, nil
		}
	}
	return 0, false, nil
}

func (f *Fake) Comment(_ context.Context, n int, body string) (int64, error) {
	f.mu.Lock()
	if err := f.fail("Comment"); err != nil {
		f.mu.Unlock()
		return 0, err
	}
	f.nextID++
	id := f.nextID
	f.Cmts[n] = append(f.Cmts[n], Comment{ID: id, Login: "issueflow-bot", Body: body, CreatedAt: f.Now()})
	f.Writes = append(f.Writes, Write{"comment", fmt.Sprintf("#%d %s", n, firstLine(body))})
	f.mu.Unlock()
	return id, f.after("comment")
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func (f *Fake) AddLabel(_ context.Context, n int, label string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("AddLabel"); err != nil {
		return err
	}
	i := f.Issues[n]
	for _, l := range i.Labels {
		if l == label {
			return nil
		}
	}
	i.Labels = append(i.Labels, label)
	f.Writes = append(f.Writes, Write{"add-label", fmt.Sprintf("#%d %s", n, label)})
	return nil
}

func (f *Fake) RemoveLabel(_ context.Context, n int, label string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.Issues[n]
	var keep []string
	for _, l := range i.Labels {
		if l != label {
			keep = append(keep, l)
		}
	}
	i.Labels = keep
	f.Writes = append(f.Writes, Write{"remove-label", fmt.Sprintf("#%d %s", n, label)})
	return nil
}

func (f *Fake) EnsureRelease(_ context.Context, tag, title, notes string) (Release, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("EnsureRelease"); err != nil {
		return Release{}, err
	}
	if r, ok := f.Releases[tag]; ok {
		return *r, nil
	}
	r := &Release{Tag: tag, URL: "https://example.invalid/releases/" + tag, Prerelease: true}
	f.Releases[tag] = r
	f.Writes = append(f.Writes, Write{"release", tag})
	return *r, nil
}

func (f *Fake) ListAssets(_ context.Context, tag string) ([]Asset, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Asset(nil), f.Assets[tag]...), nil
}

func (f *Fake) UploadAsset(_ context.Context, tag, path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("UploadAsset"); err != nil {
		return err
	}
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	name := filepath.Base(path)
	list := f.Assets[tag]
	for i := range list {
		if list[i].Name == name {
			return fmt.Errorf("asset %s already exists; refusing to overwrite", name)
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	d := sha256.Sum256(b)
	f.Assets[tag] = append(list, Asset{Name: name, Size: st.Size(), Digest: "sha256:" + hex.EncodeToString(d[:]), URL: "https://example.invalid/download/" + tag + "/" + name})
	f.Writes = append(f.Writes, Write{"upload", tag + "/" + name})
	return nil
}

func (f *Fake) WritesOf(kind string) []Write {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Write
	for _, w := range f.Writes {
		if w.Kind == kind {
			out = append(out, w)
		}
	}
	return out
}
