package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"orchestrator/test/integration/v03run"
	"orchestrator/test/integration/v04run"
)

// V04 full-application runner commands. They never print a credential, a secret value, a kubeconfig or a
// Redis/PostgreSQL password; they print names, counts, identities and verdicts only.
func init() {
	extra["v04-scores"] = v04Scores
	extra["v04-output-refs"] = v04OutputRefs
	extra["v04-output-inventory"] = v04OutputInventory
	extra["v04-output-clean"] = v04OutputClean
	extra["v04-app"] = v04App
	extra["v04-instances"] = v04Instances
	extra["v04-volumes-gone"] = v04VolumesGone
}

func v04OutputRefs(args []string) int {
	fs := flag.NewFlagSet("v04-output-refs", flag.ExitOnError)
	database := fs.String("database-owner-url-file", "", "private operator owner database URL file")
	runID := fs.String("run-id", "", "exact operator run ID stamped in physical intent")
	out := fs.String("out", "", "private cumulative reference inventory")
	_ = fs.Parse(args)
	if *database == "" || *runID == "" || *out == "" {
		return fail("-database-owner-url-file, -run-id and -out are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	conn, err := openReceipts(ctx, *database)
	if err != nil {
		return fail("the output-reference inventory could not read the private operator database")
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, v04OutputRefQuery, *runID)
	if err != nil {
		return fail("the output-reference inventory query failed")
	}
	refs := map[string]bool{}
	for rows.Next() {
		var document map[string]any
		if err := rows.Scan(&document); err != nil {
			rows.Close()
			return fail("the output-reference inventory row is malformed")
		}
		collectOutputRefs(document, refs)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fail("the output-reference inventory was incomplete")
	}
	rows.Close()
	var prior []string
	if data, err := os.ReadFile(*out); err == nil {
		if !strings.HasPrefix(strings.TrimSpace(string(data)), "[") || json.Unmarshal(data, &prior) != nil {
			return fail("the existing output-reference inventory is malformed")
		}
	} else if !os.IsNotExist(err) {
		return fail("the existing output-reference inventory cannot be read")
	}
	for _, ref := range prior {
		refs[ref] = true
	}
	all := make([]string, 0, len(refs))
	for ref := range refs {
		all = append(all, ref)
	}
	sort.Strings(all)
	if err := writeJSON(*out, all); err != nil {
		return fail("the private output-reference inventory could not be written")
	}
	fmt.Printf("run-owned durable output references inventoried: %d (private path inventory)\n", len(all))
	return 0
}

const v04OutputRefQuery = `
 WITH owned AS MATERIALIZED (
  SELECT tri.id, tri.descriptor, tri.active_resource_id, tri.physical_intent
  FROM terraform_resource_instances tri
  WHERE tri.physical_intent #>> '{tags,run-id}' = $1
 ), history AS MATERIALIZED (
  SELECT owned.id, owned.descriptor, dr.active_resource_id, dr.output_snapshot
  FROM owned
  JOIN terraform_resource_attempts ta ON ta.resource_instance_id=owned.id AND ta.deployment_id IS NOT NULL
  JOIN deployment_resources dr ON dr.deployment_id=ta.deployment_id AND dr.node_descriptor=owned.descriptor
  WHERE owned.active_resource_id IS NULL OR dr.active_resource_id IS NOT DISTINCT FROM owned.active_resource_id
 ), links AS MATERIALIZED (
  SELECT descriptor, active_resource_id FROM owned WHERE active_resource_id IS NOT NULL
  UNION SELECT descriptor, active_resource_id FROM history WHERE active_resource_id IS NOT NULL
 )
 SELECT physical_intent FROM owned
 UNION ALL SELECT output_snapshot FROM history
 UNION ALL SELECT ar.outputs FROM links JOIN active_resources ar ON ar.id=links.active_resource_id
 UNION ALL SELECT dr.output_snapshot FROM links JOIN deployment_resources dr
  ON dr.active_resource_id=links.active_resource_id AND dr.node_descriptor=links.descriptor
`

func v04OutputClean(args []string) int {
	fs := flag.NewFlagSet("v04-output-clean", flag.ExitOnError)
	addr := fs.String("vault-addr", "", "output Vault address")
	mount := fs.String("mount", "secret", "KV v2 mount")
	tokenFile := fs.String("token-file", "", "scoped output-cleanup token file")
	refsFile := fs.String("refs-file", "", "private run-owned reference inventory")
	baselineFile := fs.String("baseline-file", "", "pre-writer protected output metadata baseline")
	runID := fs.String("run-id", "", "exact run identity for baseline verification")
	_ = fs.Parse(args)
	var refs []string
	raw, err := os.ReadFile(*refsFile)
	if err != nil || !strings.HasPrefix(strings.TrimSpace(string(raw)), "[") || json.Unmarshal(raw, &refs) != nil {
		return fail("the exact run-owned output-reference inventory is missing or malformed")
	}
	guarded := *baselineFile != "" || *runID != ""
	checkInventory := func(mode string) int {
		return v04OutputInventory([]string{"-mode", mode, "-vault-addr", *addr, "-mount", *mount,
			"-token-file", *tokenFile, "-baseline-file", *baselineFile, "-run-id", *runID, "-refs-file", *refsFile})
	}
	if guarded && checkInventory("check") != 0 {
		return fail("output deletion refused: metadata completeness and baseline protection are not proven")
	}
	if len(refs) == 0 {
		if guarded && checkInventory("verify") != 0 {
			return 1
		}
		fmt.Println("run-owned durable output references deleted and absence verified: 0 (verified empty inventory)")
		return 0
	}
	token, err := os.ReadFile(*tokenFile)
	if err != nil {
		return fail("the scoped output-cleanup token file cannot be read")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	result := (v03run.Vault{Address: *addr, Mount: *mount, Token: strings.TrimSpace(string(token))}).CleanOutputRefs(ctx, refs)
	fmt.Printf("run-owned durable output references deleted and absence verified: %d\n", len(result.Deleted))
	if len(result.Failed) > 0 {
		fmt.Printf("FAILED durable output reference cleanup: %d object(s)\n", len(result.Failed))
	}
	if len(result.Failed) != 0 {
		return fail("durable output-reference cleanup is not proven")
	}
	if guarded && checkInventory("verify") != 0 {
		return fail("output cleanup did not restore the protected metadata baseline")
	}
	return 0
}

func collectOutputRefs(value any, refs map[string]bool) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if key == "secretRef" {
				if ref, ok := child.(string); ok && strings.HasPrefix(ref, "secret://vault/") && strings.Contains(ref, "/orchestrator/outputs/") {
					refs[ref] = true
				}
				continue
			}
			collectOutputRefs(child, refs)
		}
	case []any:
		for _, child := range v {
			collectOutputRefs(child, refs)
		}
	case string:
		if strings.HasPrefix(v, "secret://vault/") && strings.Contains(v, "/orchestrator/outputs/") {
			refs[v] = true
		}
	}
}

func v04Scores(args []string) int {
	fs := flag.NewFlagSet("v04-scores", flag.ExitOnError)
	out := fs.String("out", "", "output directory for the Score documents (public evidence)")
	frontend, frontendB, backend, worker := fs.String("frontend", "", ""), fs.String("frontend-b", "", "second frontend image for the update step"), fs.String("backend", "", ""), fs.String("worker", "", "")
	app, env, run := fs.String("application", "", ""), fs.String("environment", "staging", ""), fs.String("run-id", "", "")
	ttl := fs.Int("ttl", 30, "Redis key TTL seconds")
	dbClass, cacheClass := fs.String("database-class", "in-cluster", ""), fs.String("cache-class", "in-cluster", "")
	noSecret := fs.Bool("no-app-secret", false, "omit the optional ACCEPTANCE_SECRET binding (EKS targets: labelled exception)")
	_ = fs.Parse(args)
	opts := v04run.Options{Frontend: *frontend, Backend: *backend, Worker: *worker, Application: *app, Environment: *env, RunID: *run, TTLSeconds: *ttl, DatabaseClass: *dbClass, CacheClass: *cacheClass, OmitAppSecret: *noSecret}
	result, err := v04run.Build(opts)
	if err != nil {
		return fail("%v", err)
	}
	if err := os.MkdirAll(*out, 0o700); err != nil {
		return fail("%v", err)
	}
	for _, w := range result.Workloads {
		if err := os.WriteFile(filepath.Join(*out, w.Name+".yaml"), []byte(w.YAML), 0o600); err != nil {
			return fail("%v", err)
		}
	}
	probe, _, err := v04run.ProbeScore(opts)
	if err != nil {
		return fail("%v", err)
	}
	if err := os.WriteFile(filepath.Join(*out, "probe.yaml"), []byte(probe), 0o600); err != nil {
		return fail("%v", err)
	}
	e01, _, err := v04run.PreApplyProbeScore(opts)
	if err != nil {
		return fail("%v", err)
	}
	if err := os.WriteFile(filepath.Join(*out, "probe-e01.yaml"), []byte(e01), 0o600); err != nil {
		return fail("%v", err)
	}
	if *frontendB != "" {
		optsB := opts
		optsB.Frontend = *frontendB
		second, err := v04run.Build(optsB)
		if err != nil {
			return fail("%v", err)
		}
		for _, w := range second.Workloads {
			if w.Name != "frontend" {
				continue
			}
			if err := os.WriteFile(filepath.Join(*out, "frontend-b.yaml"), []byte(w.YAML), 0o600); err != nil {
				return fail("%v", err)
			}
			result.Workloads = append(result.Workloads, v04run.Workload{Name: "frontend-b", SourceHash: w.SourceHash, Hash: w.Hash})
		}
	}
	if err := writeJSON(filepath.Join(*out, "scores.json"), result); err != nil {
		return fail("%v", err)
	}
	for _, w := range result.Workloads {
		fmt.Printf("score %s sha256=%s source-sha256=%s\n", w.Name, w.Hash, w.SourceHash)
	}
	fmt.Printf("%d recorded differences from the repository Score source; %d preserved properties; %d configuration keys\n", len(result.Changes), len(result.Preserved), len(result.Config))
	return 0
}

// v04App verifies the whole application in the namespace and (with -identity-out) records the identity of
// the shared services so a later call can prove they were reused. -identity-in compares with an earlier record.
func v04App(args []string) int {
	fs := flag.NewFlagSet("v04-app", flag.ExitOnError)
	kube := v03Flags(fs)
	namespace := fs.String("namespace", "", "")
	frontend, backend, worker := fs.String("frontend", "", "expected image"), fs.String("backend", "", "expected image"), fs.String("worker", "", "expected image")
	frontendDigest := fs.String("frontend-digest", "", "expected digest of the running frontend image")
	identityOut, identityIn := fs.String("identity-out", "", "write the identity snapshot here (private)"), fs.String("identity-in", "", "compare with this earlier snapshot")
	_ = fs.Parse(args)
	k, err := kube.runner()
	if err != nil {
		return fail("%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	expect := v04run.Expect{Namespace: *namespace, Images: map[string]string{"frontend": *frontend, "backend": *backend, "worker": *worker}, Digests: map[string]string{}}
	if *frontendDigest != "" {
		expect.Digests["frontend"] = *frontendDigest
	}
	report, err := v04run.VerifyApp(ctx, k, expect)
	if err != nil {
		return fail("application verification: %v", err)
	}
	fmt.Printf("application verified: 3 workloads ready, passwords by Secret reference, one PostgreSQL (%d) and one Redis (%d) shared by backend and worker, %d claims\n", report.PostgresSets, report.RedisSets, len(report.Identity.PVCs))
	if *identityIn != "" {
		var earlier v04run.Identity
		if err := readJSON(*identityIn, &earlier); err != nil {
			return fail("identity-in: %v", err)
		}
		if !v04run.SameIdentity(earlier, report.Identity) {
			return fail("the shared services were recreated: namespace, PostgreSQL, Redis, claim or volume identity changed")
		}
		fmt.Println("identity unchanged: the same namespace, PostgreSQL, Redis, claims and volumes (reused, not recreated)")
	}
	if *identityOut != "" {
		if err := writeJSON(*identityOut, report.Identity); err != nil {
			return fail("%v", err)
		}
	}
	return 0
}

// v04Instances prints the safe Terraform instance identities of the product database and optionally compares them with
// an earlier read: restart/redeploy/reuse must not create a second PostgreSQL or Redis or replace one.
func v04Instances(args []string) int {
	fs := flag.NewFlagSet("v04-instances", flag.ExitOnError)
	dbFile := fs.String("database-url-file", "", "Console database URL file")
	out, compare := fs.String("out", "", "write the snapshot here (private)"), fs.String("compare", "", "an earlier snapshot")
	_ = fs.Parse(args)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	conn, err := openReceipts(ctx, *dbFile)
	if err != nil {
		return fail("%v", err)
	}
	defer conn.Close(ctx)
	instances, err := v04run.ReadInstances(ctx, conn)
	if err != nil {
		return fail("%v", err)
	}
	if err := v04run.CheckSingleton(instances); err != nil {
		return fail("%v", err)
	}
	fmt.Printf("%d recorded instances; exactly one PostgreSQL and one Redis\n", len(instances))
	if *compare != "" {
		var earlier []v04run.Instance
		if err := readJSON(*compare, &earlier); err != nil {
			return fail("compare: %v", err)
		}
		if err := v04run.SameInstances(earlier, instances); err != nil {
			return fail("%v", err)
		}
		fmt.Println("instance identities unchanged since the earlier snapshot")
	}
	if *out != "" {
		raw, _ := json.Marshal(instances)
		if err := os.WriteFile(*out, raw, 0o600); err != nil {
			return fail("%v", err)
		}
	}
	return 0
}

// v04VolumesGone proves the recorded PersistentVolumes are gone after the run namespace was removed.
func v04VolumesGone(args []string) int {
	fs := flag.NewFlagSet("v04-volumes-gone", flag.ExitOnError)
	kube := v03Flags(fs)
	identityIn := fs.String("identity-in", "", "identity snapshot written by v04-app")
	_ = fs.Parse(args)
	k, err := kube.runner()
	if err != nil {
		return fail("%v", err)
	}
	var identity v04run.Identity
	if err := readJSON(*identityIn, &identity); err != nil {
		return fail("identity-in: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := v04run.VerifyVolumesGone(ctx, k, identity.PVs); err != nil {
		return fail("%v", err)
	}
	fmt.Printf("%d run-owned persistent volume(s) verified absent\n", len(identity.PVs))
	return 0
}

func init() { extra["v05-persisted-scan"] = v05PersistedScan }

// v05PersistedScan proves that nothing the product persisted for a target (workload target references, deployment
// workload history, persisted plans, resource outputs) holds credential material: no token, kubeconfig, certificate,
// cluster endpoint or temporary path. It prints counts and finding kinds only.
func v05PersistedScan(args []string) int {
	fs := flag.NewFlagSet("v05-persisted-scan", flag.ExitOnError)
	dbFile := fs.String("database-url-file", "", "Console database URL file")
	_ = fs.Parse(args)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	conn, err := openReceipts(ctx, *dbFile)
	if err != nil {
		return fail("%v", err)
	}
	defer conn.Close(ctx)
	scanned, findings, err := v04run.ScanPersisted(ctx, conn)
	if err != nil {
		return fail("%v", err)
	}
	for _, f := range findings {
		fmt.Printf("FINDING %s: %s\n", f.Source, f.Kind)
	}
	if len(findings) > 0 {
		return fail("credential material is persisted (%d finding(s))", len(findings))
	}
	fmt.Printf("%d persisted documents scanned: no token, kubeconfig, certificate, cluster endpoint, access key or temporary path\n", scanned)
	return 0
}
