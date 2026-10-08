package kubernetes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"orchestrator/internal/ports/execution"
)

// Transition implements the ADR-012 target-transition ports on kubectl: writer
// quiescing, PostgreSQL 16 backup/stream/restore through the database Pod's
// local socket, route inspection and owned-namespace cleanup. It never reads a
// database password: a server that demands one is reported unsupported.
type Transition struct {
	KubectlPath string
	Credentials execution.KubeconfigSource
	Timeout     time.Duration
}

var (
	identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,62}$`)
	dnsLabel   = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	hexSHA     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	backupDir  = regexp.MustCompile(`^/tmp/orch-backup\.[A-Za-z0-9]{8,32}$`)
)

// ErrPostgresUnsupported marks a server this transfer cannot back up safely.
var ErrPostgresUnsupported = errors.New("kubernetes: the PostgreSQL server is not supported for automatic transfer")

func (t *Transition) timeout() time.Duration {
	if t.Timeout > 0 {
		return t.Timeout
	}
	return 5 * time.Minute
}

func (t *Transition) open(ctx context.Context, target execution.Target) (CLI, func(), error) {
	if !target.Explicit() {
		return CLI{}, func() {}, fmt.Errorf("kubernetes: an explicit cluster target is required")
	}
	return OpenCLI(ctx, t.KubectlPath, t.Credentials, target)
}

// Probe verifies reachability and the permissions a deployment needs.
func (t *Transition) Probe(ctx context.Context, target execution.Target) error {
	cli, cleanup, err := t.open(ctx, target)
	if err != nil {
		return err
	}
	defer cleanup()
	if _, err := cli.Run(ctx, nil, "version", "-o", "json"); err != nil {
		return fmt.Errorf("kubernetes: the destination cluster is not reachable")
	}
	for _, check := range [][]string{{"namespaces"}, {"deployments", "-A"}, {"statefulsets", "-A"}, {"services", "-A"}, {"secrets", "-A"}, {"ingresses", "-A"}} {
		args := append([]string{"auth", "can-i", "create"}, check...)
		out, err := cli.Run(ctx, nil, args...)
		if err != nil || strings.TrimSpace(string(out)) != "yes" {
			return fmt.Errorf("kubernetes: the destination credential may not create %s", check[0])
		}
	}
	return nil
}

// ClusterIdentity reads the kube-system namespace UID, a read-only identity that
// is unique per cluster and independent of the Connection or context name.
func (t *Transition) ClusterIdentity(ctx context.Context, target execution.Target) (string, error) {
	cli, cleanup, err := t.open(ctx, target)
	if err != nil {
		return "", err
	}
	defer cleanup()
	object, found, err := cli.Get(ctx, "", "namespace", "kube-system")
	if err != nil || !found {
		return "", fmt.Errorf("kubernetes: the cluster identity could not be read")
	}
	metadata, _ := object["metadata"].(map[string]any)
	uid, _ := metadata["uid"].(string)
	if uid == "" {
		return "", fmt.Errorf("kubernetes: the cluster identity could not be read")
	}
	return uid, nil
}

// Replicas reads spec.replicas of the workload Deployment.
func (t *Transition) Replicas(ctx context.Context, target execution.Target, workloadID string) (int, bool, error) {
	cli, cleanup, err := t.open(ctx, target)
	if err != nil {
		return 0, false, err
	}
	defer cleanup()
	object, found, err := cli.Get(ctx, target.Namespace, "deployment", workloadID)
	if err != nil || !found {
		return 0, false, err
	}
	spec, _ := object["spec"].(map[string]any)
	if replicas, ok := spec["replicas"].(float64); ok {
		return int(replicas), true, nil
	}
	return 1, true, nil
}

// Scale sets the replicas of the workload Deployment and waits for Pods to go
// away when scaling to zero.
func (t *Transition) Scale(ctx context.Context, target execution.Target, workloadID string, replicas int) error {
	if replicas < 0 || !dnsLabel.MatchString(workloadID) || !dnsLabel.MatchString(target.Namespace) {
		return fmt.Errorf("kubernetes: invalid scale request")
	}
	cli, cleanup, err := t.open(ctx, target)
	if err != nil {
		return err
	}
	defer cleanup()
	if _, err := cli.Run(ctx, nil, "scale", "deployment", workloadID, "-n", target.Namespace, "--replicas="+strconv.Itoa(replicas)); err != nil {
		return err
	}
	if replicas > 0 {
		return nil
	}
	deadline, cancel := context.WithTimeout(ctx, t.timeout())
	defer cancel()
	for {
		object, found, err := cli.Get(deadline, target.Namespace, "deployment", workloadID)
		if err != nil {
			return err
		}
		status, _ := object["status"].(map[string]any)
		if !found || numeric(status["replicas"]) == 0 {
			return nil
		}
		select {
		case <-deadline.Done():
			return fmt.Errorf("kubernetes: workload %s did not stop in time", workloadID)
		case <-time.After(time.Second):
		}
	}
}

func numeric(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

func validPostgres(res execution.PostgresResource) error {
	if !dnsLabel.MatchString(res.Namespace) || !dnsLabel.MatchString(res.Name) || !identifier.MatchString(res.Database) || !identifier.MatchString(res.Username) {
		return fmt.Errorf("%w: invalid resource identity", ErrPostgresUnsupported)
	}
	return nil
}

func podOf(res execution.PostgresResource) string { return res.Name + "-0" }

// sh runs a fixed script in the database container; user-influenced values
// travel only as positional parameters, never inside the script text.
func (t *Transition) sh(ctx context.Context, cli CLI, res execution.PostgresResource, script string, params ...string) ([]byte, error) {
	args := []string{"exec", "-n", res.Namespace, podOf(res), "--", "sh", "-c", script, "sh"}
	return cli.Run(ctx, nil, append(args, params...)...)
}

// Inspect proves the server accepts local connections and fingerprints it.
func (t *Transition) Inspect(ctx context.Context, res execution.PostgresResource) (execution.PostgresInventory, error) {
	if err := validPostgres(res); err != nil {
		return execution.PostgresInventory{}, err
	}
	cli, cleanup, err := t.open(ctx, res.Target)
	if err != nil {
		return execution.PostgresInventory{}, err
	}
	defer cleanup()
	const script = `psql -X -q -U "$1" -d "$2" -v ON_ERROR_STOP=1 -At -F '|' -c "SHOW server_version_num" -c "SELECT table_schema||'.'||table_name, (xpath('/row/c/text()', query_to_xml(format('select count(*) as c from %I.%I', table_schema, table_name), false, true, '')))[1]::text FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog','information_schema') AND table_type='BASE TABLE' ORDER BY 1"`
	out, err := t.sh(ctx, cli, res, script, res.Username, res.Database)
	if err != nil {
		return execution.PostgresInventory{}, fmt.Errorf("%w: the server could not be inspected", ErrPostgresUnsupported)
	}
	inventory := execution.PostgresInventory{Tables: map[string]int64{}}
	for index, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if index == 0 {
			version, err := strconv.Atoi(line)
			if err != nil {
				return inventory, fmt.Errorf("%w: unreadable server version", ErrPostgresUnsupported)
			}
			inventory.ServerVersionNum = version
			continue
		}
		name, count, ok := strings.Cut(line, "|")
		rows, err := strconv.ParseInt(count, 10, 64)
		if !ok || err != nil {
			return inventory, fmt.Errorf("%w: unreadable table inventory", ErrPostgresUnsupported)
		}
		inventory.Tables[name] = rows
	}
	return inventory, nil
}

// Backup creates the private archive at want.Dir on the source Pod. The script
// owns its directory on every path: any failure removes it before exiting, and
// the result is trusted only after the 0700/0600 modes were read back.
func (t *Transition) Backup(ctx context.Context, res execution.PostgresResource, want execution.PostgresArchive) (execution.PostgresArchive, error) {
	if err := validPostgres(res); err != nil {
		return execution.PostgresArchive{}, err
	}
	if !backupDir.MatchString(want.Dir) {
		return execution.PostgresArchive{}, fmt.Errorf("kubernetes: invalid backup path")
	}
	cli, cleanup, err := t.open(ctx, res.Target)
	if err != nil {
		return execution.PostgresArchive{}, err
	}
	defer cleanup()
	const script = `umask 077; d="$3"; [ ! -e "$d" ] || exit 1; mkdir -m 700 "$d" || exit 1;
if pg_dump -U "$1" -d "$2" -Fc --no-owner --no-privileges -f "$d/dump.pgc" && chmod 600 "$d/dump.pgc" && [ "$(stat -c %a "$d")" = 700 ] && [ "$(stat -c %a "$d/dump.pgc")" = 600 ]; then
  h=$(sha256sum "$d/dump.pgc" | cut -d' ' -f1); s=$(wc -c < "$d/dump.pgc"); printf '%s %s' "$h" "$s"
else rm -rf "$d"; exit 1; fi`
	out, err := t.sh(ctx, cli, res, script, res.Username, res.Database, want.Dir)
	if err != nil {
		// The script cleans up after itself; an interrupted exec is covered by a
		// bounded, independent removal of the recorded path.
		removal, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		_ = t.removeDir(removal, cli, res, want.Dir)
		return execution.PostgresArchive{}, fmt.Errorf("kubernetes: the PostgreSQL backup failed")
	}
	fields := strings.Fields(string(out))
	if len(fields) != 2 || !hexSHA.MatchString(fields[0]) {
		_ = t.removeDir(context.WithoutCancel(ctx), cli, res, want.Dir)
		return execution.PostgresArchive{}, fmt.Errorf("kubernetes: the PostgreSQL backup returned an unexpected handle")
	}
	size, _ := strconv.ParseInt(fields[1], 10, 64)
	return execution.PostgresArchive{Pod: podOf(res), Dir: want.Dir, SHA256: fields[0], Bytes: size}, nil
}

func (t *Transition) removeDir(ctx context.Context, cli CLI, res execution.PostgresResource, dir string) error {
	if !backupDir.MatchString(dir) {
		return fmt.Errorf("kubernetes: refusing to remove a non-backup path")
	}
	_, err := t.sh(ctx, cli, res, `rm -rf "$1"`, dir)
	return err
}

// RemoveArchive deletes the private archive directory from the source Pod.
func (t *Transition) RemoveArchive(ctx context.Context, src execution.PostgresResource, archive execution.PostgresArchive) error {
	if err := validPostgres(src); err != nil {
		return err
	}
	cli, cleanup, err := t.open(ctx, src.Target)
	if err != nil {
		return err
	}
	defer cleanup()
	return t.removeDir(ctx, cli, src, archive.Dir)
}

// Restore pipes the source archive into pg_restore on the destination Pod.
func (t *Transition) Restore(ctx context.Context, src execution.PostgresResource, archive execution.PostgresArchive, dst execution.PostgresResource) error {
	if err := validPostgres(src); err != nil {
		return err
	}
	if err := validPostgres(dst); err != nil {
		return err
	}
	if !backupDir.MatchString(archive.Dir) || !hexSHA.MatchString(archive.SHA256) {
		return fmt.Errorf("kubernetes: invalid backup handle")
	}
	srcCLI, srcCleanup, err := t.open(ctx, src.Target)
	if err != nil {
		return err
	}
	defer srcCleanup()
	dstCLI, dstCleanup, err := t.open(ctx, dst.Target)
	if err != nil {
		return err
	}
	defer dstCleanup()

	reader, writer := io.Pipe()
	hasher := sha256.New()
	dump := exec.CommandContext(ctx, srcCLI.Path, append(srcCLI.baseArgs(), "exec", "-n", src.Namespace, podOf(src), "--", "cat", archive.Dir+"/dump.pgc")...)
	dump.Stdout = io.MultiWriter(writer, hasher)
	var dumpErr bytes.Buffer
	dump.Stderr = &dumpErr
	restoreArgs := append(dstCLI.baseArgs(), "exec", "-i", "-n", dst.Namespace, podOf(dst), "--", "sh", "-c",
		`exec pg_restore -U "$1" -d "$2" --clean --if-exists --no-owner --no-privileges --exit-on-error`, "sh", dst.Username, dst.Database)
	restore := exec.CommandContext(ctx, dstCLI.Path, restoreArgs...)
	restore.Stdin = reader
	// Restore diagnostics may quote rows; they are discarded, never surfaced.
	restore.Stdout, restore.Stderr = io.Discard, io.Discard

	if err := restore.Start(); err != nil {
		return fmt.Errorf("kubernetes: the PostgreSQL restore could not start")
	}
	dumpDone := make(chan error, 1)
	go func() {
		err := dump.Run()
		_ = writer.CloseWithError(err)
		dumpDone <- err
	}()
	restoreErr := restore.Wait()
	_ = reader.Close()
	if dumpResult := <-dumpDone; dumpResult != nil {
		return fmt.Errorf("kubernetes: the backup archive could not be read")
	}
	if restoreErr != nil {
		return fmt.Errorf("kubernetes: the PostgreSQL restore failed")
	}
	if hex.EncodeToString(hasher.Sum(nil)) != archive.SHA256 {
		return fmt.Errorf("kubernetes: the streamed archive does not match its recorded hash")
	}
	return nil
}

// HasPublicRoute reports whether the Environment-owned Ingress exists.
func (t *Transition) HasPublicRoute(ctx context.Context, target execution.Target, applicationID, environmentID string) (bool, error) {
	cli, cleanup, err := t.open(ctx, target)
	if err != nil {
		return false, err
	}
	defer cleanup()
	object, found, err := cli.Get(ctx, target.Namespace, "ingress", publicIngressName)
	if err != nil || !found {
		return false, err
	}
	metadata, _ := object["metadata"].(map[string]any)
	labels, _ := metadata["labels"].(map[string]any)
	return labels["app.kubernetes.io/managed-by"] == "orchestrator" && labels["orchestrator.io/application"] == applicationID && labels["orchestrator.io/environment"] == environmentID, nil
}

// DeleteOwnedNamespace deletes a namespace only when its labels show it was
// created for exactly this Application Environment by the orchestrator.
func (t *Transition) DeleteOwnedNamespace(ctx context.Context, target execution.Target, namespace, applicationID, environmentID string) error {
	if !dnsLabel.MatchString(namespace) || namespace == "default" || strings.HasPrefix(namespace, "kube-") {
		return fmt.Errorf("kubernetes: refusing to delete a protected or invalid namespace")
	}
	cli, cleanup, err := t.open(ctx, target)
	if err != nil {
		return err
	}
	defer cleanup()
	object, found, err := cli.Get(ctx, "", "namespace", namespace)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	metadata, _ := object["metadata"].(map[string]any)
	labels, _ := metadata["labels"].(map[string]any)
	if labels["app.kubernetes.io/managed-by"] != "orchestrator" || labels["orchestrator.io/application"] != applicationID || labels["orchestrator.io/environment"] != environmentID {
		return fmt.Errorf("kubernetes: the namespace is not owned by this Environment")
	}
	_, err = cli.Run(ctx, nil, "delete", "namespace", namespace, "--wait=true", "--timeout=300s")
	return err
}

var (
	_ execution.TargetProbe      = (*Transition)(nil)
	_ execution.WorkloadScaler   = (*Transition)(nil)
	_ execution.PostgresTransfer = (*Transition)(nil)
	_ execution.RouteInspector   = (*Transition)(nil)
	_ execution.NamespaceCleaner = (*Transition)(nil)
)
