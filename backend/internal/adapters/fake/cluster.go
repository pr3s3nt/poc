package fake

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"orchestrator/internal/ports/execution"
)

// Cluster is the in-memory workload/database/public-route state used by the
// fake executor and deployer. Objects keep physical cluster/namespace identity.
type Cluster struct {
	mu sync.Mutex

	Counts     map[string]int
	Databases  map[string]*Database
	Routes     map[string]execution.PublicRoute
	Namespaces map[string]bool
	// Fail injects a named fake route operation failure.
	Fail   map[string]error
	Log    []string
	failAt map[string]map[int]error
	counts map[string]int
}

// Database is one fake PostgreSQL server.
type Database struct {
	Version int
	Tables  map[string]int64
}

// NewCluster returns an empty cluster.
func NewCluster() *Cluster {
	return &Cluster{Counts: map[string]int{}, Databases: map[string]*Database{}, Routes: map[string]execution.PublicRoute{}, Namespaces: map[string]bool{}, Fail: map[string]error{}, failAt: map[string]map[int]error{}, counts: map[string]int{}}
}

func clusterKey(t execution.Target, parts ...string) string {
	return t.Context + "|" + t.Namespace + "|" + strings.Join(parts, "/")
}

func (c *Cluster) failure(op string) error {
	c.counts[op]++
	if err, ok := c.failAt[op][c.counts[op]]; ok {
		return err
	}
	return c.Fail[op]
}

func (c *Cluster) logf(format string, args ...any) {
	c.Log = append(c.Log, fmt.Sprintf(format, args...))
}

// Calls returns a copy of the operation log.
func (c *Cluster) Calls() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.Log...)
}

// Inject sets or clears (nil) a failure for an operation name.
func (c *Cluster) Inject(op string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err == nil {
		delete(c.Fail, op)
		return
	}
	c.Fail[op] = err
}

// Observe records applied Deployments so replicas can be scaled.
func (c *Cluster) Observe(target execution.Target, manifests []execution.Manifest) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, m := range manifests {
		if m.Kind != "Deployment" {
			continue
		}
		replicas := 1
		if spec, ok := m.Object["spec"].(map[string]any); ok {
			switch v := spec["replicas"].(type) {
			case int:
				replicas = v
			case float64:
				replicas = int(v)
			}
		}
		c.Counts[clusterKey(target, m.Name)] = replicas
	}
}

// SeedDatabase creates or replaces a database with the given tables.
func (c *Cluster) SeedDatabase(target execution.Target, name string, tables map[string]int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	copied := map[string]int64{}
	for k, v := range tables {
		copied[k] = v
	}
	c.Databases[clusterKey(target, name)] = &Database{Version: 160004, Tables: copied}
}

// Database returns a copy of a database.
func (c *Cluster) Database(target execution.Target, name string) (Database, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	db, ok := c.Databases[clusterKey(target, name)]
	if !ok {
		return Database{}, false
	}
	return copyDatabase(*db), true
}

func copyDatabase(in Database) Database {
	out := Database{Version: in.Version, Tables: map[string]int64{}}
	for k, v := range in.Tables {
		out.Tables[k] = v
	}
	return out
}

// Replicas, Scale -------------------------------------------------------

func (c *Cluster) ReplicasOf(target execution.Target, workload string) (int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n, ok := c.Counts[clusterKey(target, workload)]
	return n, ok
}

// Routes ---------------------------------------------------------------

// Reconcile implements execution.PublicRouteManager over the fake cluster.
func (c *Cluster) Reconcile(_ context.Context, target execution.Target, route execution.PublicRoute) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logf("route %s paths=%d", target.Namespace, len(route.Paths))
	if err := c.failure("route"); err != nil {
		return err
	}
	if len(route.Paths) == 0 {
		delete(c.Routes, clusterKey(target))
		return nil
	}
	c.Routes[clusterKey(target)] = route
	return nil
}

var _ execution.PublicRouteManager = (*Cluster)(nil)

// EnsureDatabase creates an empty database when none exists; it never resets one.
func (c *Cluster) EnsureDatabase(target execution.Target, name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := clusterKey(target, name)
	if _, ok := c.Databases[key]; !ok {
		c.Databases[key] = &Database{Version: 160004, Tables: map[string]int64{}}
		c.logf("provision-database %s/%s", target.Namespace, name)
	}
}

// Record appends an entry to the operation log (used by ordering tests).
func (c *Cluster) Record(entry string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Log = append(c.Log, entry)
}

// SetVersion sets the server_version_num of a database.
func (c *Cluster) SetVersion(target execution.Target, name string, version int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if db, ok := c.Databases[clusterKey(target, name)]; ok {
		db.Version = version
	}
}

// InjectOnce makes the nth (1-based, counted from now) call of an operation
// fail once; several can be registered for one operation.
func (c *Cluster) InjectOnce(op string, err error, nth int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failAt[op] == nil {
		c.failAt[op] = map[int]error{}
	}
	c.failAt[op][c.counts[op]+nth] = err
}
