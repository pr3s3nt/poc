// Command v2ctl is the operator tooling of the real Terraform v2 acceptance
// runner (M04 I04-10). It never prints a credential, handle content or
// kubeconfig; only names, counts, IDs and verdicts. Exit status: 0 success,
// 1 failure, 2 usage, 10 (guards only) verified not run-owned.
//
//	v2ctl validate-config   -config F
//	v2ctl capacity          -scenario A02|A03 -instance t4g.medium
//	v2ctl manifest-init     -config F -manifest PATH
//	v2ctl manifest-pin      -manifest PATH -name N [-version V] [-sha256 H]
//	v2ctl manifest-plan     -manifest PATH -kind K -count N
//	v2ctl manifest-namespace -manifest PATH -identity I -generation G -observed NS
//	v2ctl manifest-asset    -manifest PATH -kind K -id ID -layer L -owner O [-linkage X]
//	v2ctl manifest-arm      -manifest PATH
//	v2ctl estimate-check    -config F -manifest PATH -estimate PATH
//	v2ctl supervisor-start  -config F -manifest PATH -private DIR -supervisor BIN -cleanup BIN -handles FILE -runner-pid P [-browser-pid P]
//	v2ctl supervisor-finish -private DIR
//	v2ctl supervisor-wait   -private DIR [-timeout 30m]
//	v2ctl k8s-guard|k8s-verify -manifest PATH [-layer L]   (supervisor ownership guard and independent absence verification; read-only)
//	v2ctl k8s-sentinel      -config F
//	v2ctl k8s-absent        -config F -manifest PATH
//	v2ctl dirty             -root DIR
//	v2ctl scan              -public DIR -secrets FILE...
//	v2ctl check-evidence    -scenario S -marks F -assertions F
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"orchestrator/internal/domain/environment"
	"orchestrator/test/integration/v2run"
)

// extra holds the commands registered by extra.go.
var extra = map[string]func([]string) int{}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: v2ctl <command> [flags]")
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	handlers := map[string]func([]string) int{
		"validate-config": validateConfig, "capacity": capacity, "manifest-init": manifestInit, "manifest-pin": manifestPin,
		"manifest-plan": manifestPlan, "manifest-namespace": manifestNamespace, "manifest-asset": manifestAsset, "manifest-arm": manifestArm,
		"estimate-check": estimateCheck, "supervisor-start": supervisorStart, "supervisor-finish": supervisorFinish,
		"supervisor-wait": supervisorWait, "k8s-guard": k8sGuard, "k8s-verify": k8sVerify,
		"k8s-sentinel": k8sSentinel, "k8s-absent": k8sAbsent, "dirty": dirty, "scan": scan, "check-evidence": checkEvidence,
	}
	for name, fn := range extra {
		handlers[name] = fn
	}
	h, ok := handlers[cmd]
	if !ok {
		fmt.Fprintf(os.Stderr, "v2ctl: unknown command %q\n", cmd)
		os.Exit(2)
	}
	os.Exit(h(args))
}

func fail(format string, args ...any) int {
	fmt.Fprintf(os.Stderr, "v2ctl: "+format+"\n", args...)
	return 1
}

func validateConfig(args []string) int {
	fs := flag.NewFlagSet("validate-config", flag.ExitOnError)
	path := fs.String("config", "", "private configuration file")
	fresh := fs.Bool("fresh", false, "also require that the evidence directory does not exist yet")
	_ = fs.Parse(args)
	c, err := v2run.LoadConfig(*path)
	if err != nil {
		return fail("%v", err)
	}
	if *fresh {
		if err := c.ValidateFresh(); err != nil {
			return fail("%v", err)
		}
	}
	out, _ := json.MarshalIndent(c.PublicView(), "", "  ")
	fmt.Println(string(out))
	return 0
}

func capacity(args []string) int {
	fs := flag.NewFlagSet("capacity", flag.ExitOnError)
	scenario := fs.String("scenario", "", "A02 or A03")
	instance := fs.String("instance", "", "node instance type")
	_ = fs.Parse(args)
	res, err := v2run.CheckAWSScenario(*scenario, *instance)
	if err != nil {
		return fail("%v", err)
	}
	out, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(out))
	if !res.OK() {
		return fail("pod/CPU/memory capacity is insufficient; no resource is created")
	}
	return 0
}

func loadCfg(path string) (v2run.Config, error) { return v2run.LoadConfig(path) }

func manifestInit(args []string) int {
	fs := flag.NewFlagSet("manifest-init", flag.ExitOnError)
	cfgPath, path := fs.String("config", "", ""), fs.String("manifest", "", "new private manifest path")
	_ = fs.Parse(args)
	c, err := loadCfg(*cfgPath)
	if err != nil {
		return fail("%v", err)
	}
	if err := v2run.SaveManifest(*path, v2run.NewManifest(c, time.Now())); err != nil {
		return fail("%v", err)
	}
	return 0
}

func update(path string, fn func(*v2run.Manifest) error) int {
	if err := v2run.UpdateManifest(path, fn); err != nil {
		return fail("%v", err)
	}
	return 0
}

func manifestPin(args []string) int {
	fs := flag.NewFlagSet("manifest-pin", flag.ExitOnError)
	path, name, version, sha := fs.String("manifest", "", ""), fs.String("name", "", ""), fs.String("version", "", ""), fs.String("sha256", "", "")
	_ = fs.Parse(args)
	return update(*path, func(m *v2run.Manifest) error {
		return m.AddPin(v2run.Pin{Name: *name, Version: *version, SHA256: *sha})
	})
}

func manifestPlan(args []string) int {
	fs := flag.NewFlagSet("manifest-plan", flag.ExitOnError)
	path, kind, count := fs.String("manifest", "", ""), fs.String("kind", "", ""), fs.Int("count", 0, "")
	_ = fs.Parse(args)
	return update(*path, func(m *v2run.Manifest) error { return m.AddPlanned(*kind, *count) })
}

func manifestNamespace(args []string) int {
	fs := flag.NewFlagSet("manifest-namespace", flag.ExitOnError)
	path, identity, observed, gen := fs.String("manifest", "", ""), fs.String("identity", "", "namespace identity reported by the Environment"), fs.String("observed", "", "optional exact namespace; when empty it is derived with the product's own NamespaceFor(identity, generation)"), fs.Int64("generation", 0, "")
	_ = fs.Parse(args)
	name := *observed
	if name == "" {
		name = environment.NamespaceFor(*identity, *gen)
	}
	if code := update(*path, func(m *v2run.Manifest) error { return m.RecordNamespace(*identity, *gen, name) }); code != 0 {
		return code
	}
	fmt.Println(name)
	return 0
}

func manifestAsset(args []string) int {
	fs := flag.NewFlagSet("manifest-asset", flag.ExitOnError)
	path, kind, id, layer, owner, linkage := fs.String("manifest", "", ""), fs.String("kind", "", ""), fs.String("id", "", ""), fs.String("layer", "", ""), fs.String("owner", "", ""), fs.String("linkage", "", "")
	_ = fs.Parse(args)
	return update(*path, func(m *v2run.Manifest) error {
		return m.AddAsset(v2run.Asset{Kind: *kind, ID: *id, Layer: *layer, Owner: *owner, Linkage: *linkage})
	})
}

func manifestArm(args []string) int {
	fs := flag.NewFlagSet("manifest-arm", flag.ExitOnError)
	path := fs.String("manifest", "", "")
	_ = fs.Parse(args)
	return update(*path, func(m *v2run.Manifest) error { return m.Arm() })
}

func estimateCheck(args []string) int {
	fs := flag.NewFlagSet("estimate-check", flag.ExitOnError)
	cfgPath, mpath, epath := fs.String("config", "", ""), fs.String("manifest", "", ""), fs.String("estimate", "", "")
	_ = fs.Parse(args)
	c, err := loadCfg(*cfgPath)
	if err != nil {
		return fail("%v", err)
	}
	m, err := v2run.LoadManifest(*mpath)
	if err != nil {
		return fail("%v", err)
	}
	raw, err := os.ReadFile(*epath)
	if err != nil {
		return fail("%v", err)
	}
	e, err := v2run.ParseEstimate(raw)
	if err != nil {
		return fail("%v", err)
	}
	if err := e.Check(c, m, time.Now()); err != nil {
		return fail("%v", err)
	}
	fmt.Printf("estimate complete: %d priced lines, total %.2f USD (no ceiling is asserted)\n", len(e.Lines), e.TotalUSD)
	return 0
}

// supervisorStart builds the spec, starts the supervisor detached (own
// session and process group) and returns only after it is ARMED.
func supervisorStart(args []string) int {
	fs := flag.NewFlagSet("supervisor-start", flag.ExitOnError)
	cfgPath, mpath, private, binary := fs.String("config", "", ""), fs.String("manifest", "", ""), fs.String("private", "", "private run directory"), fs.String("supervisor", "", "v2supervisor binary")
	cleanupBin, handles := fs.String("cleanup", "", "v2cleanup binary (the independent Terraform safety net)"), fs.String("handles", "", "private cleanup handles file")
	runnerPID, browserPID := fs.Int("runner-pid", 0, ""), fs.Int("browser-pid", 0, "")
	_ = fs.Parse(args)
	c, err := loadCfg(*cfgPath)
	if err != nil {
		return fail("%v", err)
	}
	m, err := v2run.LoadManifest(*mpath)
	if err != nil {
		return fail("%v", err)
	}
	if !m.Armed {
		return fail("the manifest must be armed (pins, planned counts and expiry recorded) before the supervisor starts")
	}
	if c.Target == v2run.TargetAWS {
		// No AWS effect before I04-11 passed and was cleaned.
		if err := v2run.AWSGate(c); err != nil {
			return fail("%v", err)
		}
	}
	ctl, err := os.Executable()
	if err != nil {
		return fail("%v", err)
	}
	if !filepath.IsAbs(*cleanupBin) || !filepath.IsAbs(*handles) {
		return fail("-cleanup and -handles must be absolute paths")
	}
	if _, err := v2run.LoadCleanupHandles(*handles); err != nil {
		return fail("%v", err)
	}
	var watch []v2run.Watch
	for name, pid := range map[string]int{"runner": *runnerPID, "browser": *browserPID} {
		if pid == 0 {
			if name == "runner" {
				return fail("-runner-pid is required")
			}
			continue
		}
		ticks, err := v2run.ProcessIdentity(pid)
		if err != nil {
			return fail("watched %s process %d does not exist", name, pid)
		}
		watch = append(watch, v2run.Watch{Name: name, PID: pid, StartTicks: ticks})
	}
	spec := v2run.Spec{
		RunID: c.RunID, StatusPath: filepath.Join(*private, "supervisor-status.json"), FinishPath: filepath.Join(*private, "supervisor-finish"),
		LogDir: filepath.Join(*private, "supervisor-logs"), ExpiresAt: m.ExpiresAt, CleanupBudget: c.Cleanup, Watch: watch,
		Steps: cleanupSteps(c, ctl, *cleanupBin, *mpath, *handles, *cfgPath, filepath.Join(*private, "vault-snapshot.json")),
	}
	if err := spec.Validate(time.Now()); err != nil {
		return fail("%v", err)
	}
	specPath := filepath.Join(*private, "supervisor-spec.json")
	raw, _ := json.Marshal(spec)
	if err := v2run.WriteFilePrivate(specPath, raw); err != nil {
		return fail("%v", err)
	}
	armed, err := v2run.StartDetached(*binary, []string{"-spec", specPath}, spec.StatusPath, 20*time.Second)
	if err != nil {
		return fail("%v", err)
	}
	out, _ := json.Marshal(map[string]any{"armed": true, "pid": armed.PID, "pgid": armed.PGID, "sid": armed.SID})
	fmt.Println(string(out))
	return 0
}

func supervisorFinish(args []string) int {
	fs := flag.NewFlagSet("supervisor-finish", flag.ExitOnError)
	private := fs.String("private", "", "")
	_ = fs.Parse(args)
	if err := v2run.RequestFinish(filepath.Join(*private, "supervisor-finish")); err != nil {
		return fail("%v", err)
	}
	return 0
}

func supervisorWait(args []string) int {
	fs := flag.NewFlagSet("supervisor-wait", flag.ExitOnError)
	private, timeout := fs.String("private", "", ""), fs.Duration("timeout", 30*time.Minute, "")
	_ = fs.Parse(args)
	st, err := v2run.WaitTerminal(filepath.Join(*private, "supervisor-status.json"), *timeout)
	if err != nil {
		return fail("%v", err)
	}
	out, _ := json.MarshalIndent(st, "", "  ")
	fmt.Println(string(out))
	if st.State != v2run.StateClean {
		return fail("cleanup needs attention; state and recovery evidence are kept, remaining: %s", strings.Join(st.Remaining, ","))
	}
	return 0
}

func kube() (v2run.ExecKubectl, error) {
	k := v2run.ExecKubectl{Binary: os.Getenv("V2_KUBECTL"), Kubeconfig: os.Getenv("V2_KUBECONFIG"), Context: os.Getenv("V2_KUBECONTEXT")}
	if k.Binary == "" || k.Kubeconfig == "" || k.Context != v2run.KubeContextK8s4f {
		return k, errors.New("V2_KUBECTL, V2_KUBECONFIG and V2_KUBECONTEXT=" + v2run.KubeContextK8s4f + " are required")
	}
	return k, nil
}

func withManifest(args []string, name string, withLayer bool, fn func(context.Context, v2run.ExecKubectl, v2run.Manifest, string) int) int {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	mpath, layer := fs.String("manifest", "", ""), ""
	lp := fs.String("layer", "", "")
	_ = fs.Parse(args)
	layer = *lp
	k, err := kube()
	if err != nil {
		return fail("%v", err)
	}
	m, err := v2run.LoadManifest(*mpath)
	if err != nil {
		return fail("%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Minute)
	defer cancel()
	return fn(ctx, k, m, layer)
}

func k8sGuard(args []string) int {
	fs := flag.NewFlagSet("k8s-guard", flag.ExitOnError)
	mpath, cfgPath := fs.String("manifest", "", ""), fs.String("config", "", "")
	_ = fs.Parse(args)
	k, err := kube()
	if err != nil {
		return fail("%v", err)
	}
	m, err := v2run.LoadManifest(*mpath)
	if err != nil {
		return fail("%v", err)
	}
	c, err := loadCfg(*cfgPath)
	if err != nil {
		return fail("%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Minute)
	defer cancel()
	return v2run.GuardNamespaceForRun(ctx, k, m, c)
}

func k8sVerify(args []string) int {
	return withManifest(args, "k8s-verify", true, func(ctx context.Context, k v2run.ExecKubectl, m v2run.Manifest, layer string) int {
		if err := v2run.VerifyLayer(ctx, k, m, layer); err != nil {
			return fail("%v", err)
		}
		return 0
	})
}

func k8sSentinel(args []string) int {
	fs := flag.NewFlagSet("k8s-sentinel", flag.ExitOnError)
	cfgPath := fs.String("config", "", "")
	_ = fs.Parse(args)
	c, err := loadCfg(*cfgPath)
	if err != nil {
		return fail("%v", err)
	}
	k := v2run.ExecKubectl{Binary: c.Binaries["kubectl"].Path, Kubeconfig: c.Kube.Kubeconfig, Context: c.Kube.Context}
	if err := v2run.VerifySentinel(context.Background(), k, *c.Kube); err != nil {
		return fail("%v", err)
	}
	fmt.Println("sentinel preserved (UID unchanged)")
	return 0
}

// k8sAbsent runs every read-only inventory query and prints verdicts only.
func k8sAbsent(args []string) int {
	fs := flag.NewFlagSet("k8s-absent", flag.ExitOnError)
	cfgPath, mpath := fs.String("config", "", ""), fs.String("manifest", "", "")
	_ = fs.Parse(args)
	c, err := loadCfg(*cfgPath)
	if err != nil {
		return fail("%v", err)
	}
	m, err := v2run.LoadManifest(*mpath)
	if err != nil {
		return fail("%v", err)
	}
	k := v2run.ExecKubectl{Binary: c.Binaries["kubectl"].Path, Kubeconfig: c.Kube.Kubeconfig, Context: c.Kube.Context}
	bad := 0
	for _, q := range v2run.K8sQueries(m, c.Binaries["kubectl"].Path, c.Kube.Kubeconfig, c.Kube.Context) {
		out, stderr, exit := k.Run(context.Background(), q.Argv[5:]...)
		p := q.Evaluate(out, stderr, exit)
		verdict := "ABSENT"
		if !p.Absent() {
			verdict, bad = "PRESENT-OR-UNKNOWN", bad+1
		}
		fmt.Printf("%s %s %s\n", q.ID, verdict, strings.Join(p.Present, ","))
	}
	out, _, exit := k.Run(context.Background(), "get", "pv", "-o", "json")
	if exit != 0 {
		return fail("persistent volumes could not be listed")
	}
	claimed, _ := v2run.PVsClaimedBy(m.Namespace, out)
	fmt.Printf("pv-claimed-by-run-namespace %d\n", len(claimed))
	if len(claimed) > 0 {
		bad++
	}
	if bad > 0 {
		return fail("%d check(s) are not verified absent", bad)
	}
	return 0
}

func dirty(args []string) int {
	fs := flag.NewFlagSet("dirty", flag.ExitOnError)
	root := fs.String("root", "", "directory holding previous run evidence directories")
	_ = fs.Parse(args)
	list, err := v2run.DirtyRuns(*root)
	if err != nil {
		return fail("%v", err)
	}
	if len(list) > 0 {
		return fail("a previous run is not clean (%s); no new run while dirty", strings.Join(list, ", "))
	}
	fmt.Println("no dirty run")
	return 0
}

func scan(args []string) int {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	public := fs.String("public", "", "directory of public evidence")
	secrets := fs.String("secrets", "", "comma-separated private files whose values must not appear")
	_ = fs.Parse(args)
	n, err := v2run.ScanPublic(*public, strings.Split(*secrets, ","))
	if err != nil {
		return fail("%v", err)
	}
	fmt.Printf("no-secret-in-public-evidence files=%d\n", n)
	return 0
}

func checkEvidence(args []string) int {
	fs := flag.NewFlagSet("check-evidence", flag.ExitOnError)
	scenario, marksPath, assertPath := fs.String("scenario", "", ""), fs.String("marks", "", ""), fs.String("assertions", "", "")
	_ = fs.Parse(args)
	var marks []v2run.Mark
	var asserts []v2run.Assertion
	for path, dst := range map[string]any{*marksPath: &marks, *assertPath: &asserts} {
		raw, err := os.ReadFile(path)
		if err != nil {
			return fail("%v", err)
		}
		if err := json.Unmarshal(raw, dst); err != nil {
			return fail("%s: %v", filepath.Base(path), err)
		}
	}
	if err := v2run.ValidateMarks(*scenario, marks); err != nil {
		return fail("marks: %v", err)
	}
	if err := v2run.ValidateAssertions(*scenario, asserts); err != nil {
		return fail("assertions: %v", err)
	}
	fmt.Printf("evidence complete for %s: %d marks, %d assertions, all live\n", *scenario, len(marks), len(asserts))
	return 0
}

func cleanupSteps(c v2run.Config, ctl, cleanup, manifest, handles, config, snapshot string) []v2run.Step {
	if c.Target == v2run.TargetAWS {
		return v2run.AWSCleanupSteps(ctl, cleanup, manifest, handles, config, snapshot, c)
	}
	return v2run.K8sCleanupSteps(ctl, cleanup, manifest, handles, config, snapshot, c)
}
