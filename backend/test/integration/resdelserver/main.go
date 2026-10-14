// Command resdelserver is a test-only launcher of the orchestrator for the
// I04-08 resource detail / confirmed deletion browser scenario. It wires the
// PUBLIC product adapters that matter to the scenario: the PostgreSQL Console
// and State stores, the Vault KV v2 Connection credential store, the Web
// Console bundle, the encrypted Terraform state backend and the REAL tfexec
// cleanup engine driving the pinned terraform binary.
//
// What is FAKE (and named so): the resources are terraform_data objects of one
// hand-built module in a local state (no provider, no cloud, no cluster), the
// provider binder and the readiness check bind nothing, and the topology is
// arranged by this launcher ("seed") because the scenario does not deploy
// Workloads. A passing scenario is NOT live proof for V2-A01/A02/A03.
//
// "resdelserver provision" and "resdelserver drop" create and remove one
// disposable database with its roles on a PostgreSQL server the task owns.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"

	"orchestrator/internal/application/tfexec"
	"orchestrator/internal/bootstrap"
	"orchestrator/internal/platform/ids"
	"orchestrator/internal/seed"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "provision":
			os.Exit(provision(os.Args[2:]))
		case "drop":
			os.Exit(drop(os.Args[2:]))
		case "touch-environment":
			os.Exit(touchEnvironment(os.Args[2:]))
		}
	}
	os.Exit(serve(os.Args[1:]))
}

// lateEngine lets the bootstrap take the engine before the state backend it
// depends on exists; the launcher sets the real service right after the build.
type lateEngine struct {
	svc atomic.Pointer[tfexec.Service]
}

func (l *lateEngine) RunCleanup(ctx context.Context, org string, source tfexec.CleanupSource, report tfexec.StageReporter) (tfexec.CleanupResult, error) {
	svc := l.svc.Load()
	if svc == nil {
		return tfexec.CleanupResult{}, tfexec.ErrNoClaim
	}
	return svc.RunCleanup(ctx, org, source, report)
}

func serve(args []string) int {
	fs := flag.NewFlagSet("resdelserver", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:0", "listen address")
	addrFile := fs.String("addr-file", "", "file that receives the resolved listen address (written after seeding)")
	uiDir := fs.String("ui-dir", "../frontend/dist", "Web Console production bundle")
	databaseURLFile := fs.String("database-url-file", "", "owner-only file with the PostgreSQL Console login URL")
	stateURLFile := fs.String("state-database-url-file", "", "owner-only file with the PostgreSQL State login URL")
	stateKeyFile := fs.String("terraform-state-key-file", "", "owner-only file with the 32-byte state key")
	stateKeyID := fs.String("terraform-state-key-id", "key-1", "state key id")
	vaultAddress := fs.String("connection-vault-address", "", "Vault address of the Connection credential store")
	vaultTokenFile := fs.String("connection-vault-token-file", "", "scoped Vault token file")
	vaultMount := fs.String("connection-vault-mount", "secret", "KV v2 mount")
	terraformPath := fs.String("terraform", "", "path of the pinned terraform 1.9.8 binary")
	workDir := fs.String("work-dir", "", "absolute private directory for artifacts and encrypted recovery files")
	profile := fs.String("profile", "test", "seed profile")
	runID := fs.String("run-id", "", "run id")
	doSeed := fs.Bool("seed", false, "arrange the fixture topology (first start only)")
	scenarioFile := fs.String("scenario-file", "", "file that receives the fixture IDs when seeding")
	_ = fs.Parse(args)
	for name, value := range map[string]string{"-database-url-file": *databaseURLFile, "-state-database-url-file": *stateURLFile, "-terraform-state-key-file": *stateKeyFile, "-connection-vault-address": *vaultAddress,
		"-connection-vault-token-file": *vaultTokenFile, "-terraform": *terraformPath, "-work-dir": *workDir} {
		if value == "" {
			log.Printf("resdelserver: %s is required", name)
			return 2
		}
	}
	if !filepath.IsAbs(*workDir) || !filepath.IsAbs(*terraformPath) {
		log.Print("resdelserver: -work-dir and -terraform must be absolute paths")
		return 2
	}
	consoleURL, err := bootstrap.ReadCredentialFile("Console database URL", *databaseURLFile)
	if err != nil {
		log.Printf("resdelserver: %v", err)
		return 1
	}
	stateURL, err := bootstrap.ReadCredentialFile("State database URL", *stateURLFile)
	if err != nil {
		log.Printf("resdelserver: %v", err)
		return 1
	}
	opts := seed.Defaults()
	opts.Profile, opts.RunID = *profile, *runID
	late := &lateEngine{}
	app, err := bootstrap.Build(context.Background(), bootstrap.Options{
		Seed: opts, UIDir: *uiDir, Adapters: bootstrap.AdapterFake, DatabaseURL: consoleURL,
		TerraformStateDatabaseURL: stateURL, TerraformStateKeyFile: *stateKeyFile, TerraformStateKeyID: *stateKeyID,
		ConnectionCredentialStore: "vault", ConnectionVaultAddress: *vaultAddress, ConnectionVaultTokenFile: *vaultTokenFile, ConnectionVaultMount: *vaultMount,
		CleanupEngineOverride: late,
	})
	if err != nil {
		log.Printf("resdelserver: startup failed: %v", err)
		return 1
	}
	defer app.Close()

	ctx := context.Background()
	bundleID, err := ensureBundle(ctx, consoleURL, *doSeed)
	if err != nil {
		log.Printf("resdelserver: %v", err)
		return 1
	}
	eng, err := buildEngine(ctx, app.TerraformStates, app.TerraformStateHandler, stateURL, *stateKeyFile, *stateKeyID, *terraformPath, *workDir, bundleID)
	if err != nil {
		log.Printf("resdelserver: engine: %v", err)
		return 1
	}
	defer eng.close()
	late.svc.Store(eng.Service)
	app.ResourceCleanup.SetState(eng.state)

	if *doSeed {
		if err := seedTopology(ctx, app, eng, *scenarioFile); err != nil {
			log.Printf("resdelserver: seed failed: %v", err)
			return 1
		}
	}

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Printf("resdelserver: listen: %v", err)
		return 1
	}
	if *addrFile != "" {
		if err := os.WriteFile(*addrFile, []byte(listener.Addr().String()), 0o600); err != nil {
			log.Printf("resdelserver: write addr file: %v", err)
			return 1
		}
	}
	fmt.Printf("resdelserver listening on http://%s (REAL tfexec cleanup engine and state backend over a FAKE module/provider; PostgreSQL, Vault)\n", listener.Addr())
	server := &http.Server{Handler: app.Server, ReadHeaderTimeout: 10 * time.Second}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	status := 0
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("resdelserver: %v", err)
		status = 1
	}
	// An admitted cleanup finishes (or holds) before the process goes away.
	app.ResourceCleanup.Wait()
	return status
}

// ensureBundle registers the one fixture module bundle on the first start and
// reads its ID back on later starts. The Console login may insert bundles; it
// cannot change or delete them.
func ensureBundle(ctx context.Context, consoleURL string, first bool) (string, error) {
	conn, err := pgx.Connect(ctx, consoleURL)
	if err != nil {
		return "", errors.New("cannot connect to the Console database")
	}
	defer conn.Close(ctx)
	var id string
	err = conn.QueryRow(ctx, `SELECT id::text FROM terraform_module_bundles WHERE module_id='fixture-object' AND source_id='test'`).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !first || !errors.Is(err, pgx.ErrNoRows) {
		return "", errors.New("the fixture module bundle is missing")
	}
	id = ids.New()
	_, err = conn.Exec(ctx, `INSERT INTO terraform_module_bundles(id,source_id,module_id,revision,subdirectory,content_sha256,terraform_version,terraform_binary_sha256,provider_lock_digest,contract,provider_requirements,status,created_at,updated_at) VALUES($1::uuid,'test','fixture-object','rev1','root',repeat('a',64),'1.9.8',repeat('b',64),repeat('c',64),'{}','{}','VERIFIED',now(),now())`, id)
	if err != nil {
		return "", errors.New("cannot register the fixture module bundle")
	}
	return id, nil
}

// touchEnvironment is the scenario's administrator action that makes an open
// deletion preview stale: it bumps the draft version of one Environment, the way
// a concurrent Workload change would, as the database OWNER (the product logins
// cannot do it). Nothing else is written.
func touchEnvironment(args []string) int {
	fs := flag.NewFlagSet("touch-environment", flag.ExitOnError)
	privateDir := fs.String("private-dir", "", "0700 directory holding owner.url")
	application := fs.String("application", "", "application key")
	environment := fs.String("environment", "staging", "environment key")
	_ = fs.Parse(args)
	if *privateDir == "" || *application == "" {
		log.Print("resdelserver touch-environment: -private-dir and -application are required")
		return 2
	}
	ownerURL, err := bootstrap.ReadCredentialFile("owner database URL", filepath.Join(*privateDir, "owner.url"))
	if err != nil {
		log.Printf("resdelserver touch-environment: %v", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, ownerURL)
	if err != nil {
		log.Print("resdelserver touch-environment: cannot connect to the owner database")
		return 1
	}
	defer conn.Close(ctx)
	tag, err := conn.Exec(ctx, `UPDATE environments e SET draft_version=draft_version+1 FROM applications a WHERE e.application_id=a.id AND a.application_key=$1 AND e.environment_key=$2`, *application, *environment)
	if err != nil || tag.RowsAffected() != 1 {
		log.Print("resdelserver touch-environment: the Environment draft version was not bumped")
		return 1
	}
	fmt.Println("environment draft version bumped (stale preview injection)")
	return 0
}
