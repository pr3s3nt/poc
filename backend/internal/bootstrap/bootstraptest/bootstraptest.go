// Package bootstraptest builds the application for tests with the seeded
// cluster explicitly prepared READY (see seedtest).
package bootstraptest

import (
	"context"

	"orchestrator/internal/bootstrap"
	"orchestrator/internal/ports/persistence"
	"orchestrator/internal/seed"
	"orchestrator/internal/seed/seedtest"
)

// Build builds the application and then marks the seeded cluster READY with
// a synthetic scoped credential. Only test harnesses use it.
func Build(ctx context.Context, opts bootstrap.Options) (*bootstrap.App, error) {
	app, err := bootstrap.Build(ctx, opts)
	if err != nil {
		return nil, err
	}
	// Use the app's injected credential store when it has one so the reference
	// is resolvable; otherwise this is fake-only metadata (see seedtest.Ready).
	ready := seedtest.Ready
	if app.ConnectionCredentials != nil {
		ready = func(ctx context.Context, st persistence.Store, o seed.Options) error {
			return seedtest.ReadyIn(ctx, st, app.ConnectionCredentials, o)
		}
	}
	if err := ready(ctx, app.Store, opts.Seed); err != nil {
		return nil, err
	}
	return app, nil
}
