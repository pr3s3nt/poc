package main

import (
	"context"
	"fmt"
	"os"
	"sync"

	"orchestrator/internal/ports/credentials"
)

// journaledCredentials records every credential object the product writes or
// deletes in a durable append-only file, so a test runner can later delete
// exactly the objects this run created (and nothing else) even after a crash,
// a failed scenario or a server restart. Lines are "put <reference>" and
// "del <reference>" (references hold no secret). A write whose journal entry
// cannot be made durable is removed again and reported as failed.
type journaledCredentials struct {
	inner credentials.Store
	path  string
	mu    sync.Mutex
}

func (j *journaledCredentials) record(line string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	f, err := os.OpenFile(j.path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, line); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func (j *journaledCredentials) Durable() bool { return j.inner.Durable() }

func (j *journaledCredentials) Put(ctx context.Context, org, connection string, value []byte) (string, error) {
	ref, err := j.inner.Put(ctx, org, connection, value)
	if ref != "" {
		// Also a failed write may have landed (lost response): track it.
		if jerr := j.record("put " + ref); jerr != nil {
			_ = j.inner.Delete(context.WithoutCancel(ctx), org, connection, ref)
			return "", credentials.ErrUnavailable
		}
	}
	return ref, err
}

func (j *journaledCredentials) Get(ctx context.Context, org, connection, ref string) ([]byte, error) {
	return j.inner.Get(ctx, org, connection, ref)
}

func (j *journaledCredentials) Delete(ctx context.Context, org, connection, ref string) error {
	if err := j.inner.Delete(ctx, org, connection, ref); err != nil {
		return err
	}
	return j.record("del " + ref)
}
