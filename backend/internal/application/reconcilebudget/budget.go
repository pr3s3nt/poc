// Package reconcilebudget bounds independent background provider reads. It does
// not grant mutation authority or replace persisted leases and Environment locks.
package reconcilebudget

import "context"

var global = make(chan struct{}, 8)
var terraform = make(chan struct{}, 2)

func acquire(ctx context.Context, ch chan struct{}) error {
	select {
	case ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func Observation(ctx context.Context) (func(), error) {
	if err := acquire(ctx, global); err != nil {
		return nil, err
	}
	return func() { <-global }, nil
}
func Terraform(ctx context.Context) (func(), error) {
	if err := acquire(ctx, terraform); err != nil {
		return nil, err
	}
	release, err := Observation(ctx)
	if err != nil {
		<-terraform
		return nil, err
	}
	return func() { release(); <-terraform }, nil
}
