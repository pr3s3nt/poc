package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"orchestrator/test/integration/v03run"
	"orchestrator/test/integration/v2run"
)

// V03 internal-frontend runner commands. They only read the cluster (and, for
// v03-reap, delete a namespace whose recorded UID and exact ownership tags still
// match, atomically on the UID). None of them prints a credential, kubeconfig or secret.
func init() {
	extra["v03-preflight"] = v03Preflight
	extra["v03-record-owned"] = v03RecordOwned
	extra["v03-workload"] = v03Workload
	extra["v03-absent"] = v03Absent
	extra["v03-reap"] = v03Reap
	extra["v03-target"] = v03Target
	extra["v03-vault-clean"] = v03VaultClean
}

type v03Kube struct{ binary, kubeconfig, context *string }

func v03Flags(fs *flag.FlagSet) v03Kube {
	return v03Kube{
		binary:     fs.String("kubectl", "kubectl", "pinned kubectl binary"),
		kubeconfig: fs.String("kubeconfig", "", "host kubeconfig handle (runner and safety net only)"),
		context:    fs.String("context", "", "exact host context"),
	}
}

func (k v03Kube) runner() (v2run.ExecKubectl, error) {
	if *k.kubeconfig == "" || *k.context == "" {
		return v2run.ExecKubectl{}, fmt.Errorf("-kubeconfig and -context are required")
	}
	return v2run.ExecKubectl{Binary: *k.binary, Kubeconfig: *k.kubeconfig, Context: *k.context}, nil
}

func splitNames(list string) []string {
	var out []string
	for _, name := range strings.Split(list, ",") {
		if name = strings.TrimSpace(name); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func writeJSON(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}

func readJSON(path string, v any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

func v03Preflight(args []string) int {
	fs := flag.NewFlagSet("v03-preflight", flag.ExitOnError)
	kube := v03Flags(fs)
	namespaces, out := fs.String("namespaces", "", "comma list of the exact run-owned namespaces (must not exist)"), fs.String("out", "", "baseline output (private)")
	_ = fs.Parse(args)
	k, err := kube.runner()
	if err != nil {
		return fail("%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	base, checks, err := v03run.Preflight(ctx, k, *kube.context, splitNames(*namespaces))
	code := printChecks(checks)
	if err != nil {
		return fail("%v", err)
	}
	if *out != "" {
		if err := writeJSON(*out, base); err != nil {
			return fail("%v", err)
		}
	}
	return code
}

// openReceipts reads the product's Console database when a URL file is given; a database that cannot be
// read is reported as an error so the caller never mistakes "unknown" for "none".
func openReceipts(ctx context.Context, urlFile string) (*pgx.Conn, error) {
	raw, err := os.ReadFile(urlFile)
	if err != nil {
		return nil, fmt.Errorf("the database URL file cannot be read")
	}
	conn, err := pgx.Connect(ctx, strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("the Console database cannot be reached")
	}
	return conn, nil
}

func loadOwned(path string) []v03run.OwnedNamespace {
	var owned []v03run.OwnedNamespace
	if path != "" {
		_ = readJSON(path, &owned)
	}
	return owned
}

func v03RecordOwned(args []string) int {
	fs := flag.NewFlagSet("v03-record-owned", flag.ExitOnError)
	kube := v03Flags(fs)
	baselinePath, namespaces, out := fs.String("baseline", "", ""), fs.String("namespaces", "", "subset of the baseline run namespaces to record"), fs.String("out", "", "owned output (private); an existing record is merged")
	runID, dbFile := fs.String("run-id", "", "exact run ID the product stamped on its namespaces"), fs.String("database-url-file", "", "Console database URL file: the product receipts")
	_ = fs.Parse(args)
	k, err := kube.runner()
	if err != nil {
		return fail("%v", err)
	}
	var base v03run.Baseline
	if err := readJSON(*baselinePath, &base); err != nil {
		return fail("baseline: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	conn, err := openReceipts(ctx, *dbFile)
	if err != nil {
		return fail("%v", err)
	}
	defer conn.Close(ctx)
	receipts, err := v03run.ReadNamespaceReceipts(ctx, conn)
	if err != nil {
		return fail("%v", err)
	}
	owned, absent, recordErr := v03run.RecordOwned(ctx, k, base, *runID, receipts, splitNames(*namespaces))
	merged := map[string]v03run.OwnedNamespace{}
	for _, record := range loadOwned(*out) {
		merged[record.Name] = record
	}
	for _, record := range owned {
		merged[record.Name] = record
	}
	list := make([]v03run.OwnedNamespace, 0, len(merged))
	for _, record := range merged {
		list = append(list, record)
	}
	if len(list) > 0 {
		if err := writeJSON(*out, list); err != nil {
			return fail("%v", err)
		}
	}
	for _, ns := range owned {
		fmt.Printf("owned namespace %s (exact run tags and product receipt proven; UID recorded in the private file)\n", ns.Name)
	}
	for _, name := range absent {
		fmt.Printf("namespace %s does not exist yet\n", name)
	}
	if recordErr != nil {
		return fail("%v", recordErr)
	}
	return 0
}

// v03Reap is the cleanup attempt shared by the runner's trap and its independent supervisor.
func v03Reap(args []string) int {
	fs := flag.NewFlagSet("v03-reap", flag.ExitOnError)
	kube := v03Flags(fs)
	baselinePath, ownedPath, out := fs.String("baseline", "", ""), fs.String("owned", "", "the scenario's earlier ownership record (optional)"), fs.String("out", "", "result JSON (private)")
	runID, dbFile, tmp := fs.String("run-id", "", ""), fs.String("database-url-file", "", "Console database URL file (product receipts)"), fs.String("tmp-dir", "", "private directory for request bodies")
	waitSeconds := fs.Int("wait", 600, "seconds to wait for the deletions to finish")
	_ = fs.Parse(args)
	k, err := kube.runner()
	if err != nil {
		return fail("%v", err)
	}
	var base v03run.Baseline
	if err := readJSON(*baselinePath, &base); err != nil {
		return fail("baseline: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*waitSeconds+120)*time.Second)
	defer cancel()
	var receipts []v03run.Receipt
	if conn, err := openReceipts(ctx, *dbFile); err != nil {
		fmt.Printf("product receipts unavailable (%v); only the scenario record can prove ownership\n", err)
	} else {
		defer conn.Close(ctx)
		if receipts, err = v03run.ReadNamespaceReceipts(ctx, conn); err != nil {
			fmt.Printf("product receipts unavailable (%v); only the scenario record can prove ownership\n", err)
			receipts = nil
		}
	}
	result := v03run.Reap(ctx, k, *tmp, base, *runID, loadOwned(*ownedPath), receipts, time.Duration(*waitSeconds)*time.Second)
	if *out != "" {
		_ = writeJSON(*out, result)
	}
	for _, name := range result.Deleted {
		fmt.Printf("namespace %s: ownership proven (UID, exact tags), deletion requested atomically on the UID\n", name)
	}
	for _, name := range result.Absent {
		fmt.Printf("namespace %s: absent\n", name)
	}
	for _, name := range result.Unproven {
		fmt.Printf("namespace %s: exists but is NOT proven run-owned; left untouched\n", name)
	}
	for _, name := range result.Remaining {
		fmt.Printf("namespace %s: still present after the wait\n", name)
	}
	for _, problem := range result.Problems {
		fmt.Printf("problem: %s\n", problem)
	}
	if !result.OK() {
		return fail("the cluster cleanup is not proven")
	}
	fmt.Println("run namespaces proven absent; baseline namespaces unchanged")
	return 0
}

// v03VaultClean deletes the credential objects of the run's exact account keys with strict HTTP handling.
func v03VaultClean(args []string) int {
	fs := flag.NewFlagSet("v03-vault-clean", flag.ExitOnError)
	addr, mount, tokenFile, org := fs.String("vault-addr", "", ""), fs.String("mount", "secret", ""), fs.String("token-file", "", ""), fs.String("org", "", "")
	dbFile, scenarioFile := fs.String("database-url-file", "", "Console database URL file (account receipts)"), fs.String("scenario-keys", "", "the scenario's own record of the account keys")
	_ = fs.Parse(args)
	token, err := os.ReadFile(*tokenFile)
	if err != nil {
		return fail("the Vault token file cannot be read")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var refs []v03run.ConnectionRef
	receiptsKnown := false
	if conn, err := openReceipts(ctx, *dbFile); err == nil {
		defer conn.Close(ctx)
		if refs, err = v03run.ReadConnectionRefs(ctx, conn); err == nil {
			receiptsKnown = true
		}
	}
	var scenario struct {
		AccountKeys []string `json:"accountKeys"`
	}
	scenarioKnown := *scenarioFile != "" && readJSON(*scenarioFile, &scenario) == nil
	keys := v03run.VaultKeys(*org, refs, scenario.AccountKeys)
	if !receiptsKnown && !scenarioKnown {
		return fail("the run's account keys cannot be determined (no product receipt and no scenario record); Vault is not proven clean")
	}
	vault := v03run.Vault{Address: *addr, Mount: *mount, Token: strings.TrimSpace(string(token))}
	result := vault.CleanAccounts(ctx, *org, keys)
	for _, item := range result.Deleted {
		fmt.Printf("run-owned vault object %s: deleted, absence verified\n", item)
	}
	for _, item := range result.Failed {
		fmt.Printf("FAILED vault %s\n", item)
	}
	if len(result.Failed) > 0 {
		return fail("the Vault cleanup is not proven")
	}
	if !receiptsKnown {
		fmt.Println("note: the product database was not readable; only the scenario-recorded keys were cleaned")
	}
	fmt.Printf("vault: %d account key(s) of this run clean\n", len(keys))
	return 0
}

func v03Workload(args []string) int {
	fs := flag.NewFlagSet("v03-workload", flag.ExitOnError)
	kube := v03Flags(fs)
	namespace, name, image := fs.String("namespace", "", ""), fs.String("name", "", ""), fs.String("image", "", "expected image reference")
	digest := fs.String("digest", "", "expected sha256 digest of the image running in every pod")
	envList := fs.String("env", "", "comma list of NAME=VALUE plain env expectations")
	ready := fs.Bool("ready", false, "require a fully rolled out, ready deployment")
	_ = fs.Parse(args)
	k, err := kube.runner()
	if err != nil {
		return fail("%v", err)
	}
	want := map[string]string{}
	for _, pair := range splitNames(*envList) {
		key, value, ok := strings.Cut(pair, "=")
		if !ok || key == "" {
			return fail("-env entries are NAME=VALUE")
		}
		want[key] = value
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	report, err := v03run.VerifyWorkload(ctx, k, *namespace, *name, *image, *digest, want, *ready)
	// Values are compared, never printed: only names and verdicts.
	fmt.Printf("workload %s/%s image-match=%t ready=%t generation=%d observed=%d replicas-want=%d total=%d updated=%d available=%d ready-replicas=%d pods=%d digest-pods=%d expected-digest=%s pod-image-ids=%v env-mismatch-names=%v service=%t namespace-managed=%t\n",
		report.Namespace, report.Name, report.ImageMatch, report.Ready, report.Generation, report.ObservedGeneration, report.Replicas, report.Total, report.UpdatedReplicas, report.AvailableReplicas, report.ReadyReplicas,
		report.Pods, report.DigestPods, *digest, report.PodImageIDs, report.EnvMismatch, report.Service, report.Managed)
	if err != nil {
		return fail("%v", err)
	}
	return 0
}

func v03Absent(args []string) int {
	fs := flag.NewFlagSet("v03-absent", flag.ExitOnError)
	kube := v03Flags(fs)
	baselinePath, ownedPath := fs.String("baseline", "", ""), fs.String("owned", "", "")
	_ = fs.Parse(args)
	k, err := kube.runner()
	if err != nil {
		return fail("%v", err)
	}
	var base v03run.Baseline
	var owned []v03run.OwnedNamespace
	if err := readJSON(*baselinePath, &base); err != nil {
		return fail("baseline: %v", err)
	}
	if *ownedPath != "" {
		if err := readJSON(*ownedPath, &owned); err != nil {
			return fail("owned: %v", err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := v03run.VerifyAbsent(ctx, k, base, owned); err != nil {
		return fail("%v", err)
	}
	fmt.Printf("run namespaces verified absent (%d), %d baseline namespaces unchanged\n", len(base.RunNamespaces), len(base.Namespaces))
	return 0
}

// v03Target prints the persisted per-workload target pins (safe identities only) so the scenario can
// compare them before and after a restart and an update.
func v03Target(args []string) int {
	fs := flag.NewFlagSet("v03-target", flag.ExitOnError)
	dbFile := fs.String("database-url-file", "", "Console database URL file")
	_ = fs.Parse(args)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	conn, err := openReceipts(ctx, *dbFile)
	if err != nil {
		return fail("%v", err)
	}
	defer conn.Close(ctx)
	targets, err := v03run.ReadAppliedTargets(ctx, conn)
	if err != nil {
		return fail("%v", err)
	}
	raw, _ := json.Marshal(targets)
	fmt.Println(string(raw))
	return 0
}
