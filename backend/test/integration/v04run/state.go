package v04run

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"orchestrator/test/integration/v03run"
)

// Instance is the safe identity of one Terraform resource instance the product recorded: the immutable
// instance ID, the first 12 characters of its state key (the key itself is never read) and its status.
type Instance struct {
	Descriptor   string `json:"descriptor"`
	ID           string `json:"id"`
	StateKey     string `json:"stateKey12"`
	Status       string `json:"status"`
	CreatedByRun string `json:"createdByRun"`
	Scope        string `json:"scope"`
}

// ReadInstances lists every instance of the run's (exclusive, disposable) Console database.
func ReadInstances(ctx context.Context, q v03run.Querier) ([]Instance, error) {
	rows, err := q.Query(ctx, `SELECT descriptor, id::text, left(state_key, 12), status, created_by_run_id, scope_type FROM terraform_resource_instances ORDER BY descriptor, scope_type, scope_id`)
	if err != nil {
		return nil, fmt.Errorf("instances cannot be read: %w", err)
	}
	defer rows.Close()
	var out []Instance
	for rows.Next() {
		var in Instance
		if err := rows.Scan(&in.Descriptor, &in.ID, &in.StateKey, &in.Status, &in.CreatedByRun, &in.Scope); err != nil {
			return nil, fmt.Errorf("instances cannot be read: %w", err)
		}
		out = append(out, in)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("instances cannot be read: %w", err)
	}
	return out, nil
}

// SharedServices returns the instances of the in-cluster PostgreSQL and Redis. There must be exactly one of each.
func SharedServices(instances []Instance) (postgres, redis []Instance) {
	for _, in := range instances {
		switch {
		case strings.HasPrefix(in.Descriptor, "postgres."):
			postgres = append(postgres, in)
		case strings.HasPrefix(in.Descriptor, "redis."):
			redis = append(redis, in)
		}
	}
	return postgres, redis
}

// CheckSingleton fails when the product recorded zero or several instances of a shared service.
func CheckSingleton(instances []Instance) error {
	pg, rd := SharedServices(instances)
	if len(pg) != 1 || len(rd) != 1 {
		return fmt.Errorf("expected one PostgreSQL and one Redis instance, found %d and %d", len(pg), len(rd))
	}
	return nil
}

// SameInstances proves a later read describes the same logical resources with the same immutable identity:
// nothing was replaced, duplicated or dropped. A resource that is new in the later read is reported.
func SameInstances(before, after []Instance) error {
	index := func(list []Instance) map[string]Instance {
		m := map[string]Instance{}
		for _, in := range list {
			m[in.Descriptor+"|"+in.Scope] = in
		}
		return m
	}
	b, a := index(before), index(after)
	keys := make([]string, 0, len(b))
	for key := range b {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		later, ok := a[key]
		switch {
		case !ok:
			return fmt.Errorf("instance %s disappeared", strings.SplitN(key, "|", 2)[0])
		case later.ID != b[key].ID || later.StateKey != b[key].StateKey:
			return fmt.Errorf("instance %s was replaced (new identity)", strings.SplitN(key, "|", 2)[0])
		}
	}
	for key := range a {
		if _, ok := b[key]; !ok {
			return fmt.Errorf("instance %s is new", strings.SplitN(key, "|", 2)[0])
		}
	}
	if len(before) != len(after) {
		return fmt.Errorf("the instance count changed from %d to %d", len(before), len(after))
	}
	return nil
}
