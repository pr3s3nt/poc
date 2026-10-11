// Package gh is the GitHub port: a gh-CLI transport and an in-memory fake.
package gh

import (
	"context"
	"errors"
	"time"
)

type Issue struct {
	Number int
	Title  string
	Body   string
	Author string
	Labels []string
	State  string
}

type Comment struct {
	ID        int64
	Login     string
	Body      string
	CreatedAt time.Time
}

type Asset struct {
	Name   string
	Size   int64
	URL    string
	Digest string // "sha256:<hex>" when the host reports one
}

// ErrNotFound is returned only for a definite "does not exist" answer.
var ErrNotFound = errors.New("not found")

type Release struct {
	Tag        string
	URL        string
	Prerelease bool
}

// GitHub is everything the supervisor needs. All writes are idempotent at the
// caller via markers; the transport itself does not retry.
type GitHub interface {
	ListIssues(ctx context.Context, label string) ([]Issue, error)
	GetIssue(ctx context.Context, n int) (Issue, error)
	// LabelActor returns the login that most recently applied label.
	LabelActor(ctx context.Context, n int, label string) (string, error)
	Comments(ctx context.Context, n int, afterID int64) ([]Comment, error)
	FindMarker(ctx context.Context, n int, marker string) (int64, bool, error)
	Comment(ctx context.Context, n int, body string) (int64, error)
	AddLabel(ctx context.Context, n int, label string) error
	RemoveLabel(ctx context.Context, n int, label string) error
	EnsureRelease(ctx context.Context, tag, title, notes string) (Release, error)
	ListAssets(ctx context.Context, tag string) ([]Asset, error)
	// UploadAsset never overwrites: an existing asset with the same name is an error.
	UploadAsset(ctx context.Context, tag, path string) error
}
