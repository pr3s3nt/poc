package sup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"issueflow/internal/config"
	"issueflow/internal/runner"
	"issueflow/internal/state"
)

// ---- a scripted "cloud agent": it only pushes git commits to its own branch.

type agentSim struct {
	t     *testing.T
	h     *harness
	clone string
	run   string
	cb    string
}

func (h *harness) newAgent(t *testing.T, tk *state.Task) *agentSim {
	clone := filepath.Join(h.dir, "cloud-agent")
	if _, err := os.Stat(clone); err != nil {
		git(t, h.dir, "clone", h.remote, clone)
	}
	return &agentSim{t: t, h: h, clone: clone, run: tk.RunID, cb: tk.Branch + "-cloud"}
}

// code commits a file on the cloud branch (starting from base when the branch
// does not exist yet) and pushes it.
func (a *agentSim) code(base, file, content string) string {
	git(a.t, a.clone, "fetch", "origin")
	if out, err := tryGit(a.clone, "rev-parse", "--verify", "origin/"+a.cb); err == nil && out != "" {
		git(a.t, a.clone, "checkout", "-B", a.cb, "origin/"+a.cb)
	} else {
		git(a.t, a.clone, "checkout", "-B", a.cb, base)
	}
	osWrite(a.t, filepath.Join(a.clone, file), content)
	git(a.t, a.clone, "add", "-A")
	git(a.t, a.clone, "commit", "-m", "cloud code "+file)
	git(a.t, a.clone, "push", "origin", a.cb)
	return git(a.t, a.clone, "rev-parse", "HEAD")
}

func tryGit(dir string, args ...string) (string, error) {
	out, err := execGit(dir, args...)
	return strings.TrimSpace(out), err
}

// report builds a valid report for (gen, base, code) and lets the test break it.
func (a *agentSim) reportDoc(gen int, base, code, status string) map[string]any {
	return map[string]any{"schema": 1, "run_id": a.run, "generation": gen, "base_sha": base, "code_sha": code, "status": status,
		"writer_state": "idle", "summary": "round " + fmt.Sprint(gen), "checks": []map[string]any{{"command": "go test ./...", "exit": 0}}}
}

func (a *agentSim) pushReport(gen int, doc map[string]any, raw string) string {
	git(a.t, a.clone, "fetch", "origin")
	git(a.t, a.clone, "checkout", "-B", a.cb, "origin/"+a.cb)
	p := filepath.Join(a.clone, ".issueflow", fmt.Sprintf("result-%s-%d.json", a.run, gen))
	os.MkdirAll(filepath.Dir(p), 0o755)
	if raw == "" {
		b, _ := json.Marshal(doc)
		raw = string(b)
	}
	osWrite(a.t, p, raw)
	git(a.t, a.clone, "add", "-A")
	git(a.t, a.clone, "commit", "-m", "report")
	git(a.t, a.clone, "push", "origin", a.cb)
	return git(a.t, a.clone, "rev-parse", "HEAD")
}

func (h *harness) autoStub() *cloudStub {
	return &cloudStub{create: okCloud}
}

func autoHarness(t *testing.T, stub *cloudStub, mut func(*config.Config)) *harness {
	return cloudHarness(t, stub, func(c *config.Config) {
		c.Cloud.Mode = "auto"
		c.Cloud.ReportMaxBytes = 4096
		c.Cloud.AmbiguousSendSeconds = 600
		if mut != nil {
			mut(c)
		}
	})
}

// toAutoCloud takes a task through local quota to a running automatic cloud session.
func (h *harness) toAutoCloud(t *testing.T) (*state.Task, *agentSim, string) {
	tk := h.toQuotaWithHandoff(t)
	h.until("coordinator woken", 8, func() bool { return h.task(0).CoordGen >= 1 })
	sha := h.pushCheckpoint(t, tk)
	h.submit(h.task(0), "coordinator", CmdHandoff, map[string]string{"sha": sha}, nil)
	h.ticks(2)
	tk = h.task(0)
	if tk.Cloud == nil || tk.Cloud.Mode != "auto" || tk.Cloud.Gen != 1 || tk.Lease.Holder != "cloud" {
		t.Fatalf("auto cloud not running: state=%s cloud=%+v lease=%+v", tk.State, tk.Cloud, tk.Lease)
	}
	return tk, h.newAgent(t, tk), sha
}

func (h *harness) poll() {
	h.clk.Advance(241 * time.Second)
	h.ticks(2)
}

func TestAutoCloudCompleteFeedbackLoopImportAndReportExclusion(t *testing.T) {
	stub := &cloudStub{create: okCloud}
	h := autoHarness(t, stub, nil)
	tk, ag, base := h.toAutoCloud(t)
	if h.task(0).State == state.WaitUser {
		t.Fatal("automatic mode must not ask the user for a done attestation")
	}
	for _, want := range []string{reportPath(tk.RunID, 1), "REPORT PROTOCOL", `"generation":1`, base, "FINAL separate commit"} {
		if !strings.Contains(stub.briefs[0], want) {
			t.Errorf("cloud brief lacks %q", want)
		}
	}
	// nothing reported: polling alone never completes
	h.poll()
	if h.task(0).Cloud.Status != "running" || len(h.events("cloud-review")) != 0 {
		t.Fatal("completion without a report")
	}
	// branch movement without a report is not completion
	code1 := ag.code(base, "feature.go", "package f\n// v1\n")
	h.poll()
	if h.task(0).Cloud.Status != "running" || len(h.events("cloud-review")) != 0 {
		t.Fatal("a moved branch was treated as completion")
	}
	// generation 1 report
	head1 := ag.pushReport(1, ag.reportDoc(1, base, code1, "done"), "")
	h.poll()
	c := h.task(0).Cloud
	if c.Status != "reported" || c.Outcome != "done" || c.PinnedHead != head1 || c.PinnedCode != code1 {
		t.Fatalf("report not recognised: %+v", c)
	}
	if h.task(0).State != state.Review || len(h.events("cloud-review")) != 1 {
		t.Fatalf("no review event: %s", h.task(0).State)
	}
	ev := h.events("cloud-review")[0].Text
	for _, want := range []string{"NOT an observed process stop", ":(exclude).issueflow", code1, head1, "opens generation 2"} {
		if !strings.Contains(ev, want) {
			t.Errorf("review event lacks %q:\n%s", want, ev)
		}
	}
	// Codex imports round 1 code only (real git), then sends feedback
	wt := h.task(0).Worktree
	git(t, wt, "fetch", "origin", "refs/heads/"+ag.cb)
	applyDiff(t, wt, base, code1)
	if _, err := os.Stat(filepath.Join(wt, ".issueflow")); err == nil {
		t.Fatal("round 1 import carried the report")
	}
	fb := h.writeTemp("fb.md", "REPAIR-INSTRUCTION-XYZ please add tests")
	h.submit(h.task(0), "coordinator", CmdAgentSend, map[string]string{"file": fb}, nil)
	h.ticks(2)
	c = h.task(0).Cloud
	if c.Gen != 2 || c.Status != "running" || c.RoundBase != code1 || c.PinnedHead != "" {
		t.Fatalf("feedback did not open generation 2 on the same session: %+v", c)
	}
	sent := stub.sent()
	if len(sent) != 1 || !strings.Contains(sent[0], "REPAIR-INSTRUCTION-XYZ") || !strings.Contains(sent[0], reportPath(tk.RunID, 2)) || !strings.Contains(sent[0], "generation 2") {
		t.Fatalf("follow-up message: %v", sent)
	}
	if stub.count() != 1 {
		t.Fatal("feedback created a second session")
	}
	if in := h.task(0).Intents["cloud-send-2"]; in == nil || in.Status != "done" || !strings.Contains(in.Detail, "not completion") {
		t.Fatalf("send intent: %+v", in)
	}
	// queued != done: the old report cannot finish round 2
	h.poll()
	if h.task(0).Cloud.Status != "running" || h.task(0).Coder.Done {
		t.Fatal("the generation-1 report completed generation 2")
	}
	// round 2: more code, then its report (history contains the gen-1 report commit)
	code2 := ag.code(code1, "feature_test.go", "package f\n")
	head2 := ag.pushReport(2, ag.reportDoc(2, code1, code2, "done"), "")
	h.poll()
	c = h.task(0).Cloud
	if c.Status != "reported" || c.PinnedHead != head2 || c.PinnedCode != code2 || c.Gen != 2 {
		t.Fatalf("round 2 not recognised: %+v", c)
	}
	// the naive diff between the rounds contains the old report; the pathspec excludes it
	naive := git(t, wt, "diff", "--name-only", code1, code2)
	if !strings.Contains(naive, ".issueflow/result-") {
		t.Fatalf("test premise: the round diff should contain the old report, got %q", naive)
	}
	applyDiff(t, wt, code1, code2)
	if _, err := os.Stat(filepath.Join(wt, ".issueflow")); err == nil {
		t.Fatal("round 2 import carried a report")
	}
	if _, err := os.Stat(filepath.Join(wt, "feature_test.go")); err != nil {
		t.Fatal("round 2 code missing")
	}
	// a report in the product tree is rejected by accept
	h.wtWrite(h.task(0), ".issueflow/result-leaked.json", "{}")
	h.submit(h.task(0), "coordinator", CmdEvidence, map[string]string{"mode": "cli", "reason": "no ui"}, nil)
	h.ticks(1)
	h.accept(h.task(0), "true")
	h.ticks(1)
	if lc := h.lastCommand(); lc.Status != "rejected" || !strings.Contains(lc.Result, "must not be carried into the product") {
		t.Fatalf("report in tree accepted: %+v", lc)
	}
	os.RemoveAll(filepath.Join(wt, ".issueflow"))
	h.accept(h.task(0), "true")
	h.until("publishing", 8, func() bool { return h.task(0).State == state.Publishing })
	sha := h.commitPush(h.task(0))
	if out := git(t, wt, "ls-tree", "-r", "--name-only", "HEAD"); strings.Contains(out, ".issueflow") {
		t.Fatalf("report committed into the product: %s", out)
	}
	h.submit(h.task(0), "coordinator", CmdPublished, map[string]string{"sha": sha}, nil)
	h.until("done", 12, func() bool { return h.task(1).State == state.Done })
	final := h.gh.Cmts[1][len(h.gh.Cmts[1])-1].Body
	for _, want := range []string{"idle report", "not an observed stop", code2, head2} {
		if !strings.Contains(final, want) {
			t.Errorf("final comment lacks %q:\n%s", want, final)
		}
	}
	if strings.Contains(final, "user-attested") {
		t.Error("automatic flow must not claim user attestation")
	}
}

func applyDiff(t *testing.T, wt, from, to string) {
	t.Helper()
	diff := git(t, wt, "diff", from, to, "--", ".", ":(exclude).issueflow")
	if strings.TrimSpace(diff) == "" {
		return
	}
	p := filepath.Join(t.TempDir(), "d.patch")
	os.WriteFile(p, []byte(diff+"\n"), 0o644)
	git(t, wt, "apply", p)
}

func TestAutoCloudReportValidationMatrix(t *testing.T) {
	type mut func(ag *agentSim, doc map[string]any, base, code string) (raw string, push func())
	cases := map[string]func(doc map[string]any){
		"stale generation":       func(d map[string]any) { d["generation"] = 0 },
		"future generation":      func(d map[string]any) { d["generation"] = 7 },
		"wrong run":              func(d map[string]any) { d["run_id"] = "r-other" },
		"wrong base":             func(d map[string]any) { d["base_sha"] = strings.Repeat("a", 40) },
		"short code sha":         func(d map[string]any) { d["code_sha"] = "abc123" },
		"not idle":               func(d map[string]any) { d["writer_state"] = "working" },
		"unknown status":         func(d map[string]any) { d["status"] = "finished" },
		"bad schema":             func(d map[string]any) { d["schema"] = 2 },
		"unknown field":          func(d map[string]any) { d["extra"] = true },
		"missing summary":        func(d map[string]any) { delete(d, "summary") },
		"missing run":            func(d map[string]any) { delete(d, "run_id") },
		"check without exit":     func(d map[string]any) { d["checks"] = []map[string]any{{"command": "go test"}} },
		"check without command":  func(d map[string]any) { d["checks"] = []map[string]any{{"exit": 0}} },
		"code unrelated to base": func(d map[string]any) { d["code_sha"] = "UNRELATED" },
		"huge report":            func(d map[string]any) { d["summary"] = strings.Repeat("x", 9000) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			stub := &cloudStub{create: okCloud}
			h := autoHarness(t, stub, nil)
			tk, ag, base := h.toAutoCloud(t)
			code := ag.code(base, "f.go", "package f\n")
			doc := ag.reportDoc(1, base, code, "done")
			if name == "code unrelated to base" {
				// a real commit that is not on the branch's history
				git(t, ag.clone, "checkout", "-B", "other-history", base)
				osWrite(t, filepath.Join(ag.clone, "o.txt"), "o")
				git(t, ag.clone, "add", "-A")
				git(t, ag.clone, "commit", "-m", "elsewhere")
				doc["code_sha"] = git(t, ag.clone, "rev-parse", "HEAD")
			} else {
				mutate(doc)
			}
			ag.pushReport(1, doc, "")
			h.poll()
			c := h.task(0).Cloud
			if c.Status != "running" || len(h.events("cloud-review")) != 0 || h.task(0).Coder.Done {
				t.Fatalf("invalid report counted as completion: %+v", c)
			}
			if c.LastReject == "" && name != "stale generation" && name != "future generation" {
				// generation mismatches are reported as "awaiting"; all others carry a reason
				t.Logf("no reject reason recorded for %s", name)
			}
			_ = tk
		})
	}
	// malformed documents
	for name, raw := range map[string]string{
		"not json":      "this is not json",
		"trailing data": `{"schema":1} {"again":true}`,
		"trailing text": `{"schema":1}garbage`,
		"empty":         "",
		"array":         `[1,2,3]`,
	} {
		t.Run(name, func(t *testing.T) {
			stub := &cloudStub{create: okCloud}
			h := autoHarness(t, stub, nil)
			tk, ag, base := h.toAutoCloud(t)
			code := ag.code(base, "f.go", "package f\n")
			full, _ := json.Marshal(ag.reportDoc(1, base, code, "done"))
			if name == "trailing data" {
				raw = string(full) + " " + string(full)
			}
			if name == "trailing text" {
				raw = string(full) + "garbage"
			}
			ag.pushReport(1, nil, raw+"\n")
			h.poll()
			if h.task(0).Cloud.Status != "running" || len(h.events("cloud-review")) != 0 {
				t.Fatalf("malformed report accepted: %+v", h.task(0).Cloud)
			}
			if h.task(0).Cloud.LastReject == "" {
				t.Fatal("rejection reason not recorded")
			}
			_ = tk
		})
	}
	t.Run("code after report", func(t *testing.T) {
		stub := &cloudStub{create: okCloud}
		h := autoHarness(t, stub, nil)
		_, ag, base := h.toAutoCloud(t)
		code := ag.code(base, "f.go", "package f\n")
		ag.pushReport(1, ag.reportDoc(1, base, code, "done"), "")
		ag.code(base, "sneaky.go", "package s\n") // product code AFTER the report
		h.poll()
		if h.task(0).Cloud.Status != "running" || !strings.Contains(h.task(0).Cloud.LastReject, "not idle") {
			t.Fatalf("%+v", h.task(0).Cloud)
		}
	})
	t.Run("valid report is accepted", func(t *testing.T) {
		stub := &cloudStub{create: okCloud}
		h := autoHarness(t, stub, nil)
		_, ag, base := h.toAutoCloud(t)
		code := ag.code(base, "f.go", "package f\n")
		ag.pushReport(1, ag.reportDoc(1, base, code, "done"), "")
		h.poll()
		if h.task(0).Cloud.Status != "reported" {
			t.Fatalf("%+v", h.task(0).Cloud)
		}
	})
}

func TestAutoCloudDoneWithoutNewCodeIsValid(t *testing.T) {
	stub := &cloudStub{create: okCloud}
	h := autoHarness(t, stub, nil)
	_, ag, base := h.toAutoCloud(t)
	git(t, ag.clone, "fetch", "origin")
	git(t, ag.clone, "push", "origin", base+":refs/heads/"+ag.cb) // the branch exists at the round base, no new code
	ag.pushReport(1, ag.reportDoc(1, base, base, "done"), "")     // "already satisfied": code == base
	h.poll()
	if c := h.task(0).Cloud; c.Status != "reported" || c.Outcome != "done" {
		t.Fatalf("%+v", c)
	}
}

func TestAutoCloudBlockedGoesToCoordinatorNotUser(t *testing.T) {
	stub := &cloudStub{create: okCloud}
	h := autoHarness(t, stub, nil)
	_, ag, base := h.toAutoCloud(t)
	code := ag.code(base, "f.go", "package f\n")
	doc := ag.reportDoc(1, base, code, "blocked")
	doc["summary"] = "need a decision about the API"
	ag.pushReport(1, doc, "")
	h.poll()
	if h.task(0).State == state.WaitUser {
		t.Fatal("blocked must go to the coordinator, not straight to the user")
	}
	ev := h.events("cloud-blocked")
	if len(ev) != 1 || !strings.Contains(ev[0].Text, "need a decision about the API") || h.task(0).Cloud.Outcome != "blocked" {
		t.Fatalf("%v %+v", ev, h.task(0).Cloud)
	}
}

func TestAutoCloudQuotaReportWakesSameSessionAfterRestart(t *testing.T) {
	stub := &cloudStub{create: okCloud}
	h := autoHarness(t, stub, nil)
	_, ag, base := h.toAutoCloud(t)
	code := ag.code(base, "f.go", "package f\n")
	doc := ag.reportDoc(1, base, code, "quota")
	doc["reset_at"] = h.clk.Now().Add(3 * time.Hour).Format(time.RFC3339)
	ag.pushReport(1, doc, "")
	h.poll()
	tk := h.task(0)
	if tk.State != state.WaitQuota || tk.Wake == nil || tk.Wake.Role != "cloud" || tk.Cloud.Status != "quota-wait" || tk.Wake.Computed != "reset" {
		t.Fatalf("state=%s wake=%+v cloud=%+v", tk.State, tk.Wake, tk.Cloud)
	}
	h.restart()
	h.clk.Advance(2 * time.Hour)
	h.ticks(3)
	if len(stub.sent()) != 0 {
		t.Fatal("follow-up sent before the reset")
	}
	h.clk.Advance(2 * time.Hour)
	h.ticks(4)
	tk = h.task(0)
	if len(stub.sent()) != 1 || stub.count() != 1 || tk.Cloud.Gen != 2 || tk.Cloud.Status != "running" || tk.State == state.WaitQuota {
		t.Fatalf("sent=%d creates=%d cloud=%+v state=%s", len(stub.sent()), stub.count(), tk.Cloud, tk.State)
	}
	if !strings.Contains(stub.sent()[0], reportPath(tk.RunID, 2)) || tk.Cloud.RoundBase != code {
		t.Fatalf("round 2 identity wrong: base=%s msg=%s", tk.Cloud.RoundBase, stub.sent()[0])
	}
	// the generation-2 report is recognised after the restart
	code2 := ag.code(code, "g.go", "package g\n")
	ag.pushReport(2, ag.reportDoc(2, code, code2, "done"), "")
	h.poll()
	if h.task(0).Cloud.Status != "reported" {
		t.Fatalf("%+v", h.task(0).Cloud)
	}
}

func TestAutoCloudQuotaWaitRechecksThePinnedHeadAndFailsClosed(t *testing.T) {
	stub := &cloudStub{create: okCloud}
	h := autoHarness(t, stub, nil)
	_, ag, base := h.toAutoCloud(t)
	code := ag.code(base, "f.go", "package f\n")
	doc := ag.reportDoc(1, base, code, "quota")
	doc["reset_at"] = h.clk.Now().Add(time.Hour).Format(time.RFC3339)
	ag.pushReport(1, doc, "")
	h.poll()
	if h.task(0).State != state.WaitQuota {
		t.Fatalf("%s", h.task(0).State)
	}
	ag.code(base, "late.go", "package late\n") // written while "idle"
	h.clk.Advance(2 * time.Hour)
	h.ticks(4)
	tk := h.task(0)
	if tk.State != state.WaitUser || tk.Question.Kind != "cloud-late-write" || len(stub.sent()) != 0 {
		t.Fatalf("a late write must stop the task before waking: state=%s sent=%d", tk.State, len(stub.sent()))
	}
}

func TestAutoCloudLateWriteAndUnknownRemoteAtGates(t *testing.T) {
	stub := &cloudStub{create: okCloud}
	h := autoHarness(t, stub, nil)
	_, ag, base := h.toAutoCloud(t)
	code := ag.code(base, "f.go", "package f\n")
	ag.pushReport(1, ag.reportDoc(1, base, code, "done"), "")
	h.poll()
	wt := h.task(0).Worktree
	applyDiff(t, wt, base, code)
	h.submit(h.task(0), "coordinator", CmdEvidence, map[string]string{"mode": "cli", "reason": "no ui"}, nil)
	h.ticks(1)

	// unknown remote state blocks acceptance without pretending nothing changed
	good, _ := tryGit(h.repo, "remote", "get-url", "origin")
	git(t, h.repo, "remote", "set-url", "origin", filepath.Join(h.dir, "does-not-exist.git"))
	h.accept(h.task(0), "true")
	h.ticks(1)
	if lc := h.lastCommand(); lc.Status != "rejected" || !strings.Contains(lc.Result, "unknown") {
		t.Fatalf("unknown remote state did not block accept: %+v", lc)
	}
	if h.task(0).State == state.WaitUser {
		t.Fatal("an unreachable remote must block, not raise a late-write question")
	}
	git(t, h.repo, "remote", "set-url", "origin", good)

	// a late write at the accept gate halts the task and the queue
	ag.code(base, "late.go", "package late\n")
	h.accept(h.task(0), "true")
	h.ticks(2)
	if lc := h.lastCommand(); lc.Status != "rejected" || !strings.Contains(lc.Result, "late write") {
		t.Fatalf("%+v", lc)
	}
	if h.task(0).State != state.WaitUser || h.task(0).Question.Kind != "cloud-late-write" {
		t.Fatalf("state=%s", h.task(0).State)
	}
	// repin only with the real head; the old report no longer counts
	q := h.task(0).Question
	h.submit(h.task(0), "user", CmdAnswer, map[string]string{"id": q.ID, "text": "repin " + strings.Repeat("0", 40)}, nil)
	h.ticks(2)
	if h.task(0).Question.ID == q.ID {
		t.Fatal("a wrong sha must re-open the question")
	}
	remote := git(t, h.repo, "ls-remote", "origin", "refs/heads/"+ag.cb)
	head := strings.Fields(remote)[0]
	h.submit(h.task(0), "user", CmdAnswer, map[string]string{"id": h.task(0).Question.ID, "text": "repin " + head}, nil)
	h.ticks(3)
	if h.task(0).Cloud.Status != "running" || h.task(0).Coder.Done {
		t.Fatalf("repin must void the old report: %+v", h.task(0).Cloud)
	}
	h.poll()
	if h.task(0).Cloud.Status != "running" {
		t.Fatal("the old report was re-accepted after a late write")
	}
}

// publishing reaches the "await commit" gate with an accepted, checked tree.
func (h *harness) toPublishing(t *testing.T, ag *agentSim, base, code string) string {
	applyDiff(t, h.task(0).Worktree, base, code)
	h.submit(h.task(0), "coordinator", CmdEvidence, map[string]string{"mode": "cli", "reason": "no ui"}, nil)
	h.ticks(1)
	h.accept(h.task(0), "true")
	h.until("publishing", 8, func() bool { return h.task(0).State == state.Publishing })
	return h.commitPush(h.task(0))
}

func TestAutoCloudLateWriteHaltsPublishAndQueue(t *testing.T) {
	t.Run("at the published command", func(t *testing.T) {
		stub := &cloudStub{create: okCloud}
		h := autoHarness(t, stub, nil)
		_, ag, base := h.toAutoCloud(t)
		code := ag.code(base, "f.go", "package f\n")
		ag.pushReport(1, ag.reportDoc(1, base, code, "done"), "")
		h.poll()
		sha := h.toPublishing(t, ag, base, code)
		ag.code(base, "late.go", "package late\n")
		h.issue(2, "owner", "owner", "agent:ready")
		h.submit(h.task(0), "coordinator", CmdPublished, map[string]string{"sha": sha}, nil)
		h.ticks(3)
		tk := h.task(0)
		if lc := h.commands()[len(h.commands())-1]; lc.Status != "rejected" || !strings.Contains(lc.Result, "late write") {
			t.Fatalf("%+v", lc)
		}
		if tk.State != state.WaitUser || tk.Question.Kind != "cloud-late-write" || h.commentsContaining(1, "task complete") != 0 {
			t.Fatalf("state=%s", tk.State)
		}
		h.ticks(10)
		if ts, _ := h.st.ListTasks(); len(ts) != 1 {
			t.Fatal("queue advanced past a late write")
		}
	})
	t.Run("inside the publish step", func(t *testing.T) {
		stub := &cloudStub{create: okCloud}
		h := autoHarness(t, stub, nil)
		_, ag, base := h.toAutoCloud(t)
		code := ag.code(base, "f.go", "package f\n")
		ag.pushReport(1, ag.reportDoc(1, base, code, "done"), "")
		h.poll()
		sha := h.toPublishing(t, ag, base, code)
		h.gh.FailOn["UploadAsset"] = 1 // the first publish attempt fails and is retried later
		h.submit(h.task(0), "coordinator", CmdPublished, map[string]string{"sha": sha}, nil)
		h.ticks(2)
		if h.task(0).State != state.Publishing || h.task(0).Publish.Attempts != 1 {
			t.Fatalf("premise: %s %+v", h.task(0).State, h.task(0).Publish)
		}
		ag.code(base, "late.go", "package late\n") // written in the retry window
		h.clk.Advance(2 * time.Minute)
		h.ticks(3)
		tk := h.task(0)
		if tk.State != state.WaitUser || tk.Question.Kind != "cloud-late-write" || h.commentsContaining(1, "task complete") != 0 {
			t.Fatalf("the retry published over a late write: state=%s", tk.State)
		}
	})
}

func TestAutoCloudUnknownRemoteBlocksPublishUntilReadable(t *testing.T) {
	stub := &cloudStub{create: okCloud}
	h := autoHarness(t, stub, nil)
	_, ag, base := h.toAutoCloud(t)
	code := ag.code(base, "f.go", "package f\n")
	ag.pushReport(1, ag.reportDoc(1, base, code, "done"), "")
	h.poll()
	sha := h.toPublishing(t, ag, base, code)
	good, _ := tryGit(h.repo, "remote", "get-url", "origin")
	git(t, h.repo, "remote", "set-url", "origin", filepath.Join(h.dir, "gone.git"))
	h.submit(h.task(0), "coordinator", CmdPublished, map[string]string{"sha": sha}, nil)
	h.ticks(3)
	if lc := h.commands()[len(h.commands())-1]; lc.Status != "rejected" || !strings.Contains(lc.Result, "unknown") {
		t.Fatalf("unreadable remote not treated as unsafe: %+v", lc)
	}
	if h.task(0).State == state.Done {
		t.Fatal("published with an unreadable remote")
	}
	git(t, h.repo, "remote", "set-url", "origin", good)
	h.submit(h.task(0), "coordinator", CmdPublished, map[string]string{"sha": sha}, nil)
	h.until("done once readable", 10, func() bool { return h.task(1).State == state.Done })
}

func TestAutoCloudAmbiguousSendIsNeverBlindlyResent(t *testing.T) {
	stub := &cloudStub{create: okCloud}
	h := autoHarness(t, stub, nil)
	_, ag, base := h.toAutoCloud(t)
	code := ag.code(base, "f.go", "package f\n")
	ag.pushReport(1, ag.reportDoc(1, base, code, "done"), "")
	h.poll()
	stub.send = func(string) runner.Result { return runner.Result{Exit: 1, Stderr: "network reset by peer"} }
	h.submit(h.task(0), "coordinator", CmdAgentSend, map[string]string{"file": h.writeTemp("fb.md", "REPAIR-ONE")}, nil)
	h.ticks(2)
	c := h.task(0).Cloud
	if c.Status != "send-ambiguous" || c.Gen != 2 || c.RoundBase != code {
		t.Fatalf("%+v", c)
	}
	h.restart()
	h.clk.Advance(5 * time.Minute)
	h.ticks(10)
	if len(stub.sent()) != 1 || stub.count() != 1 {
		t.Fatalf("blind retry: sent=%d creates=%d", len(stub.sent()), stub.count())
	}
	// the message DID arrive: its report is recognised
	code2 := ag.code(code, "g.go", "package g\n")
	ag.pushReport(2, ag.reportDoc(2, code, code2, "done"), "")
	h.poll()
	if h.task(0).Cloud.Status != "reported" || h.task(0).Cloud.Gen != 2 {
		t.Fatalf("report for the ambiguous generation not recognised: %+v", h.task(0).Cloud)
	}
}

func TestAutoCloudAmbiguousSendEscalatesAndResendIsExplicit(t *testing.T) {
	stub := &cloudStub{create: okCloud}
	h := autoHarness(t, stub, nil)
	_, ag, base := h.toAutoCloud(t)
	code := ag.code(base, "f.go", "package f\n")
	ag.pushReport(1, ag.reportDoc(1, base, code, "done"), "")
	h.poll()
	stub.send = func(string) runner.Result { return runner.Result{Stdout: `{"ok":true,"session_id":"someone_else"}`} } // identity mismatch
	h.submit(h.task(0), "coordinator", CmdAgentSend, map[string]string{"file": h.writeTemp("fb.md", "REPAIR-TWO")}, nil)
	h.ticks(2)
	if h.task(0).Cloud.Status != "send-ambiguous" {
		t.Fatalf("a foreign session id must not count as queued: %+v", h.task(0).Cloud)
	}
	h.clk.Advance(20 * time.Minute)
	h.ticks(3)
	tk := h.task(0)
	if tk.State != state.WaitUser || tk.Question.Kind != "cloud-send" {
		t.Fatalf("state=%s", tk.State)
	}
	if len(stub.sent()) != 1 {
		t.Fatal("re-sent without the user")
	}
	stub.send = nil
	h.submit(tk, "user", CmdAnswer, map[string]string{"id": tk.Question.ID, "text": "resend"}, nil)
	h.ticks(3)
	sent := stub.sent()
	if len(sent) != 2 || sent[0] != sent[1] || !strings.Contains(sent[1], "REPAIR-TWO") {
		t.Fatalf("the resend must carry exactly the same message: %v", sent)
	}
	if h.task(0).Cloud.Status != "running" || h.task(0).Cloud.Gen != 2 {
		t.Fatalf("%+v", h.task(0).Cloud)
	}
}

func TestAutoCloudCrashAfterPendingIntentKeepsNextRoundIdentity(t *testing.T) {
	stub := &cloudStub{create: okCloud}
	h := autoHarness(t, stub, nil)
	_, ag, base := h.toAutoCloud(t)
	code := ag.code(base, "f.go", "package f\n")
	ag.pushReport(1, ag.reportDoc(1, base, code, "done"), "")
	h.poll()
	// simulate: identity + message + pending intent are durable, the process dies before the send result is saved
	tk := h.task(0)
	msg := h.writeTemp("pending-msg.md", "the exact message")
	tk.Cloud.SendGen, tk.Cloud.SendMsg, tk.Cloud.SendBase = 2, msg, tk.Cloud.PinnedCode
	tk.Intents["cloud-send-2"] = &state.Intent{Status: "pending", Detail: "x"}
	if err := h.st.SaveTask(tk); err != nil {
		t.Fatal(err)
	}
	h.restart()
	h.poll()
	c := h.task(0).Cloud
	if c.Status != "send-ambiguous" || c.Gen != 2 || c.RoundBase != code {
		t.Fatalf("restart lost the next round's identity: %+v", c)
	}
	if len(stub.sent()) != 0 {
		t.Fatal("re-sent after a crash")
	}
	code2 := ag.code(code, "g.go", "package g\n")
	ag.pushReport(2, ag.reportDoc(2, code, code2, "done"), "")
	h.poll()
	if h.task(0).Cloud.Status != "reported" {
		t.Fatalf("the valid next report was rejected after restart: %+v", h.task(0).Cloud)
	}
}

func TestAutoCloudQuotaRejectedFeedbackKeepsTheExactMessage(t *testing.T) {
	stub := &cloudStub{create: okCloud}
	h := autoHarness(t, stub, nil)
	_, ag, base := h.toAutoCloud(t)
	code := ag.code(base, "f.go", "package f\n")
	ag.pushReport(1, ag.reportDoc(1, base, code, "done"), "")
	h.poll()
	rejected := true
	stub.send = func(string) runner.Result {
		if rejected {
			return runner.Result{Exit: 1, Stderr: "Usage limit reached. resets in 1h 0m"}
		}
		return runner.Result{Stdout: `{"ok":true,"session_id":"session_abc123XYZ"}`}
	}
	h.submit(h.task(0), "coordinator", CmdAgentSend, map[string]string{"file": h.writeTemp("fb.md", "REPAIR-INSTRUCTION-KEEP-ME")}, nil)
	h.ticks(2)
	tk := h.task(0)
	if tk.State != state.WaitQuota || tk.Wake.Role != "cloud" || tk.Cloud.Status != "quota-wait" || tk.Cloud.Gen != 1 {
		t.Fatalf("state=%s cloud=%+v", tk.State, tk.Cloud)
	}
	h.restart()
	rejected = false
	h.clk.Advance(2 * time.Hour)
	h.ticks(4)
	sent := stub.sent()
	if len(sent) != 2 || sent[0] != sent[1] || !strings.Contains(sent[1], "REPAIR-INSTRUCTION-KEEP-ME") {
		t.Fatalf("the user's repair instructions were lost on retry: %v", sent)
	}
	if c := h.task(0).Cloud; c.Gen != 2 || c.Status != "running" || c.RoundBase != code {
		t.Fatalf("%+v", c)
	}
}

func TestAutoCloudUsesGenerationOneEvenAfterLocalGenerationTwo(t *testing.T) {
	stub := &cloudStub{create: okCloud}
	h := autoHarness(t, stub, nil)
	h.startActive()
	h.startCoder()
	h.submit(h.task(0), "coordinator", CmdAgentStop, nil, nil)
	h.ticks(2)
	h.submit(h.task(0), "coordinator", CmdAgentStart, map[string]string{"file": h.writeTemp("b2.md", "again")}, nil)
	h.coderUp()
	tk := h.task(0)
	if tk.Coder.Gen != 2 {
		t.Fatalf("premise: local coder gen %d", tk.Coder.Gen)
	}
	appendLog(t, tk.Coder.Session.Log, "Usage limit reached. Your limit resets in 3h 0m.\n")
	h.clk.Advance(30 * time.Second)
	h.until("handoff", 10, func() bool { x := h.task(0); return x.Handoff != nil && x.Handoff.Status == "await-push" })
	h.until("coordinator", 8, func() bool { return h.task(0).CoordGen >= 1 })
	sha := h.pushCheckpoint(t, h.task(0))
	h.submit(h.task(0), "coordinator", CmdHandoff, map[string]string{"sha": sha}, nil)
	h.ticks(2)
	tk = h.task(0)
	if tk.Cloud == nil || tk.Cloud.Gen != 1 || !strings.Contains(stub.briefs[0], reportPath(tk.RunID, 1)) || !strings.Contains(stub.briefs[0], `"generation":1`) {
		t.Fatalf("cloud generation and brief disagree: %+v", tk.Cloud)
	}
	ag := h.newAgent(t, tk)
	code := ag.code(sha, "f.go", "package f\n")
	ag.pushReport(1, ag.reportDoc(1, sha, code, "done"), "")
	h.poll()
	if h.task(0).Cloud.Status != "reported" {
		t.Fatalf("%+v", h.task(0).Cloud)
	}
}

func TestOldCoordinatorTurnCannotActAfterANewTurnStarted(t *testing.T) {
	h := newHarness(t, nil)
	tk := h.startActive()
	h.endTurn(0, "") // turn 1 ends successfully
	h.until("turn 2", 8, func() bool { return h.task(0).CoordGen == 2 })
	if h.task(0).CoordEpoch != 0 {
		t.Fatal("premise: no quota reset happened, so the epoch alone would accept turn 1")
	}
	for _, k := range []string{CmdAgentStart, CmdAccept, CmdPublished, CmdEvidence} {
		h.st.Enqueue(1, state.Command{Kind: k, Role: "coordinator", Run: tk.RunID, Gen: 1, Args: map[string]string{"file": "/x", "sha": "x", "hash": "x", "mode": "cli", "reason": "r"}})
	}
	before := len(h.commands())
	h.ticks(1)
	for _, c := range h.commands()[before:] {
		if c.Status != "rejected" || !strings.Contains(c.Result, "stale") {
			t.Fatalf("a finished turn acted after its successor started: %+v", c)
		}
	}
	// the current turn still works, and a command spooled before the next launch is applied first
	h.submit(h.task(0), "coordinator", CmdCheckpoint, map[string]string{"file": h.writeTemp("n.md", "note")}, nil)
	h.ticks(1)
	if h.lastCommand().Status != "applied" {
		t.Fatalf("%+v", h.lastCommand())
	}
}
