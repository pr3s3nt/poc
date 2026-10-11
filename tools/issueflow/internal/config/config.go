// Package config loads the fixed single-repository configuration.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// CoordCfg describes the Codex coordinator. Turns are deterministic
// `exec`/`exec resume` runs; no keys are injected into an interactive TUI.
type CoordCfg struct {
	Command            string   `json:"command"`
	Args               []string `json:"args"`
	TurnTimeoutSeconds int      `json:"turn_timeout_seconds"`
}

type CoderCfg struct {
	Adapter    string `json:"adapter"`
	Command    string `json:"command"`
	ReadyRegex string `json:"ready_regex"`
	// TrustRegex detects the folder-trust dialog. Two screen formats are
	// supported and Enter is never pressed on its own:
	//   numbered:   a line "<n>. Yes ..." -> the supervisor types that n.
	//   unnumbered: the Yes line has no number (the observed Claude Code 2.1.296
	//               screen starts on "No, exit"). Selecting Yes needs explicit
	//               keys in TrustUnnumberedKeys (first key must be a movement or
	//               letter, never Enter). Without them the supervisor fails
	//               closed and asks for a one-time manual trust.
	TrustRegex           string   `json:"trust_regex"`
	TrustYesRegex        string   `json:"trust_yes_regex"`
	TrustUnnumberedRegex string   `json:"trust_unnumbered_regex"`
	TrustUnnumberedKeys  []string `json:"trust_unnumbered_keys"`
	Fallback             []string `json:"fallback"`
}

// CloudCfg: the documented CLI has no status/result/stop command, so only
// "assisted" exists: a cloud session is created, the task parks in WAIT_USER
// holding the write lease, and the user reports completion. There is no
// unattended cloud mode.
type CloudCfg struct {
	// Mode: "off" (default), "auto" (supervisor reads the agent's idle report
	// from the cloud branch; needs an explicit opt-in and a passing probe) or
	// "assisted" (legacy: the user attests completion).
	Mode                 string `json:"mode"`
	Command              string `json:"command"`
	ReportMaxBytes       int    `json:"report_max_bytes"`
	AmbiguousSendSeconds int    `json:"ambiguous_send_seconds"`
}

type CheckCfg struct {
	Name string `json:"name"`
	Cmd  string `json:"cmd"`
	Dir  string `json:"dir"`
}

type QuotaCfg struct {
	MarginSeconds       int      `json:"margin_seconds"`
	BackoffSeconds      []int    `json:"backoff_seconds"`
	QuotaPatterns       []string `json:"quota_patterns"`
	ContextPatterns     []string `json:"context_patterns"`
	AuthPatterns        []string `json:"auth_patterns"`
	MaxTransientRetries int      `json:"max_transient_retries"`
}

// TimeoutCfg bounds every external command so nothing can freeze the supervisor.
type TimeoutCfg struct {
	CloudSeconds  int `json:"cloud_seconds"`  // claude (create/follow-up/probe)
	GHSeconds     int `json:"gh_seconds"`     // gh
	GitSeconds    int `json:"git_seconds"`    // git (fetch/ls-remote can hit the network)
	TmuxSeconds   int `json:"tmux_seconds"`   // tmux
	ChecksSeconds int `json:"checks_seconds"` // validation checks run through bash
	MediaSeconds  int `json:"media_seconds"`  // ffmpeg/ffprobe
}

type EvidenceCfg struct {
	Publisher       string `json:"publisher"`
	SkipVideoVerify bool   `json:"skip_video_verify"`
	TagPrefix       string `json:"tag_prefix"`
}

type Config struct {
	Repo                string      `json:"repo"`
	RepoPath            string      `json:"repo_path"`
	BaseBranch          string      `json:"base_branch"`
	StateDir            string      `json:"state_dir"`
	WorktreeRoot        string      `json:"worktree_root"`
	OwnerLogins         []string    `json:"owner_logins"`
	ReadyLabel          string      `json:"ready_label"`
	ActiveLabel         string      `json:"active_label"`
	DoneLabel           string      `json:"done_label"`
	LocalOnlyLabel      string      `json:"local_only_label"`
	BrowserLabel        string      `json:"browser_label"`
	PollSeconds         int         `json:"poll_seconds"`
	TickSeconds         int         `json:"tick_seconds"`
	QuietSeconds        int         `json:"quiet_seconds"`
	EnterDelaySeconds   int         `json:"enter_delay_seconds"`
	SubmitCheckSeconds  int         `json:"submit_check_seconds"`
	ReadyTimeoutSeconds int         `json:"ready_timeout_seconds"`
	BriefInlineMax      int         `json:"brief_inline_max"`
	TmuxPrefix          string      `json:"tmux_prefix"`
	Binary              string      `json:"binary"`
	Coordinator         CoordCfg    `json:"coordinator"`
	Coder               CoderCfg    `json:"coder"`
	Cloud               CloudCfg    `json:"cloud"`
	Checks              []CheckCfg  `json:"checks"`
	Quota               QuotaCfg    `json:"quota"`
	Evidence            EvidenceCfg `json:"evidence"`
	Timeouts            TimeoutCfg  `json:"timeouts"`
	ExternalLabel       string      `json:"external_label"`
}

var repoRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// KnownAdapters lists coder adapters that have an implementation.
// OpenCode/Kimi are intentionally absent: they would need their own
// implementation and tests; "tmux-generic" only runs a configured command.
var KnownAdapters = map[string]bool{"claude-local": true, "tmux-generic": true, "claude-cloud": true}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	c.Defaults()
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) Defaults() {
	setS := func(p *string, v string) {
		if *p == "" {
			*p = v
		}
	}
	setI := func(p *int, v int) {
		if *p == 0 {
			*p = v
		}
	}
	setS(&c.BaseBranch, "main")
	setS(&c.ReadyLabel, "agent:ready")
	setS(&c.ActiveLabel, "agent:active")
	setS(&c.DoneLabel, "agent:done")
	setS(&c.LocalOnlyLabel, "agent:local-only")
	setS(&c.BrowserLabel, "agent:browser")
	setI(&c.PollSeconds, 240)
	setI(&c.TickSeconds, 5)
	setI(&c.QuietSeconds, 20)
	setI(&c.ReadyTimeoutSeconds, 180)
	setI(&c.BriefInlineMax, 1200)
	setS(&c.TmuxPrefix, "issueflow")
	setS(&c.Coordinator.Command, "codex")
	if len(c.Coordinator.Args) == 0 {
		c.Coordinator.Args = []string{"--yolo", "--json"}
	}
	setI(&c.Coordinator.TurnTimeoutSeconds, 7200)
	setS(&c.Coder.Adapter, "claude-local")
	setS(&c.Coder.Command, "clauded")
	// UNVERIFIED against a real UI: a banner or welcome text is NOT readiness;
	// only the hint under an empty input box or a bare prompt line is.
	setS(&c.Coder.ReadyRegex, `\? for shortcuts|(?m)^\s*[│|]?\s*[>❯]\s*[│|]?\s*$`)
	setS(&c.Coder.TrustRegex, `(?i)(do you trust|trust the files|trust this folder)`)
	setS(&c.Coder.TrustYesRegex, `(?m)^\W*(\d+)[.)]\s*Yes`)
	setS(&c.Coder.TrustUnnumberedRegex, `(?i)yes,?\s+i\s+trust\s+this\s+folder`)
	setS(&c.Cloud.Mode, "off")
	setS(&c.Cloud.Command, "claude")
	setI(&c.Cloud.ReportMaxBytes, 65536)
	setI(&c.Cloud.AmbiguousSendSeconds, 3600)
	setI(&c.EnterDelaySeconds, 2)
	setI(&c.SubmitCheckSeconds, 10)
	setI(&c.Quota.MarginSeconds, 90)
	setI(&c.Quota.MaxTransientRetries, 3)
	if len(c.Quota.BackoffSeconds) == 0 {
		c.Quota.BackoffSeconds = []int{900, 1800, 3600, 7200, 10800}
	}
	if len(c.Quota.QuotaPatterns) == 0 {
		// UNVERIFIED strings: refine from real captured limit messages.
		c.Quota.QuotaPatterns = []string{`(?i)usage limit reached`, `(?i)you'?ve hit your (usage )?limit`, `(?i)5-hour limit reached`, `(?i)weekly limit reached`, `(?i)rate_limit_error`, `(?i)quota exceeded`, `(?i)insufficient_quota`}
	}
	if len(c.Quota.ContextPatterns) == 0 {
		c.Quota.ContextPatterns = []string{`(?i)context (window|length) (is )?(full|exceeded)`, `(?i)prompt is too long`, `(?i)conversation is too long`}
	}
	if len(c.Quota.AuthPatterns) == 0 {
		c.Quota.AuthPatterns = []string{`(?i)please run /login`, `(?i)authentication_error`, `(?i)invalid api key`, `(?i)not logged in`}
	}
	setS(&c.ExternalLabel, "agent:external-verify")
	setI(&c.Timeouts.CloudSeconds, 120)
	setI(&c.Timeouts.GHSeconds, 60)
	setI(&c.Timeouts.GitSeconds, 120)
	setI(&c.Timeouts.TmuxSeconds, 30)
	setI(&c.Timeouts.ChecksSeconds, 3600)
	setI(&c.Timeouts.MediaSeconds, 900)
	setS(&c.Evidence.Publisher, "release")
	setS(&c.Evidence.TagPrefix, "evidence")
}

func (c *Config) Validate() error {
	if !repoRe.MatchString(c.Repo) {
		return fmt.Errorf("config: repo must be owner/name, got %q", c.Repo)
	}
	if len(c.OwnerLogins) == 0 {
		return fmt.Errorf("config: owner_logins is required")
	}
	for _, p := range []struct{ n, v string }{{"repo_path", c.RepoPath}, {"state_dir", c.StateDir}, {"worktree_root", c.WorktreeRoot}} {
		if !filepath.IsAbs(p.v) {
			return fmt.Errorf("config: %s must be an absolute path", p.n)
		}
	}
	if !KnownAdapters[c.Coder.Adapter] || c.Coder.Adapter == "claude-cloud" {
		return fmt.Errorf("config: coder.adapter %q has no local implementation", c.Coder.Adapter)
	}
	for _, f := range c.Coder.Fallback {
		if !KnownAdapters[f] || f == "claude-local" {
			return fmt.Errorf("config: coder.fallback %q is not a supported fallback", f)
		}
	}
	if k := c.Coder.TrustUnnumberedKeys; len(k) > 0 && strings.EqualFold(k[0], "Enter") {
		return fmt.Errorf("config: coder.trust_unnumbered_keys must not start with Enter (a bare Enter may accept the default 'No, exit')")
	}
	if c.Cloud.Mode != "off" && c.Cloud.Mode != "assisted" && c.Cloud.Mode != "auto" {
		return fmt.Errorf("config: cloud.mode must be off, auto or assisted")
	}
	if c.Evidence.Publisher != "release" {
		return fmt.Errorf("config: evidence.publisher %q not supported", c.Evidence.Publisher)
	}
	if c.PollSeconds < 1 {
		return fmt.Errorf("config: poll_seconds must be positive")
	}
	for _, p := range [][]string{c.Quota.QuotaPatterns, c.Quota.ContextPatterns, c.Quota.AuthPatterns, {c.Coder.ReadyRegex, c.Coder.TrustRegex, c.Coder.TrustYesRegex, c.Coder.TrustUnnumberedRegex}} {
		for _, s := range p {
			if _, err := regexp.Compile(s); err != nil {
				return fmt.Errorf("config: bad regex %q: %w", s, err)
			}
		}
	}
	return nil
}

func (c *Config) IsOwner(login string) bool {
	for _, o := range c.OwnerLogins {
		if strings.EqualFold(o, login) {
			return true
		}
	}
	return false
}

// TimeoutMap maps command names to their configured limits.
func (c *Config) TimeoutMap() map[string]time.Duration {
	d := func(s int) time.Duration { return time.Duration(s) * time.Second }
	return map[string]time.Duration{
		"claude": d(c.Timeouts.CloudSeconds), "gh": d(c.Timeouts.GHSeconds), "git": d(c.Timeouts.GitSeconds),
		"tmux": d(c.Timeouts.TmuxSeconds), "bash": d(c.Timeouts.ChecksSeconds),
		"ffmpeg": d(c.Timeouts.MediaSeconds), "ffprobe": d(c.Timeouts.MediaSeconds),
	}
}
