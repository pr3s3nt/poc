package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"orchestrator/internal/adapters/terraform/registry"
	"orchestrator/internal/bootstrap"
	"orchestrator/test/integration/v2run"
)

func init() {
	extra["verify-binaries"] = verifyBinaries
	extra["k8s-preflight"] = k8sPreflight
	extra["pg-provision"] = pgProvision
	extra["pg-drop"] = pgDrop
	extra["keygen"] = keygen
	extra["module-pins"] = modulePins
	extra["assertion"] = appendAssertion
	extra["aws-probes-clean"] = awsProbesClean
	extra["aws-ecr-prepare"] = awsECRPrepare
	extra["aws-ecr-clean"] = awsECRClean
}

func awsECRPrepare(args []string) int {
	fs := flag.NewFlagSet("aws-ecr-prepare", flag.ExitOnError)
	cfgPath, manifestPath, outPath := fs.String("config", "", ""), fs.String("manifest", "", ""), fs.String("out", "", "private publication result")
	_ = fs.Parse(args)
	c, code := awsConfigOnly(*cfgPath)
	if code != 0 {
		return code
	}
	aws, err := v2run.NewExecAWS(c)
	if err != nil {
		return fail("AWS runner is unavailable")
	}
	m, err := v2run.LoadManifest(*manifestPath)
	if err != nil {
		return fail("the private manifest cannot be read")
	}
	deadline := time.Until(m.ExpiresAt)
	if deadline <= 0 {
		return fail("the recorded execution lease has expired")
	}
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	dockerHome := filepath.Join(filepath.Dir(*manifestPath), "docker-config")
	if err := os.MkdirAll(dockerHome, 0o700); err != nil {
		return fail("the private Docker config directory cannot be created")
	}
	pub, err := v2run.PrepareECRImages(ctx, aws, v2run.ExecDocker{Binary: c.Binaries["docker"].Path, ConfigDir: dockerHome}, c, *manifestPath)
	if err != nil {
		return fail("ECR image preparation failed; the independent supervisor owns exact cleanup")
	}
	raw, err := json.Marshal(pub)
	if err != nil || v2run.WriteFilePrivate(*outPath, raw) != nil {
		return fail("the private ECR image publication result could not be recorded")
	}
	fmt.Println("three run-owned ECR repositories and immutable image digests verified")
	return 0
}

func awsECRClean(args []string) int {
	fs := flag.NewFlagSet("aws-ecr-clean", flag.ExitOnError)
	cfgPath, manifestPath := fs.String("config", "", ""), fs.String("manifest", "", "")
	verifyOnly := fs.Bool("verify", false, "verify absence without deleting")
	_ = fs.Parse(args)
	c, cli, code := awsRunner(*cfgPath)
	if code != 0 {
		return code
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	if err := v2run.CleanupECRRepositories(ctx, cli, c, *manifestPath, *verifyOnly); err != nil {
		return fail("ECR repository cleanup is held: %v", err)
	}
	fmt.Println("exact run-owned ECR repositories verified absent")
	return 0
}

// awsProbesClean removes only the exact technical Pods recorded in the run
// manifest. It is safe to rerun after a runner or browser process-group kill.
func awsProbesClean(args []string) int {
	fs := flag.NewFlagSet("aws-probes-clean", flag.ExitOnError)
	cfgPath, manifestPath := fs.String("config", "", ""), fs.String("manifest", "", "")
	verifyOnly := fs.Bool("verify", false, "verify absence without deleting")
	_ = fs.Parse(args)
	c, err := loadCfg(*cfgPath)
	if err != nil || c.Target != v2run.TargetAWS || c.AWS == nil {
		return fail("not an AWS run")
	}
	m, err := v2run.LoadManifest(*manifestPath)
	if err != nil {
		return fail("the private run manifest cannot be read")
	}
	var cluster string
	expected := map[string]bool{}
	for _, a := range m.Assets {
		if a.Kind == "eks-cluster" && a.Owner == v2run.OwnerCreatedByRun {
			cluster = a.ID
		}
		if a.Kind == "pod" && a.Owner == v2run.OwnerCreatedByRun {
			parts := strings.Split(a.ID, "/")
			if len(parts) != 2 || parts[0] != m.Namespace || parts[1] == "" || m.Namespace == "" {
				return fail("a recorded technical Pod has an invalid exact namespace/name")
			}
			expected[parts[1]] = true
		}
	}
	if len(expected) == 0 {
		fmt.Println("no recorded technical Pods; none to remove")
		return 0
	}
	if cluster == "" {
		return fail("technical Pods are recorded but the run-owned EKS cluster is not recorded")
	}
	kubeconfig := filepath.Join(filepath.Dir(*manifestPath), "eks.kubeconfig")
	if _, err := os.Stat(kubeconfig); errors.Is(err, os.ErrNotExist) {
		// The supervisor can run before EKS has been created. In that case no
		// probe Pod could have been created; a missing kubeconfig is indeterminate.
		return fail("the EKS kubeconfig is missing while technical Pods are recorded")
	}
	kubectl := c.Binaries["kubectl"].Path
	if !filepath.IsAbs(kubectl) {
		return fail("the pinned kubectl path is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	run := func(args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, kubectl, append([]string{"--kubeconfig", kubeconfig, "--context", "eks-run"}, args...)...)
		cmd.Env = append(os.Environ(), "AWS_REGION="+c.AWS.Region, "AWS_DEFAULT_REGION="+c.AWS.Region)
		var out, stderr strings.Builder
		cmd.Stdout, cmd.Stderr = &out, &stderr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("kubectl operation failed: %s", strings.TrimSpace(stderr.String()))
		}
		return []byte(out.String()), nil
	}
	selector := "run-id=" + m.RunID + ",managed-by=orchestrator-verification,app.kubernetes.io/component=technical-probe"
	list, err := run("-n", m.Namespace, "get", "pods", "-l", selector, "-o", "json")
	if err != nil {
		return fail("the run-labelled technical Pod list is indeterminate")
	}
	var pods probePodList
	if json.Unmarshal(list, &pods) != nil {
		return fail("the run-labelled technical Pod list is unreadable")
	}
	for _, pod := range pods.Items {
		if !expected[pod.Metadata.Name] || pod.Metadata.Labels["run-id"] != m.RunID || pod.Metadata.Labels["managed-by"] != v2run.ProductManagedByTag || pod.Metadata.Labels["app.kubernetes.io/component"] != "technical-probe" {
			return fail("an unrecorded or differently labelled technical Pod was found; cleanup is held")
		}
	}
	if !*verifyOnly {
		for name := range expected {
			if !probeListed(pods.Items, name) {
				continue
			}
			if _, err := run("-n", m.Namespace, "delete", "pod", name, "--wait=true", "--timeout=2m"); err != nil {
				return fail("an exact recorded technical Pod could not be removed")
			}
		}
	}
	remainingRaw, err := run("-n", m.Namespace, "get", "pods", "-l", selector, "-o", "json")
	if err != nil {
		return fail("technical Pod absence could not be verified")
	}
	if json.Unmarshal(remainingRaw, &pods) != nil || len(pods.Items) != 0 {
		return fail("run-labelled technical Pods remain")
	}
	for name := range expected {
		if _, err := run("-n", m.Namespace, "get", "pod", name, "-o", "name"); err == nil {
			return fail("an exact recorded technical Pod remains")
		} else if !strings.Contains(strings.ToLower(err.Error()), "notfound") {
			return fail("the exact recorded technical Pod absence is indeterminate")
		}
	}
	fmt.Println("exact recorded technical Pods verified absent")
	return 0
}

type probePodList struct {
	Items []struct {
		Metadata struct {
			Name   string            `json:"name"`
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
	} `json:"items"`
}

func probeListed(items []struct {
	Metadata struct {
		Name   string            `json:"name"`
		Labels map[string]string `json:"labels"`
	} `json:"metadata"`
}, name string) bool {
	for _, item := range items {
		if item.Metadata.Name == name {
			return true
		}
	}
	return false
}

func printChecks(checks []v2run.Check) int {
	out, _ := json.MarshalIndent(checks, "", "  ")
	fmt.Println(string(out))
	if !v2run.AllOK(checks) {
		return fail("preflight failed; no resource is created")
	}
	return 0
}

func verifyBinaries(args []string) int {
	fs := flag.NewFlagSet("verify-binaries", flag.ExitOnError)
	cfgPath := fs.String("config", "", "")
	_ = fs.Parse(args)
	c, err := loadCfg(*cfgPath)
	if err != nil {
		return fail("%v", err)
	}
	return printChecks(v2run.VerifyBinaries(context.Background(), c))
}

func k8sPreflight(args []string) int {
	fs := flag.NewFlagSet("k8s-preflight", flag.ExitOnError)
	cfgPath := fs.String("config", "", "")
	_ = fs.Parse(args)
	c, err := loadCfg(*cfgPath)
	if err != nil || c.Kube == nil {
		return fail("a k8s-4f configuration is required")
	}
	k := v2run.ExecKubectl{Binary: c.Binaries["kubectl"].Path, Kubeconfig: c.Kube.Kubeconfig, Context: c.Kube.Context}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return printChecks(v2run.PreflightK8s(ctx, k, c))
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// pgManifest names exactly what pg-provision created so pg-drop removes only it.
type pgManifest struct {
	Database string   `json:"database"`
	Roles    []string `json:"roles"`
}

const pgPrefix = "i0410_"

// pgProvision creates one fresh database with an owner and two separate
// runtime login roles (Console, State), each with its own random password,
// on the DISPOSABLE server of -admin-url-file. It writes owner.url,
// console.url and state.url (0600) and pg-manifest.json into -out-dir. The
// baseline and the grants are applied by the product's explicit
// `-setup-database` owner step, not here.
func pgProvision(args []string) int {
	fs := flag.NewFlagSet("pg-provision", flag.ExitOnError)
	adminFile, outDir := fs.String("admin-url-file", "", ""), fs.String("out-dir", "", "")
	_ = fs.Parse(args)
	admin, err := bootstrap.ReadCredentialFile("administrator URL", *adminFile)
	if err != nil || *outDir == "" {
		return fail("-admin-url-file and -out-dir are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		return fail("cannot connect to the administrator URL")
	}
	defer conn.Close(ctx)
	suffix, _ := randomHex(6)
	m := pgManifest{Database: pgPrefix + suffix}
	passwords := map[string]string{}
	for _, kind := range []string{"owner", "console", "state"} {
		role := pgPrefix + suffix + "_" + kind
		pw, err := randomHex(16)
		if err != nil {
			return fail("random")
		}
		if _, err := conn.Exec(ctx, `CREATE ROLE `+pgx.Identifier{role}.Sanitize()+` LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD '`+pw+`'`); err != nil {
			return fail("create role failed")
		}
		m.Roles = append(m.Roles, role)
		passwords[kind] = pw
	}
	if _, err := conn.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{m.Database}.Sanitize()+` OWNER `+pgx.Identifier{m.Roles[0]}.Sanitize()); err != nil {
		return fail("create database failed")
	}
	parsed, err := url.Parse(admin)
	if err != nil {
		return fail("administrator URL")
	}
	parsed.Path = "/" + m.Database
	write := func(name, role, pw string) error {
		u := *parsed
		u.User = url.UserPassword(role, pw)
		return v2run.WriteFilePrivate(filepath.Join(*outDir, name), []byte(u.String()))
	}
	for i, kind := range []string{"owner", "console", "state"} {
		if err := write(kind+".url", m.Roles[i], passwords[kind]); err != nil {
			return fail("write %s.url", kind)
		}
	}
	raw, _ := json.Marshal(m)
	if err := v2run.WriteFilePrivate(filepath.Join(*outDir, "pg-manifest.json"), raw); err != nil {
		return fail("write the manifest")
	}
	fmt.Printf("provisioned database %s with %d roles (baseline applied by the product setup step)\n", m.Database, len(m.Roles))
	return 0
}

func pgDrop(args []string) int {
	fs := flag.NewFlagSet("pg-drop", flag.ExitOnError)
	adminFile, manifestFile := fs.String("admin-url-file", "", ""), fs.String("manifest", "", "")
	_ = fs.Parse(args)
	admin, err := bootstrap.ReadCredentialFile("administrator URL", *adminFile)
	raw, readErr := os.ReadFile(*manifestFile)
	var m pgManifest
	if err != nil || readErr != nil || json.Unmarshal(raw, &m) != nil || !strings.HasPrefix(m.Database, pgPrefix) {
		return fail("a valid -admin-url-file and -manifest are required")
	}
	for _, role := range m.Roles {
		if !strings.HasPrefix(role, pgPrefix) {
			return fail("refusing a role that provision did not create")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		return fail("cannot connect to the administrator URL")
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `DROP DATABASE IF EXISTS `+pgx.Identifier{m.Database}.Sanitize()+` WITH (FORCE)`); err != nil {
		return fail("drop database failed")
	}
	for i := len(m.Roles) - 1; i >= 0; i-- {
		if _, err := conn.Exec(ctx, `DROP ROLE IF EXISTS `+pgx.Identifier{m.Roles[i]}.Sanitize()); err != nil {
			return fail("drop role failed")
		}
	}
	fmt.Printf("dropped database %s and %d roles\n", m.Database, len(m.Roles))
	return 0
}

// keygen writes a fresh 32-byte hex AES-256-GCM key (0600) for the encrypted
// Terraform state; the key lives outside SQL.
func keygen(args []string) int {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	out := fs.String("out", "", "")
	_ = fs.Parse(args)
	key, err := randomHex(32)
	if err != nil || *out == "" {
		return fail("-out is required")
	}
	if _, err := os.Lstat(*out); err == nil {
		return fail("refusing to overwrite an existing key file")
	}
	if err := v2run.WriteFilePrivate(*out, []byte(key+"\n")); err != nil {
		return fail("write the key file")
	}
	return 0
}

// modulePins prints the packaged Kubernetes module pins (id, revision,
// content SHA-256), the same values the Console shows and Preview must match.
func modulePins(args []string) int {
	fs := flag.NewFlagSet("module-pins", flag.ExitOnError)
	_ = fs.Parse(args)
	refs, err := registry.PackagedRefs()
	if err != nil {
		return fail("%v", err)
	}
	for _, id := range []string{"k8s-namespace", "postgres-k8s", "redis-k8s"} {
		ref, ok := refs[id]
		if !ok {
			return fail("module %s is not part of the packaged release", id)
		}
		fmt.Printf("%s\t%s\t%s\n", id, ref.Revision, ref.ContentSHA256)
	}
	return 0
}

// appendAssertion adds one private-read assertion to assertions.json.
func appendAssertion(args []string) int {
	fs := flag.NewFlagSet("assertion", flag.ExitOnError)
	file, id, phase, source, detail := fs.String("file", "", ""), fs.String("id", "", ""), fs.String("phase", "", ""), fs.String("source", "private-read", ""), fs.String("detail", "", "")
	pass := fs.Bool("pass", false, "")
	_ = fs.Parse(args)
	var list []v2run.Assertion
	if raw, err := os.ReadFile(*file); err == nil {
		_ = json.Unmarshal(raw, &list)
	}
	list = append(list, v2run.Assertion{ID: *id, Phase: *phase, Pass: *pass, Source: *source, Detail: *detail})
	raw, _ := json.MarshalIndent(list, "", "  ")
	if err := os.WriteFile(*file, raw, 0o644); err != nil {
		return fail("%v", err)
	}
	return 0
}

func init() {
	extra["config-get"] = configGet
	extra["pg-role"] = pgRole
	extra["manifest-public"] = manifestPublic
}

// configGet prints one non-secret field of the private configuration for the
// runner shell (paths and names only; never a credential value).
func configGet(args []string) int {
	fs := flag.NewFlagSet("config-get", flag.ExitOnError)
	cfgPath, field := fs.String("config", "", ""), fs.String("field", "", "")
	_ = fs.Parse(args)
	c, err := loadCfg(*cfgPath)
	if err != nil {
		return fail("%v", err)
	}
	pinnedImage := func(img v2run.Image) string {
		if strings.Contains(img.Ref, "@") {
			return img.Ref
		}
		return img.Ref + "@" + img.Digest
	}
	image := func(w string) string { return pinnedImage(c.Images[c.NodeArch][w]) }
	values := map[string]func() string{
		"runId": func() string { return c.RunID }, "scenario": func() string { return c.Scenario }, "target": func() string { return c.Target },
		"owner": func() string { return c.Owner }, "evidenceDir": func() string { return c.EvidenceDir }, "nodeArch": func() string { return c.NodeArch },
		"executionSeconds": func() string { return fmt.Sprint(int(c.Execution.Duration.Seconds())) },
		"cleanupSeconds":   func() string { return fmt.Sprint(int(c.Cleanup.Duration.Seconds())) },
		"terraform":        func() string { return c.Binaries["terraform"].Path }, "scoreK8s": func() string { return c.Binaries["score-k8s"].Path },
		"kubectl": func() string { return c.Binaries["kubectl"].Path }, "vaultBin": func() string { return c.VaultBin },
		"vaultDelivery": func() string {
			if c.Target == v2run.TargetK8s4f {
				return "vso"
			}
			return "agent"
		},
		"pgAdminUrlFile": func() string { return c.PostgresAdminURLFile },
		"imageFrontend":  func() string { return image("frontend") }, "imageBackend": func() string { return image("backend") }, "imageWorker": func() string { return image("worker") },
		"kubeconfig":       func() string { return c.Kube.Kubeconfig },
		"kubeContext":      func() string { return c.Kube.Context },
		"namespace":        func() string { return c.Kube.Namespace },
		"applicationKey":   func() string { return c.ApplicationKey() },
		"awsBinary":        func() string { return c.Binaries["aws"].Path },
		"awsRegion":        func() string { return c.AWS.Region },
		"awsAccountId":     func() string { return c.AWS.AccountID },
		"awsProbePostgres": func() string { return pinnedImage(c.AWS.ProbePostgresImage) },
		"awsProbeRedis":    func() string { return pinnedImage(c.AWS.ProbeRedisImage) },
		"awsEstimateFile": func() string {
			if c.AWS == nil {
				return ""
			}
			return c.AWS.EstimateFile
		},
		"awsAccessKeyFile": func() string {
			if c.AWS == nil {
				return ""
			}
			return c.AWS.AccessKeyFile
		},
		"secretStoreJson": func() string {
			if c.SecretStore == nil {
				return "null"
			}
			raw, _ := json.Marshal(c.SecretStore)
			return string(raw)
		},
		"secretStoreTokenFile": func() string {
			if c.SecretStore == nil {
				return ""
			}
			return c.SecretStore.TokenFile
		},
		"sentinelName": func() string {
			s := c.Kube.Sentinel
			return s.Namespace + "/" + s.Kind + "/" + s.Name
		},
	}
	get, ok := values[*field]
	needsKube := *field == "kubeconfig" || *field == "kubeContext" || *field == "namespace" || *field == "sentinelName"
	needsAWS := strings.HasPrefix(*field, "aws")
	if !ok || needsKube && c.Kube == nil || needsAWS && c.AWS == nil {
		return fail("unknown or unavailable field %q", *field)
	}
	fmt.Println(get())
	return 0
}

func pgRole(args []string) int {
	fs := flag.NewFlagSet("pg-role", flag.ExitOnError)
	file, kind := fs.String("manifest", "", ""), fs.String("kind", "", "")
	_ = fs.Parse(args)
	raw, err := os.ReadFile(*file)
	var m pgManifest
	if err != nil || json.Unmarshal(raw, &m) != nil || len(m.Roles) != 3 {
		return fail("invalid pg manifest")
	}
	idx := map[string]int{"owner": 0, "console": 1, "state": 2}
	i, ok := idx[*kind]
	if !ok {
		return fail("kind must be owner, console or state")
	}
	fmt.Println(m.Roles[i])
	return 0
}

// manifestPublic writes the evidence-safe copy of the manifest: identity,
// tags, pins, planned counts, namespace and asset kinds/IDs; no handle.
func manifestPublic(args []string) int {
	fs := flag.NewFlagSet("manifest-public", flag.ExitOnError)
	in, out := fs.String("manifest", "", ""), fs.String("out", "", "")
	_ = fs.Parse(args)
	m, err := v2run.LoadManifest(*in)
	if err != nil {
		return fail("%v", err)
	}
	raw, _ := json.MarshalIndent(m.Public(), "", "  ")
	if err := os.WriteFile(*out, raw, 0o644); err != nil {
		return fail("%v", err)
	}
	return 0
}

func init() { extra["run-marker"] = runMarker }

// runMarker writes or advances the immutable run marker of an evidence
// directory. STARTED is written before the first create; CLOSED_NO_EFFECTS is
// an explicit operator statement that the run created nothing.
func runMarker(args []string) int {
	fs := flag.NewFlagSet("run-marker", flag.ExitOnError)
	dir, run, target := fs.String("dir", "", "run evidence directory"), fs.String("run", "", "run id"), fs.String("target", "", "target")
	state, reason := fs.String("state", "", "STARTED|ARMED|CLOSED_CLEAN|CLOSED_NO_EFFECTS"), fs.String("reason", "", "operator reason for a close")
	_ = fs.Parse(args)
	if *dir == "" || *run == "" {
		return fail("run-marker needs -dir and -run")
	}
	if *state == v2run.MarkerClosedNoEffects && *reason == "" {
		return fail("closing a run as having no effects needs a -reason")
	}
	if err := os.MkdirAll(*dir, 0o700); err != nil {
		return fail("%v", err)
	}
	if err := v2run.WriteRunMarker(*dir, v2run.RunMarker{RunID: *run, Target: *target, State: *state, Reason: *reason,
		Started: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		return fail("%v", err)
	}
	fmt.Println("run marker", *state)
	return 0
}

func init() { extra["handles-write"] = handlesWrite }

// handlesWrite writes the private cleanup handles (paths only, 0600) that the
// independent safety net needs to rebuild the product wiring after the runner,
// browser and product server were killed.
func handlesWrite(args []string) int {
	fs := flag.NewFlagSet("handles-write", flag.ExitOnError)
	cfgPath, private, vaultAddr := fs.String("config", "", ""), fs.String("private", "", "private run directory"), fs.String("vault-address", "", "run-owned Vault address")
	inv, rdy := fs.Duration("invocation-timeout", 15*time.Minute, ""), fs.Duration("readiness-timeout", 5*time.Minute, "")
	_ = fs.Parse(args)
	c, err := loadCfg(*cfgPath)
	if err != nil {
		return fail("%v", err)
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return fail("git is required")
	}
	if git, err = filepath.Abs(git); err != nil {
		return fail("%v", err)
	}
	namespace, adapters := NamespaceFor(c)
	p := func(name string) string { return filepath.Join(*private, name) }
	h := v2run.CleanupHandles{
		RunID: c.RunID, OrganizationKey: "acme", ApplicationKey: c.ApplicationKey(), EnvironmentKey: "staging",
		NamespaceIdentity: namespace, Adapters: adapters, Owner: c.Owner,
		Terraform: c.Binaries["terraform"].Path, Kubectl: c.Binaries["kubectl"].Path, ScoreK8s: c.Binaries["score-k8s"].Path, Git: git,
		DatabaseURLFile: p("console.url"), StateDatabaseURLFile: p("state.url"), StateKeyFile: p("state.key"), StateKeyID: "v2-" + c.RunID,
		RecoveryDir: p("recovery"), TerraformRoot: p("tfroot"), PluginCacheDir: p("plugin-cache"), ModuleCacheDir: p("module-cache"),
		VaultAddress: *vaultAddr, VaultMount: "secret", ConnectionTokenFile: p("connection-vault-token"), OutputTokenFile: p("output-vault-token"),
		CleanupBudget: c.Cleanup, InvocationTimeout: v2run.Duration{Duration: *inv}, ReadinessTimeout: v2run.Duration{Duration: *rdy},
	}
	if err := h.Validate(); err != nil {
		return fail("%v", err)
	}
	raw, err := json.Marshal(h)
	if err != nil {
		return fail("%v", err)
	}
	if err := v2run.WriteFilePrivate(p("cleanup-handles.json"), raw); err != nil {
		return fail("%v", err)
	}
	fmt.Println("cleanup handles written")
	return 0
}

func init() {
	extra["vault-snapshot"] = vaultSnapshot
	extra["vault-clean"] = vaultClean
}

func vaultAccess(cfgPath string) (v2run.Config, v2run.VaultAccess, int) {
	c, err := loadCfg(cfgPath)
	if err != nil {
		return c, v2run.VaultAccess{}, fail("%v", err)
	}
	if c.SecretStore == nil {
		return c, v2run.VaultAccess{}, fail("no secret store is configured")
	}
	v, err := v2run.NewVaultAccess(*c.SecretStore)
	if err != nil {
		return c, v, fail("%v", err)
	}
	return c, v, 0
}

// vaultSnapshot records the policies and roles of the configuration Vault that
// exist BEFORE the run (read-only), so only objects created by the run are ever
// cleanup candidates.
func vaultSnapshot(args []string) int {
	fs := flag.NewFlagSet("vault-snapshot", flag.ExitOnError)
	cfgPath, out := fs.String("config", "", ""), fs.String("out", "", "snapshot file (0600)")
	_ = fs.Parse(args)
	_, v, code := vaultAccess(*cfgPath)
	if code != 0 {
		return code
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	snap, err := v2run.TakeVaultSnapshot(ctx, v)
	if err != nil {
		return fail("%v", err)
	}
	raw, _ := json.Marshal(snap)
	if err := v2run.WriteFilePrivate(*out, raw); err != nil {
		return fail("%v", err)
	}
	fmt.Printf("vault snapshot: %d policies, %d roles existed before the run\n", len(snap.Policies), len(snap.Roles))
	return 0
}

// vaultClean deletes exactly the run-owned objects of the configuration Vault;
// with -verify it only proves that none remain. Foreign new objects are
// reported, never deleted, and make the verification fail.
func vaultClean(args []string) int {
	fs := flag.NewFlagSet("vault-clean", flag.ExitOnError)
	cfgPath, snapPath, verify := fs.String("config", "", ""), fs.String("snapshot", "", ""), fs.Bool("verify", false, "only verify")
	_ = fs.Parse(args)
	c, v, code := vaultAccess(*cfgPath)
	if code != 0 {
		return code
	}
	raw, err := os.ReadFile(*snapPath)
	if err != nil {
		return fail("the Vault snapshot is missing; nothing is deleted")
	}
	var snap v2run.VaultSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return fail("the Vault snapshot is unreadable; nothing is deleted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	owned, err := v2run.FindVaultOwned(ctx, v, snap, c.ApplicationKey(), c.Kube.Namespace)
	if err != nil {
		return fail("%v", err)
	}
	if !*verify && !owned.Empty() {
		n, err := v2run.CleanVaultOwned(ctx, v, owned)
		if err != nil {
			return fail("deleted %d run-owned Vault objects before: %v", n, err)
		}
		fmt.Printf("deleted %d run-owned Vault objects\n", n)
		if owned, err = v2run.FindVaultOwned(ctx, v, snap, c.ApplicationKey(), c.Kube.Namespace); err != nil {
			return fail("%v", err)
		}
	}
	if !owned.Empty() {
		return fail("run-owned Vault objects remain: %d values, %d policies, %d roles", len(owned.Values), len(owned.Policies), len(owned.Roles))
	}
	if len(owned.Foreign) > 0 {
		fmt.Printf("note: %d new Vault objects are not provably the run's and were left untouched\n", len(owned.Foreign))
	}
	fmt.Println("no run-owned Vault object remains")
	return 0
}

func init() { extra["manifest-run"] = manifestRun }

// manifestRun records the Terraform run ID of the run's Preview before Deploy.
func manifestRun(args []string) int {
	fs := flag.NewFlagSet("manifest-run", flag.ExitOnError)
	path, id := fs.String("manifest", "", ""), fs.String("id", "", "Preview run id (pending-<hash>)")
	_ = fs.Parse(args)
	return update(*path, func(m *v2run.Manifest) error { return m.AddTerraformRun(*id) })
}

func init() {
	extra["aws-preflight"] = awsPreflight
	extra["aws-gate"] = awsGate
	extra["aws-guard"] = awsGuard
	extra["aws-verify"] = awsVerify
}

func awsRunner(cfgPath string) (v2run.Config, v2run.ExecAWS, int) {
	c, err := loadCfg(cfgPath)
	if err != nil {
		return c, v2run.ExecAWS{}, fail("%v", err)
	}
	if c.AWS == nil {
		return c, v2run.ExecAWS{}, fail("this is not an AWS configuration")
	}
	cli, err := v2run.NewExecAWS(c)
	if err != nil {
		return c, cli, fail("%v", err)
	}
	return c, cli, 0
}

// awsPreflight runs the read-only AWS checks (identity, tags, permission
// simulation, pod capacity); it creates nothing.
func awsPreflight(args []string) int {
	fs := flag.NewFlagSet("aws-preflight", flag.ExitOnError)
	cfgPath := fs.String("config", "", "")
	_ = fs.Parse(args)
	c, cli, code := awsRunner(*cfgPath)
	if code != 0 {
		return code
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	return printChecks(v2run.PreflightAWS(ctx, cli, c))
}

// awsGate fails until I04-11 passed and was cleaned (no AWS effect before).
func awsGate(args []string) int {
	fs := flag.NewFlagSet("aws-gate", flag.ExitOnError)
	cfgPath := fs.String("config", "", "")
	_ = fs.Parse(args)
	c, err := loadCfg(*cfgPath)
	if err != nil {
		return fail("%v", err)
	}
	if err := v2run.AWSGate(c); err != nil {
		return fail("%v", err)
	}
	fmt.Println("I04-11 evidence complete: A01, E01 and E02 passed and were cleaned")
	return 0
}

// Identity mismatch is unknown and fails closed; only resource-level ownership
// guards may use the supervisor's SKIPPED_NOT_OWNED exit semantic.
func awsGuard(args []string) int {
	fs := flag.NewFlagSet("aws-guard", flag.ExitOnError)
	cfgPath := fs.String("config", "", "")
	_ = fs.Parse(args)
	c, cli, code := awsRunner(*cfgPath)
	if code != 0 {
		return code
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := v2run.AWSGuard(ctx, cli, c); err != nil {
		return fail("%v", err)
	}
	return 0
}

// awsVerify proves the absence of one layer's assets (exact IDs, exact linkage, run tag).
func awsVerify(args []string) int {
	fs := flag.NewFlagSet("aws-verify", flag.ExitOnError)
	cfgPath, mpath, layer := fs.String("config", "", ""), fs.String("manifest", "", ""), fs.String("layer", "", "")
	_ = fs.Parse(args)
	c, cli, code := awsRunner(*cfgPath)
	if code != 0 {
		return code
	}
	m, err := v2run.LoadManifest(*mpath)
	if err != nil {
		return fail("%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := v2run.VerifyAWSLayer(ctx, cli, c, m, *layer); err != nil {
		return fail("layer %s is not verified absent: %v", *layer, err)
	}
	fmt.Printf("layer %s verified absent\n", *layer)
	return 0
}

// NamespaceFor is the seeded namespace identity and adapter mode of a run.
func NamespaceFor(c v2run.Config) (string, string) {
	if c.Kube != nil {
		return c.Kube.Namespace, "kubernetes"
	}
	return v2run.NamespacePrefix + c.RunID, "aws"
}

func init() {
	extra["planned-resources"] = plannedResources
	extra["estimate-template"] = estimateTemplate
}

// plannedResources prints "<kind>\t<count>" of the scenario's shipped modules.
func plannedResources(args []string) int {
	fs := flag.NewFlagSet("planned-resources", flag.ExitOnError)
	scenario := fs.String("scenario", "", "A02 or A03")
	_ = fs.Parse(args)
	list, err := v2run.PlannedFromModules(*scenario)
	if err != nil {
		return fail("%v", err)
	}
	for _, p := range list {
		fmt.Printf("%s\t%d\n", p.Kind, p.Count)
	}
	return 0
}

// estimateTemplate writes the unpriced estimate skeleton for the operator to
// complete from public prices (no ceiling field exists).
func estimateTemplate(args []string) int {
	fs := flag.NewFlagSet("estimate-template", flag.ExitOnError)
	cfgPath, out := fs.String("config", "", ""), fs.String("out", "", "new file (0600)")
	_ = fs.Parse(args)
	c, err := loadCfg(*cfgPath)
	if err != nil {
		return fail("%v", err)
	}
	e, err := v2run.EstimateTemplate(c)
	if err != nil {
		return fail("%v", err)
	}
	raw, _ := json.MarshalIndent(e, "", "  ")
	if err := v2run.WriteFilePrivate(*out, raw); err != nil {
		return fail("%v", err)
	}
	fmt.Println("estimate skeleton written; price every line from a public AWS source before the run")
	return 0
}

func init() { extra["aws-input"] = awsInput }

// awsInput prints the AWS part of the private scenario input (0600 file by the
// caller): the key handle is read here and typed only into the masked Console
// fields by the scenario. Nothing is logged.
func awsInput(args []string) int {
	fs := flag.NewFlagSet("aws-input", flag.ExitOnError)
	cfgPath, catalog := fs.String("config", "", ""), fs.String("catalog", "", "aws-catalog.json")
	_ = fs.Parse(args)
	c, err := loadCfg(*cfgPath)
	if err != nil || c.Target != v2run.TargetAWS || c.AWS == nil {
		return fail("not an AWS configuration")
	}
	raw, err := os.ReadFile(c.AWS.AccessKeyFile)
	if err != nil {
		return fail("the AWS key handle is unreadable")
	}
	var key struct {
		AccessKeyID     string `json:"accessKeyId"`
		SecretAccessKey string `json:"secretAccessKey"`
	}
	if json.Unmarshal(raw, &key) != nil || key.AccessKeyID == "" || key.SecretAccessKey == "" {
		return fail("the AWS key handle must be JSON {accessKeyId, secretAccessKey}")
	}
	host := map[string]string{}
	for k, v := range map[string]string{"AWS_PROFILE": c.AWS.Profile, "AWS_SHARED_CREDENTIALS_FILE": c.AWS.SharedCredentialsFile, "AWS_CONFIG_FILE": c.AWS.ConfigFile} {
		if v != "" {
			host[k] = v
		}
	}
	out, _ := json.Marshal(map[string]any{
		"connectionName": "aws-" + c.RunID, "accessKeyId": key.AccessKeyID, "secretAccessKey": key.SecretAccessKey, "region": c.AWS.Region, "accountId": c.AWS.AccountID,
		"bin": c.Binaries["aws"].Path, "nodeArch": c.NodeArch,
		"probePostgresImage": c.AWS.ProbePostgresImage.Ref + "@" + c.AWS.ProbePostgresImage.Digest, "probeRedisImage": c.AWS.ProbeRedisImage.Ref + "@" + c.AWS.ProbeRedisImage.Digest, "hostEnv": host, "catalogFile": *catalog, "egressCidr": c.AWS.ExecutorEgress, "nodeInstanceType": c.AWS.NodeInstanceType,
	})
	fmt.Println(string(out))
	return 0
}

func init() { extra["timeouts"] = timeoutsCmd }

// timeoutsCmd prints the exact finite bounds of the run as key=seconds lines
// (validated nesting); the runner records them in the manifest before the first
// create and passes the same values to the product and the cleanup handles.
func timeoutsCmd(args []string) int {
	fs := flag.NewFlagSet("timeouts", flag.ExitOnError)
	cfgPath := fs.String("config", "", "")
	_ = fs.Parse(args)
	c, err := loadCfg(*cfgPath)
	if err != nil {
		return fail("%v", err)
	}
	t, err := v2run.PlanTimeouts(c)
	if err != nil {
		return fail("%v", err)
	}
	for _, kv := range []struct {
		k string
		v time.Duration
	}{{"terraformReadiness", t.TerraformReadiness}, {"terraformInvocation", t.TerraformInvocation}, {"deployLease", t.DeployLease}, {"execution", t.Execution}, {"cleanupBudget", t.CleanupBudget}} {
		fmt.Printf("%s=%d\n", kv.k, int(kv.v.Seconds()))
	}
	return 0
}

func init() {
	extra["aws-slr"] = awsSLR
	extra["manifest-owned"] = manifestOwned
}

// awsSLR snapshots the account's service-linked roles before the run (-out) and
// proves their exact preservation afterwards (-verify -snapshot). Read-only.
func awsSLR(args []string) int {
	fs := flag.NewFlagSet("aws-slr", flag.ExitOnError)
	cfgPath, out, snapshot, verify := fs.String("config", "", ""), fs.String("out", "", "new snapshot file (0600)"), fs.String("snapshot", "", ""), fs.Bool("verify", false, "")
	_ = fs.Parse(args)
	c, code := awsConfigOnly(*cfgPath)
	if code != 0 {
		return code
	}
	cli, err := v2run.NewExecAWS(c)
	if err != nil {
		return fail("%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	now, err := v2run.SnapshotSLRs(ctx, cli)
	if err != nil {
		return fail("%v", err)
	}
	if !*verify {
		raw, _ := json.Marshal(now)
		if err := v2run.WriteFilePrivate(*out, raw); err != nil {
			return fail("%v", err)
		}
		for _, name := range v2run.ServiceLinkedRoles {
			if now[name].Present {
				fmt.Println("present", name)
			}
		}
		return 0
	}
	raw, err := os.ReadFile(*snapshot)
	if err != nil {
		return fail("the service-linked role snapshot is missing")
	}
	var before map[string]v2run.SLRState
	if json.Unmarshal(raw, &before) != nil {
		return fail("the service-linked role snapshot is unreadable")
	}
	if err := v2run.CompareSLRs(before, now); err != nil {
		return fail("%v", err)
	}
	fmt.Println("preexisting service-linked roles preserved (same ARN, role id, trust and attached policies)")
	return 0
}

// manifestOwned lists the CREATED_BY_RUN assets "kind<TAB>id" of the manifest.
func manifestOwned(args []string) int {
	fs := flag.NewFlagSet("manifest-owned", flag.ExitOnError)
	mpath := fs.String("manifest", "", "")
	_ = fs.Parse(args)
	m, err := v2run.LoadManifest(*mpath)
	if err != nil {
		return fail("%v", err)
	}
	for _, a := range m.OwnedByRun() {
		fmt.Printf("%s\t%s\n", a.Kind, a.ID)
	}
	return 0
}

func awsConfigOnly(cfgPath string) (v2run.Config, int) {
	c, err := loadCfg(cfgPath)
	if err != nil {
		return c, fail("%v", err)
	}
	if c.Target != v2run.TargetAWS || c.AWS == nil {
		return c, fail("not an AWS configuration")
	}
	return c, 0
}
