// issueflow is a development-time supervisor that takes GitHub Issues one at a
// time, drives a Codex coordinator and a coding agent, and publishes results.
// It is separate from the Orchestrator product runtime.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"issueflow/internal/agent"
	"issueflow/internal/clock"
	"issueflow/internal/config"
	"issueflow/internal/evidence"
	"issueflow/internal/gh"
	"issueflow/internal/gitx"
	"issueflow/internal/proc"
	"issueflow/internal/quota"
	"issueflow/internal/runner"
	"issueflow/internal/state"
	"issueflow/internal/sup"
	"issueflow/internal/tmux"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "issueflow:", err)
		os.Exit(1)
	}
}

const usage = `usage: issueflow --config FILE <command>
  run | once                       supervisor loop / single tick
  status [--issue N] [--audit]     show tasks; --audit writes an audit report
  answer --issue N --id Q-xxxx --text TEXT
  resume --issue N | abort --issue N
  agent start --brief-file F | send --file F | stop | status
  ask --file F
  report done|blocked|progress --text T
  report evidence --mode cli --reason T | --mode browser [--dir D] --adapter real|fake
  report accept [--note T] [--check CMD]...
  report published --sha S [--pr URL] | handoff --sha S | checkpoint --file F
  probe                            read-only capability probe
  evidence-schema [--issue N]      machine-readable browser evidence contract (JSON)
  evidence-hash                    content hash of the task worktree (binds evidence)
`

func envDefault(k string) string { return os.Getenv(k) }

type common struct {
	cfgPath string
	issue   int
	run     string
	gen     int
	role    string
}

func parseCommon(fs *flag.FlagSet, c *common) {
	fs.StringVar(&c.cfgPath, "config", envDefault("ISSUEFLOW_CONFIG"), "config file")
	n, _ := strconv.Atoi(envDefault("ISSUEFLOW_ISSUE"))
	g, _ := strconv.Atoi(envDefault("ISSUEFLOW_GEN"))
	fs.IntVar(&c.issue, "issue", n, "issue number")
	fs.StringVar(&c.run, "run", envDefault("ISSUEFLOW_RUN"), "run id")
	fs.IntVar(&c.gen, "gen", g, "generation")
	fs.StringVar(&c.role, "role", envDefault("ISSUEFLOW_ROLE"), "coordinator|coder")
}

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

func run(args []string) error {
	// Global --config may precede the command.
	var cfgPath string
	if len(args) >= 2 && args[0] == "--config" {
		cfgPath, args = args[1], args[2:]
		os.Setenv("ISSUEFLOW_CONFIG", cfgPath)
	}
	if len(args) == 0 {
		fmt.Print(usage)
		return fmt.Errorf("missing command")
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "run", "once":
		return cmdRun(cmd, rest)
	case "status":
		return cmdStatus(rest)
	case "answer":
		return cmdAnswer(rest)
	case "resume":
		return cmdSimple(rest, sup.CmdResume)
	case "abort":
		return cmdSimple(rest, sup.CmdAbort)
	case "agent":
		return cmdAgent(rest)
	case "ask":
		return cmdAsk(rest)
	case "report":
		return cmdReport(rest)
	case "probe":
		return cmdProbe(rest)
	case "evidence-schema":
		return cmdEvidenceSchema(rest)
	case "evidence-hash":
		return cmdEvidenceHash(rest)
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	}
	fmt.Print(usage)
	return fmt.Errorf("unknown command %q", cmd)
}

func load(path string) (*config.Config, *state.Store, error) {
	if path == "" {
		return nil, nil, fmt.Errorf("--config (or ISSUEFLOW_CONFIG) is required")
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, nil, err
	}
	st, err := state.Open(cfg.StateDir)
	return cfg, st, err
}

func cmdRun(name string, args []string) error {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	var c common
	parseCommon(fs, &c)
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, st, err := load(c.cfgPath)
	if err != nil {
		return err
	}
	abs, _ := filepath.Abs(c.cfgPath)
	bin, _ := os.Executable()
	var r runner.Runner = runner.Timeout{R: runner.OS{}, ByName: cfg.TimeoutMap()}
	var ghc gh.GitHub = &gh.CLI{Repo: cfg.Repo, R: r}
	tm := &tmux.Real{R: r, Tmp: cfg.StateDir}
	procs := proc.Real{}
	local := &agent.Local{NameStr: cfg.Coder.Adapter, Command: cfg.Coder.Command, T: tm, R: r, Procs: procs}
	var cloud agent.Agent
	if cfg.Cloud.Mode != "off" {
		cloud = &agent.Cloud{Command: cfg.Cloud.Command, R: r}
	}
	q := quota.New(cfg.Quota.QuotaPatterns, cfg.Quota.ContextPatterns, cfg.Quota.AuthPatterns, cfg.Quota.MarginSeconds, cfg.Quota.BackoffSeconds)
	s := sup.New(sup.Deps{Cfg: cfg, CfgPath: abs, Binary: bin, Store: st, Clock: clock.Real{}, GH: ghc, Tmux: tm, Run: r,
		Git: gitx.Git{R: r}, Coder: local, Cloud: cloud, Procs: procs, Quota: q,
		Val: evidence.Validator{R: r}, Pub: evidence.Publisher{GH: ghc, Prefix: cfg.Evidence.TagPrefix},
		Out: func(m string) { fmt.Fprintln(os.Stderr, time.Now().UTC().Format(time.RFC3339), m) }})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if name == "once" {
		if err := st.Lock(); err != nil {
			return err
		}
		defer st.Unlock()
		s.ProbeCloud(ctx)
		return s.Tick(ctx)
	}
	return s.Run(ctx)
}

func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	var c common
	parseCommon(fs, &c)
	audit := fs.Bool("audit", false, "write tasks/N/audit.json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_, st, err := load(c.cfgPath)
	if err != nil {
		return err
	}
	tasks, err := st.ListTasks()
	if err != nil {
		return err
	}
	q, _ := st.LoadQueue()
	fmt.Printf("queue halted=%v reason=%s issue=%d\n", q.Halted, q.Reason, q.Issue)
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].Issue < tasks[j].Issue })
	for _, t := range tasks {
		if c.issue != 0 && t.Issue != c.issue {
			continue
		}
		w := "-"
		if t.Wake != nil {
			w = t.Wake.Role + "@" + t.Wake.At.Format(time.RFC3339)
		}
		qs := "-"
		if t.Question != nil && !t.Question.Answered {
			qs = t.Question.ID
		}
		fmt.Printf("#%d %-10s run=%s coder=%s/%s/%s lease=%q wake=%s question=%s\n", t.Issue, t.State, t.RunID, t.Coder.Kind, t.Coder.Session.Status, fmt.Sprint(t.Coder.Gen), t.Lease.Holder, w, qs)
		if *audit {
			b, _ := json.MarshalIndent(map[string]any{"task": t, "generated": time.Now().UTC()}, "", "  ")
			p := st.Sub(t.Issue, "audit.json")
			if err := state.WriteFileAtomic(p, b, 0o600); err != nil {
				return err
			}
			fmt.Println("  audit:", p)
		}
	}
	return nil
}

func submit(c common, cmd state.Command) error {
	_, st, err := load(c.cfgPath)
	if err != nil {
		return err
	}
	if c.issue == 0 {
		return fmt.Errorf("--issue (or ISSUEFLOW_ISSUE) is required")
	}
	if cmd.Role == "" {
		cmd.Role = c.role
	}
	cmd.Run, cmd.Gen = c.run, c.gen
	cmd.CreatedAt = time.Now().UTC()
	id, err := st.Enqueue(c.issue, cmd)
	if err != nil {
		return err
	}
	fmt.Println("queued", id)
	return nil
}

func cmdAnswer(args []string) error {
	fs := flag.NewFlagSet("answer", flag.ContinueOnError)
	var c common
	parseCommon(fs, &c)
	id := fs.String("id", "", "question id")
	text := fs.String("text", "", "answer")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" || *text == "" {
		return fmt.Errorf("--id and --text are required")
	}
	return submit(c, state.Command{Kind: sup.CmdAnswer, Role: "user", Args: map[string]string{"id": *id, "text": *text}})
}

func cmdSimple(args []string, kind string) error {
	fs := flag.NewFlagSet(kind, flag.ContinueOnError)
	var c common
	parseCommon(fs, &c)
	if err := fs.Parse(args); err != nil {
		return err
	}
	return submit(c, state.Command{Kind: kind, Role: "user"})
}

func needCoordinator(c common) error {
	if c.role != "coordinator" {
		return fmt.Errorf("this command is for the coordinator (ISSUEFLOW_ROLE=coordinator)")
	}
	return nil
}

func cmdAgent(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("agent: start|send|stop|status")
	}
	sub, rest := args[0], args[1:]
	fs := flag.NewFlagSet("agent "+sub, flag.ContinueOnError)
	var c common
	parseCommon(fs, &c)
	file := fs.String("brief-file", "", "brief file")
	ffile := fs.String("file", "", "message file")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	switch sub {
	case "status":
		_, st, err := load(c.cfgPath)
		if err != nil {
			return err
		}
		t, err := st.LoadTask(c.issue)
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(map[string]any{"state": t.State, "coder": t.Coder, "lease": t.Lease, "wake": t.Wake, "handoff": t.Handoff, "evidence": t.Evidence.Mode}, "", "  ")
		fmt.Println(string(b))
		return nil
	case "start":
		if err := needCoordinator(c); err != nil {
			return err
		}
		return submit(c, state.Command{Kind: sup.CmdAgentStart, Args: map[string]string{"file": absPath(*file)}})
	case "send":
		if err := needCoordinator(c); err != nil {
			return err
		}
		return submit(c, state.Command{Kind: sup.CmdAgentSend, Args: map[string]string{"file": absPath(*ffile)}})
	case "stop":
		if err := needCoordinator(c); err != nil {
			return err
		}
		return submit(c, state.Command{Kind: sup.CmdAgentStop})
	}
	return fmt.Errorf("unknown agent command %q", sub)
}

func absPath(p string) string {
	if p == "" {
		return ""
	}
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}

func cmdAsk(args []string) error {
	fs := flag.NewFlagSet("ask", flag.ContinueOnError)
	var c common
	parseCommon(fs, &c)
	file := fs.String("file", "", "question file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := needCoordinator(c); err != nil {
		return err
	}
	return submit(c, state.Command{Kind: sup.CmdAsk, Args: map[string]string{"file": absPath(*file)}})
}

func cmdReport(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("report: done|blocked|progress|evidence|accept|published|handoff|checkpoint")
	}
	sub, rest := args[0], args[1:]
	fs := flag.NewFlagSet("report "+sub, flag.ContinueOnError)
	var c common
	parseCommon(fs, &c)
	text := fs.String("text", "", "text")
	mode := fs.String("mode", "", "cli|browser")
	reason := fs.String("reason", "", "reason")
	dir := fs.String("dir", "", "evidence dir")
	adapter := fs.String("adapter", "", "real|fake")
	note := fs.String("note", "", "note")
	hash := fs.String("hash", "", "content hash reviewed (default: computed now)")
	sha := fs.String("sha", "", "commit sha")
	pr := fs.String("pr", "", "pr url")
	file := fs.String("file", "", "file")
	var checks multi
	fs.Var(&checks, "check", "check command (repeatable)")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	switch sub {
	case "done", "blocked", "progress":
		if c.role != "coder" {
			return fmt.Errorf("report %s is for the coder (ISSUEFLOW_ROLE=coder)", sub)
		}
		return submit(c, state.Command{Kind: "report-" + sub, Args: map[string]string{"text": *text}})
	}
	if err := needCoordinator(c); err != nil {
		return err
	}
	switch sub {
	case "evidence":
		return submit(c, state.Command{Kind: sup.CmdEvidence, Args: map[string]string{"mode": *mode, "reason": *reason, "dir": absPath(*dir), "adapter": *adapter}})
	case "accept":
		// The hash is captured NOW, when the coordinator reports, not when the
		// supervisor later applies the command.
		h := *hash
		if h == "" {
			cfg, st, err := load(c.cfgPath)
			if err != nil {
				return err
			}
			_ = cfg
			t, err := st.LoadTask(c.issue)
			if err != nil {
				return err
			}
			h, err = (gitx.Git{R: runner.OS{}}).ContentHash(context.Background(), t.Worktree)
			if err != nil {
				return fmt.Errorf("cannot hash the worktree: %w", err)
			}
		}
		return submit(c, state.Command{Kind: sup.CmdAccept, Args: map[string]string{"note": *note, "hash": h}, Lists: map[string][]string{"check": checks}})
	case "published":
		if *sha == "" {
			return fmt.Errorf("--sha is required")
		}
		return submit(c, state.Command{Kind: sup.CmdPublished, Args: map[string]string{"sha": *sha, "pr": *pr}})
	case "handoff":
		return submit(c, state.Command{Kind: sup.CmdHandoff, Args: map[string]string{"sha": *sha}})
	case "checkpoint":
		return submit(c, state.Command{Kind: sup.CmdCheckpoint, Args: map[string]string{"file": absPath(*file)}})
	}
	return fmt.Errorf("unknown report %q", sub)
}

func cmdProbe(args []string) error {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	var c common
	parseCommon(fs, &c)
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, _, err := load(c.cfgPath)
	if err != nil {
		return err
	}
	ctx := context.Background()
	r := runner.OS{}
	out := map[string]any{}
	ver := func(name string, a ...string) string {
		res, err := r.Run(ctx, "", nil, name, a...)
		if err != nil || res.Exit != 0 {
			return "unavailable"
		}
		return strings.TrimSpace(strings.SplitN(res.Stdout+res.Stderr, "\n", 2)[0])
	}
	out["tmux"] = ver("tmux", "-V")
	out["git"] = ver("git", "--version")
	out["gh"] = ver("gh", "--version")
	out["ffmpeg"] = ver("ffmpeg", "-version")
	out["ffprobe"] = ver("ffprobe", "-version")
	out["codex"] = ver("codex", "--version")
	ok := func(a ...string) string {
		res, err := r.Run(ctx, "", nil, "codex", a...)
		if err != nil || res.Exit != 0 {
			return "does not parse"
		}
		return "parses"
	}
	out["codex exec --yolo --help"] = ok("exec", "--yolo", "--help")
	out["codex exec resume --yolo --json --help"] = ok("exec", "resume", "--yolo", "--json", "--help")
	loc := &agent.Local{NameStr: cfg.Coder.Adapter, Command: cfg.Coder.Command, T: &tmux.Real{R: r}, R: r}
	out["coder"] = loc.Probe(ctx)
	cl := &agent.Cloud{Command: cfg.Cloud.Command, R: r}
	cp := cl.Probe(ctx)
	out["cloud"] = map[string]any{"mode": cfg.Cloud.Mode, "probe": cp,
		"note": "read-only: no cloud session was created. Status/result/stop retrieval are not documented, so cloud is assisted-only."}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))
	return nil
}

func loadTaskFor(c common) (*state.Task, *config.Config, *state.Store, error) {
	cfg, st, err := load(c.cfgPath)
	if err != nil {
		return nil, nil, nil, err
	}
	if c.issue == 0 {
		return nil, cfg, st, nil
	}
	t, err := st.LoadTask(c.issue)
	if err != nil {
		return nil, cfg, st, err
	}
	return t, cfg, st, nil
}

func cmdEvidenceSchema(args []string) error {
	fs := flag.NewFlagSet("evidence-schema", flag.ContinueOnError)
	var c common
	parseCommon(fs, &c)
	if err := fs.Parse(args); err != nil {
		return err
	}
	t, cfg, st, err := loadTaskFor(c)
	if err != nil {
		return err
	}
	area := "<state_dir>/tasks/<issue>/evidence-in"
	if t != nil {
		area = st.Sub(t.Issue, "evidence-in")
	}
	_ = cfg
	bin, _ := os.Executable()
	abs, _ := filepath.Abs(c.cfgPath)
	fmt.Println(string(sup.EvidenceContractJSON(t, area, fmt.Sprintf("%s --config %s", bin, abs))))
	return nil
}

func cmdEvidenceHash(args []string) error {
	fs := flag.NewFlagSet("evidence-hash", flag.ContinueOnError)
	var c common
	parseCommon(fs, &c)
	if err := fs.Parse(args); err != nil {
		return err
	}
	t, _, _, err := loadTaskFor(c)
	if err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("--issue (or ISSUEFLOW_ISSUE) is required")
	}
	h, err := (gitx.Git{R: runner.OS{}}).ContentHash(context.Background(), t.Worktree)
	if err != nil {
		return err
	}
	fmt.Println(h)
	return nil
}
