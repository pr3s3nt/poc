package gh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"issueflow/internal/runner"
)

// CLI talks to GitHub only through the authenticated `gh` binary. It never
// reads tokens; auth is whatever `gh` already has.
type CLI struct {
	Repo string
	R    runner.Runner
}

func (c *CLI) run(ctx context.Context, args ...string) (string, error) {
	res, err := c.R.Run(ctx, "", nil, "gh", args...)
	if err != nil {
		return "", err
	}
	if res.Exit != 0 {
		return "", fmt.Errorf("gh %s: exit %d: %s", strings.Join(args[:min(len(args), 3)], " "), res.Exit, strings.TrimSpace(res.Stderr))
	}
	return res.Stdout, nil
}

type ghIssue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	State  string `json:"state"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

func (g ghIssue) conv() Issue {
	i := Issue{Number: g.Number, Title: g.Title, Body: g.Body, Author: g.Author.Login, State: g.State}
	for _, l := range g.Labels {
		i.Labels = append(i.Labels, l.Name)
	}
	return i
}

const issueFields = "number,title,body,author,labels,state"

type restIssue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	State  string `json:"state"`
	User   struct {
		Login string `json:"login"`
	} `json:"user"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	PullRequest *struct{} `json:"pull_request"`
}

// ListIssues reads EVERY open Issue with the label, oldest first, through the
// REST API with --paginate (gh issue list --limit returns only the newest N,
// which cannot yield the globally smallest ready Issue). Pull requests, which
// the issues endpoint also returns, are skipped.
func (c *CLI) ListIssues(ctx context.Context, label string) ([]Issue, error) {
	q := url.Values{}
	q.Set("state", "open")
	q.Set("labels", label)
	q.Set("sort", "created")
	q.Set("direction", "asc")
	q.Set("per_page", "100")
	out, err := c.run(ctx, "api", "--paginate", fmt.Sprintf("repos/%s/issues?%s", c.Repo, q.Encode()))
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(out))
	var res []Issue
	for dec.More() {
		var page []restIssue
		if err := dec.Decode(&page); err != nil {
			return nil, fmt.Errorf("issues page: %w", err)
		}
		for _, r := range page {
			if r.PullRequest != nil {
				continue
			}
			i := Issue{Number: r.Number, Title: r.Title, Body: r.Body, Author: r.User.Login, State: strings.ToUpper(r.State)}
			for _, l := range r.Labels {
				i.Labels = append(i.Labels, l.Name)
			}
			res = append(res, i)
		}
	}
	sort.Slice(res, func(i, j int) bool { return res[i].Number < res[j].Number })
	return res, nil
}

func (c *CLI) GetIssue(ctx context.Context, n int) (Issue, error) {
	out, err := c.run(ctx, "issue", "view", fmt.Sprint(n), "-R", c.Repo, "--json", issueFields)
	if err != nil {
		return Issue{}, err
	}
	var raw ghIssue
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return Issue{}, err
	}
	return raw.conv(), nil
}

func (c *CLI) LabelActor(ctx context.Context, n int, label string) (string, error) {
	out, err := c.run(ctx, "api", "--paginate", fmt.Sprintf("repos/%s/issues/%d/events", c.Repo, n))
	if err != nil {
		return "", err
	}
	// --paginate concatenates JSON arrays: decode them one by one.
	dec := json.NewDecoder(strings.NewReader(out))
	actor := ""
	for dec.More() {
		var evs []struct {
			Event string `json:"event"`
			Actor struct {
				Login string `json:"login"`
			} `json:"actor"`
			Label struct {
				Name string `json:"name"`
			} `json:"label"`
		}
		if err := dec.Decode(&evs); err != nil {
			return "", err
		}
		for _, e := range evs {
			if e.Event == "labeled" && e.Label.Name == label {
				actor = e.Actor.Login
			}
		}
	}
	return actor, nil
}

type ghComment struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	User      struct {
		Login string `json:"login"`
	} `json:"user"`
}

func (c *CLI) allComments(ctx context.Context, n int) ([]Comment, error) {
	out, err := c.run(ctx, "api", "--paginate", fmt.Sprintf("repos/%s/issues/%d/comments", c.Repo, n))
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(out))
	var res []Comment
	for dec.More() {
		var page []ghComment
		if err := dec.Decode(&page); err != nil {
			return nil, err
		}
		for _, p := range page {
			res = append(res, Comment{ID: p.ID, Login: p.User.Login, Body: p.Body, CreatedAt: p.CreatedAt})
		}
	}
	sort.Slice(res, func(i, j int) bool { return res[i].ID < res[j].ID })
	return res, nil
}

func (c *CLI) Comments(ctx context.Context, n int, afterID int64) ([]Comment, error) {
	all, err := c.allComments(ctx, n)
	if err != nil {
		return nil, err
	}
	var res []Comment
	for _, cm := range all {
		if cm.ID > afterID {
			res = append(res, cm)
		}
	}
	return res, nil
}

func (c *CLI) FindMarker(ctx context.Context, n int, marker string) (int64, bool, error) {
	all, err := c.allComments(ctx, n)
	if err != nil {
		return 0, false, err
	}
	for _, cm := range all {
		if strings.Contains(cm.Body, marker) {
			return cm.ID, true, nil
		}
	}
	return 0, false, nil
}

func (c *CLI) Comment(ctx context.Context, n int, body string) (int64, error) {
	out, err := c.run(ctx, "api", "-X", "POST", fmt.Sprintf("repos/%s/issues/%d/comments", c.Repo, n), "-f", "body="+body)
	if err != nil {
		return 0, err
	}
	var r ghComment
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		return 0, err
	}
	return r.ID, nil
}

func (c *CLI) AddLabel(ctx context.Context, n int, label string) error {
	_, err := c.run(ctx, "issue", "edit", fmt.Sprint(n), "-R", c.Repo, "--add-label", label)
	return err
}

func (c *CLI) RemoveLabel(ctx context.Context, n int, label string) error {
	_, err := c.run(ctx, "issue", "edit", fmt.Sprint(n), "-R", c.Repo, "--remove-label", label)
	return err
}

func (c *CLI) viewRelease(ctx context.Context, tag string) (Release, error) {
	res, err := c.R.Run(ctx, "", nil, "gh", "release", "view", tag, "-R", c.Repo, "--json", "tagName,url,isPrerelease")
	if err != nil {
		return Release{}, err
	}
	if res.Exit != 0 {
		if strings.Contains(strings.ToLower(res.Stderr), "release not found") {
			return Release{}, ErrNotFound
		}
		return Release{}, fmt.Errorf("gh release view: exit %d: %s", res.Exit, strings.TrimSpace(res.Stderr))
	}
	var r struct {
		URL string `json:"url"`
		Pre bool   `json:"isPrerelease"`
	}
	if err := json.Unmarshal([]byte(res.Stdout), &r); err != nil {
		return Release{}, fmt.Errorf("gh release view: unparseable output: %w", err)
	}
	return Release{Tag: tag, URL: r.URL, Prerelease: r.Pre}, nil
}

// EnsureRelease creates the prerelease only after a confirmed "release not
// found". Auth, network and parse errors are returned, never answered by
// creating something.
func (c *CLI) EnsureRelease(ctx context.Context, tag, title, notes string) (Release, error) {
	r, err := c.viewRelease(ctx, tag)
	if err == nil {
		return r, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Release{}, err
	}
	if _, err := c.run(ctx, "release", "create", tag, "-R", c.Repo, "--prerelease", "--title", title, "--notes", notes); err != nil {
		// A concurrent creator may have won; accept only if it now exists.
		if r, verr := c.viewRelease(ctx, tag); verr == nil {
			return r, nil
		}
		return Release{}, err
	}
	return c.viewRelease(ctx, tag)
}

func (c *CLI) ListAssets(ctx context.Context, tag string) ([]Asset, error) {
	out, err := c.run(ctx, "release", "view", tag, "-R", c.Repo, "--json", "assets")
	if err != nil {
		return nil, err
	}
	var r struct {
		Assets []struct {
			Name   string `json:"name"`
			Size   int64  `json:"size"`
			URL    string `json:"url"`
			Digest string `json:"digest"`
		} `json:"assets"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		return nil, err
	}
	var res []Asset
	for _, a := range r.Assets {
		res = append(res, Asset{Name: a.Name, Size: a.Size, URL: a.URL, Digest: a.Digest})
	}
	return res, nil
}

func (c *CLI) UploadAsset(ctx context.Context, tag, path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	_, err := c.run(ctx, "release", "upload", tag, "-R", c.Repo, filepath.Clean(path))
	return err
}
