package v04run

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// scriptKube answers `kubectl` calls from a table keyed by the joined arguments.
type scriptKube map[string]struct {
	out    string
	stderr string
	exit   int
}

func (s scriptKube) Run(_ context.Context, args ...string) ([]byte, string, int) {
	if answer, ok := s[strings.Join(args, " ")]; ok {
		return []byte(answer.out), answer.stderr, answer.exit
	}
	return nil, "unexpected call: " + strings.Join(args, " "), 99
}

func deploymentJSON(image string, env []map[string]any, ready bool) string {
	status := map[string]any{"observedGeneration": 1, "readyReplicas": 1, "availableReplicas": 1, "updatedReplicas": 1, "replicas": 1}
	if !ready {
		status["readyReplicas"] = 0
	}
	raw, _ := json.Marshal(map[string]any{
		"metadata": map[string]any{"generation": 1},
		"spec": map[string]any{"replicas": 1, "selector": map[string]any{"matchLabels": map[string]string{"app": "x"}},
			"template": map[string]any{"spec": map[string]any{"containers": []map[string]any{{"image": image, "env": env}}}}},
		"status": status,
	})
	return string(raw)
}

func plain(name, value string) map[string]any { return map[string]any{"name": name, "value": value} }
func secretRef(name string) map[string]any {
	return map[string]any{"name": name, "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": "s", "key": name}}}
}

func appEnv(worker bool, extra ...map[string]any) []map[string]any {
	env := []map[string]any{plain("PGHOST", "postgres"), plain("PGPORT", "5432"), plain("PGDATABASE", "acceptance"), plain("PGUSER", "app"),
		plain("REDIS_HOST", "redis"), plain("REDIS_PORT", "6379"), plain("REDIS_USERNAME", "app"), plain("REDIS_TLS", "false"),
		plain("ACCEPTANCE_CACHE_RUN", "r1"), plain("ACCEPTANCE_CACHE_TTL_SECONDS", "30"), secretRef("PGPASSWORD"), secretRef("REDIS_PASSWORD")}
	if !worker {
		env = append(env, secretRef("ACCEPTANCE_SECRET"))
	}
	return append(env, extra...)
}

func healthyCluster() scriptKube {
	pods := `{"items":[{"metadata":{},"status":{"phase":"Running","conditions":[{"type":"Ready","status":"True"}],"containerStatuses":[{"imageID":"x@sha256:aaa","ready":true}]}}]}`
	k := scriptKube{}
	for _, name := range []string{"backend", "worker", "frontend"} {
		env := appEnv(name == "worker")
		if name == "frontend" {
			env = nil
		}
		k["-n ns get deployment "+name+" -o json"] = struct {
			out, stderr string
			exit        int
		}{out: deploymentJSON("img-"+name, env, true)}
		k["-n ns get service "+name+" -o name"] = struct {
			out, stderr string
			exit        int
		}{out: "service/" + name}
	}
	k["get namespace ns -o json"] = struct {
		out, stderr string
		exit        int
	}{out: `{"metadata":{"labels":{"orchestrator-managed":"true"}}}`}
	k["-n ns get pods -l app=x -o json"] = struct {
		out, stderr string
		exit        int
	}{out: pods}
	k["-n ns get statefulsets -o json"] = struct {
		out, stderr string
		exit        int
	}{out: `{"items":[{"metadata":{"name":"pg","uid":"u-pg"},"spec":{"template":{"spec":{"containers":[{"image":"postgres:16"}]}}}},{"metadata":{"name":"rd","uid":"u-rd"},"spec":{"template":{"spec":{"containers":[{"image":"redis:7"}]}}}}]}`}
	k["get namespace ns -o jsonpath={.metadata.uid}"] = struct {
		out, stderr string
		exit        int
	}{out: "u-ns"}
	k["-n ns get pvc -o json"] = struct {
		out, stderr string
		exit        int
	}{out: `{"items":[{"metadata":{"uid":"c1"},"spec":{"volumeName":"pv-1"}},{"metadata":{"uid":"c2"},"spec":{"volumeName":"pv-2"}}]}`}
	return k
}

func expect() Expect {
	return Expect{Namespace: "ns", Images: map[string]string{"backend": "img-backend", "worker": "img-worker", "frontend": "img-frontend"}}
}

func TestVerifyAppAcceptsOneSharedPostgresAndRedisWithSecretReferences(t *testing.T) {
	report, err := VerifyApp(context.Background(), healthyCluster(), expect())
	if err != nil {
		t.Fatal(err)
	}
	if report.PostgresSets != 1 || report.RedisSets != 1 || len(report.Identity.PVCs) != 2 || report.Identity.PostgresUID != "u-pg" {
		t.Fatalf("report = %+v", report)
	}
	if got := strings.Join(report.SecretBindings["backend"], ","); got != "PGPASSWORD,REDIS_PASSWORD,ACCEPTANCE_SECRET" {
		t.Fatalf("backend secret bindings = %s", got)
	}
	raw, _ := json.Marshal(report)
	if strings.Contains(string(raw), "secretKeyRef") {
		t.Fatal("the report must not carry Secret reference details")
	}
}

func TestVerifyAppRejectsASecretDeliveredAsAPlainValue(t *testing.T) {
	k := healthyCluster()
	k["-n ns get deployment worker -o json"] = struct {
		out, stderr string
		exit        int
	}{out: deploymentJSON("img-worker", append(appEnv(true)[:10:10], plain("PGPASSWORD", "hunter2"), secretRef("REDIS_PASSWORD")), true)}
	_, err := VerifyApp(context.Background(), k, expect())
	if err == nil || !strings.Contains(err.Error(), "PGPASSWORD must come from a Secret reference") || strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("err = %v", err)
	}
}

func TestVerifyAppRejectsADuplicateOrSplitSharedService(t *testing.T) {
	k := healthyCluster()
	split := appEnv(true)
	split[0] = plain("PGHOST", "another-postgres")
	k["-n ns get deployment worker -o json"] = struct {
		out, stderr string
		exit        int
	}{out: deploymentJSON("img-worker", split, true)}
	if _, err := VerifyApp(context.Background(), k, expect()); err == nil || !strings.Contains(err.Error(), "different PGHOST") {
		t.Fatalf("split database err = %v", err)
	}
	k = healthyCluster()
	k["-n ns get statefulsets -o json"] = struct {
		out, stderr string
		exit        int
	}{out: `{"items":[{"metadata":{"uid":"a"},"spec":{"template":{"spec":{"containers":[{"image":"postgres:16"}]}}}},{"metadata":{"uid":"b"},"spec":{"template":{"spec":{"containers":[{"image":"postgres:16"}]}}}},{"metadata":{"uid":"c"},"spec":{"template":{"spec":{"containers":[{"image":"redis:7"}]}}}}]}`}
	if _, err := VerifyApp(context.Background(), k, expect()); err == nil || !strings.Contains(err.Error(), "exactly one PostgreSQL") {
		t.Fatalf("duplicate PostgreSQL err = %v", err)
	}
}

func TestIdentityDetectsRecreationOfASharedService(t *testing.T) {
	before := Identity{NamespaceUID: "n", PostgresUID: "p", RedisUID: "r", PVCs: []string{"1", "2"}, PVs: []string{"a", "b"}}
	same := before
	if !SameIdentity(before, same) {
		t.Fatal("an unchanged identity must compare equal")
	}
	for name, mutate := range map[string]func(*Identity){
		"postgres recreated": func(i *Identity) { i.PostgresUID = "p2" },
		"redis recreated":    func(i *Identity) { i.RedisUID = "r2" },
		"claim recreated":    func(i *Identity) { i.PVCs = []string{"1", "9"} },
		"volume changed":     func(i *Identity) { i.PVs = []string{"a", "z"} },
		"namespace replaced": func(i *Identity) { i.NamespaceUID = "n2" },
	} {
		changed := before
		mutate(&changed)
		if SameIdentity(before, changed) {
			t.Fatalf("%s must not compare equal", name)
		}
	}
}

func TestVerifyVolumesGoneDoesNotTreatAnAPIErrorAsAbsence(t *testing.T) {
	notFound := scriptKube{"get pv pv-1 -o name": {stderr: `Error from server (NotFound): persistentvolumes "pv-1" not found`, exit: 1}}
	if err := VerifyVolumesGone(context.Background(), notFound, []string{"pv-1"}); err != nil {
		t.Fatal(err)
	}
	if err := VerifyVolumesGone(context.Background(), scriptKube{"get pv pv-1 -o name": {out: "persistentvolume/pv-1"}}, []string{"pv-1"}); err == nil {
		t.Fatal("an existing volume must fail")
	}
	if err := VerifyVolumesGone(context.Background(), scriptKube{"get pv pv-1 -o name": {stderr: "connection refused", exit: 1}}, []string{"pv-1"}); err == nil || !strings.Contains(err.Error(), "cannot be proven") {
		t.Fatalf("an API error is not absence: %v", err)
	}
}
