package fake

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"

	"orchestrator/internal/ports/execution"
)

// Cluster is an in-memory stand-in for the Kubernetes side of a target
// transition: workload replicas, PostgreSQL databases with row counts, public
// Ingress ownership and namespaces. It implements the transition ports so the
// service logic is tested without a cluster. Objects are keyed by the physical
// cluster (Target.Context) and namespace, so two logical Connections that share
// a cluster stay distinguishable only through their namespaces.
type Cluster struct {
	mu sync.Mutex

	Counts     map[string]int
	Databases  map[string]*Database
	Archives   map[string]Database
	Routes     map[string]execution.PublicRoute
	Namespaces map[string]bool
	// Fail injects a failure for the named operation:
	// scale, scale-up, inspect, backup, restore, route, cleanup, remove-archive.
	Fail       map[string]error
	Log        []string
	failAt     map[string]map[int]error
	counts     map[string]int
	identities map[string]string
}

// Database is one fake PostgreSQL server.
type Database struct {
	Version int
	Tables  map[string]int64
}

// NewCluster returns an empty cluster.
func NewCluster() *Cluster {
	return &Cluster{Counts: map[string]int{}, Databases: map[string]*Database{}, Archives: map[string]Database{}, Routes: map[string]execution.PublicRoute{}, Namespaces: map[string]bool{}, Fail: map[string]error{}, failAt: map[string]map[int]error{}, counts: map[string]int{}, identities: map[string]string{}}
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

// Replicas implements execution.WorkloadScaler.
func (c *Cluster) Replicas(_ context.Context, target execution.Target, workloadID string) (int, bool, error) {
	n, ok := c.ReplicasOf(target, workloadID)
	return n, ok, nil
}

// Scale implements execution.WorkloadScaler.
func (c *Cluster) Scale(_ context.Context, target execution.Target, workloadID string, replicas int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	op := "scale"
	if replicas > 0 {
		op = "scale-up"
	}
	c.logf("%s %s/%s=%d", op, target.Namespace, workloadID, replicas)
	if err := c.failure(op); err != nil {
		return err
	}
	if err := c.failure(op + ":" + workloadID); err != nil {
		return err
	}
	c.Counts[clusterKey(target, workloadID)] = replicas
	return nil
}

// PostgresTransfer -----------------------------------------------------

func (c *Cluster) database(res execution.PostgresResource) (*Database, error) {
	db, ok := c.Databases[clusterKey(res.Target, res.Name)]
	if !ok {
		return nil, fmt.Errorf("fake: database %s/%s does not exist", res.Namespace, res.Name)
	}
	return db, nil
}

// Inspect implements execution.PostgresTransfer.
func (c *Cluster) Inspect(_ context.Context, res execution.PostgresResource) (execution.PostgresInventory, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logf("inspect %s/%s", res.Namespace, res.Name)
	if err := c.failure("inspect"); err != nil {
		return execution.PostgresInventory{}, err
	}
	db, err := c.database(res)
	if err != nil {
		return execution.PostgresInventory{}, err
	}
	return execution.PostgresInventory{ServerVersionNum: db.Version, Tables: copyDatabase(*db).Tables}, nil
}

func tablesHash(tables map[string]int64) string {
	names := make([]string, 0, len(tables))
	for name := range tables {
		names = append(names, name)
	}
	sort.Strings(names)
	h := sha256.New()
	for _, name := range names {
		fmt.Fprintf(h, "%s=%d;", name, tables[name])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Backup implements execution.PostgresTransfer.
func (c *Cluster) Backup(_ context.Context, res execution.PostgresResource, want execution.PostgresArchive) (execution.PostgresArchive, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logf("backup %s/%s", res.Namespace, res.Name)
	if err := c.failure("backup"); err != nil {
		return execution.PostgresArchive{}, err
	}
	db, err := c.database(res)
	if err != nil {
		return execution.PostgresArchive{}, err
	}
	dir := want.Dir
	archive := execution.PostgresArchive{Pod: res.Name + "-0", Dir: dir, SHA256: tablesHash(db.Tables), Bytes: 1024}
	c.Archives[clusterKey(res.Target, dir)] = copyDatabase(*db)
	return archive, nil
}

// Restore implements execution.PostgresTransfer.
func (c *Cluster) Restore(_ context.Context, src execution.PostgresResource, archive execution.PostgresArchive, dst execution.PostgresResource) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logf("restore %s/%s -> %s/%s", src.Namespace, src.Name, dst.Namespace, dst.Name)
	if err := c.failure("restore"); err != nil {
		return err
	}
	snapshot, ok := c.Archives[clusterKey(src.Target, archive.Dir)]
	if !ok {
		return fmt.Errorf("fake: archive %s does not exist", archive.Dir)
	}
	db, err := c.database(dst)
	if err != nil {
		return err
	}
	db.Tables = copyDatabase(snapshot).Tables
	return nil
}

// RemoveArchive implements execution.PostgresTransfer.
func (c *Cluster) RemoveArchive(_ context.Context, src execution.PostgresResource, archive execution.PostgresArchive) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logf("remove-archive %s", archive.Dir)
	if err := c.failure("remove-archive"); err != nil {
		return err
	}
	delete(c.Archives, clusterKey(src.Target, archive.Dir))
	return nil
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

// HasPublicRoute implements execution.RouteInspector.
func (c *Cluster) HasPublicRoute(_ context.Context, target execution.Target, applicationID, environmentID string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	route, ok := c.Routes[clusterKey(target)]
	return ok && route.ApplicationID == applicationID && route.EnvironmentID == environmentID, nil
}

// DeleteOwnedNamespace implements execution.NamespaceCleaner.
func (c *Cluster) DeleteOwnedNamespace(_ context.Context, target execution.Target, namespace, applicationID, environmentID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logf("delete-namespace %s %s/%s", namespace, applicationID, environmentID)
	if err := c.failure("cleanup"); err != nil {
		return err
	}
	prefix := target.Context + "|" + namespace + "|"
	for key := range c.Counts {
		if strings.HasPrefix(key, prefix) {
			delete(c.Counts, key)
		}
	}
	for key := range c.Databases {
		if strings.HasPrefix(key, prefix) {
			delete(c.Databases, key)
		}
	}
	for key := range c.Routes {
		if strings.HasPrefix(key, prefix) {
			delete(c.Routes, key)
		}
	}
	delete(c.Namespaces, target.Context+"|"+namespace)
	return nil
}

var (
	_ execution.WorkloadScaler     = (*Cluster)(nil)
	_ execution.PostgresTransfer   = (*Cluster)(nil)
	_ execution.RouteInspector     = (*Cluster)(nil)
	_ execution.NamespaceCleaner   = (*Cluster)(nil)
	_ execution.PublicRouteManager = (*Cluster)(nil)
)

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

// Probe implements execution.TargetProbe.
func (c *Cluster) Probe(_ context.Context, target execution.Target) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logf("probe %s", target.Context)
	if err := c.failure("probe"); err != nil {
		return err
	}
	return c.failure("probe:" + target.Context)
}

// SetIdentity gives a kube context its own physical cluster identity; contexts
// without one share the default cluster (the same-physical-cluster case).
func (c *Cluster) SetIdentity(kubeContext, identity string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.identities[kubeContext] = identity
}

// ClusterIdentity implements execution.TargetProbe.
func (c *Cluster) ClusterIdentity(_ context.Context, target execution.Target) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.failure("identity"); err != nil {
		return "", err
	}
	if id, ok := c.identities[target.Context]; ok {
		return id, nil
	}
	return "fake-physical-cluster", nil
}
