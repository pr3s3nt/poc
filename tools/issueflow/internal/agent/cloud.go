package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"issueflow/internal/runner"
)

// Cloud drives `claude --cloud`. Only documented interfaces are used:
//
//	create:   claude --cloud "<brief>"                       (output format not documented)
//	follow-up: claude -p "<msg>" --cloud <id> --output-format json  -> {ok,session_id,url}
//
// No status/result/stop command is documented or verified, so those
// capabilities are Unknown/None. Callers must treat a cloud run as having
// unknown completion and an unconfirmable stop. Nothing here enables PR
// auto-fix or parallel sessions.
type Cloud struct {
	Command string
	R       runner.Runner
}

func (c *Cloud) Name() string { return "claude-cloud" }

func (c *Cloud) Caps() Caps {
	return Caps{Execution: "cloud", AsyncSendOnly: true, Status: Unknown, Result: Unknown, ConfirmedStop: false}
}

func (c *Cloud) Probe(ctx context.Context) ProbeResult {
	p := ProbeResult{Name: c.Name(), Caps: c.Caps()}
	if res, err := c.R.Run(ctx, "", nil, c.Command, "--version"); err == nil && res.Exit == 0 {
		p.Version = strings.TrimSpace(res.Stdout)
		p.Available = true
	} else {
		p.Notes = append(p.Notes, "claude --version failed")
		return p
	}
	res, err := c.R.Run(ctx, "", nil, c.Command, "--help")
	if err == nil && res.Exit == 0 {
		p.CloudFlags = strings.Contains(res.Stdout, "--cloud") && strings.Contains(res.Stdout, "--teleport")
	}
	if !p.CloudFlags {
		p.Available = false
		p.Notes = append(p.Notes, "--cloud/--teleport not present in help")
	}
	p.Notes = append(p.Notes, "flags present does not prove account/repo/environment setup; no cloud session was created",
		"status/result/stop retrieval is not documented: capability stays unknown")
	return p
}

var (
	reSessionID = regexp.MustCompile(`\bsession_[A-Za-z0-9_-]{6,}\b`)
	reCloudURL  = regexp.MustCompile(`https://claude\.ai/code/[A-Za-z0-9_/-]+`)
)

// Launch creates a session. If the command's outcome cannot be established it
// returns ErrAmbiguous and the caller must not create another session.
func (c *Cloud) Launch(ctx context.Context, s StartSpec) (Handle, error) {
	res, err := c.R.Run(ctx, s.Dir, nil, c.Command, "--cloud", s.Text)
	h := Handle{Kind: "cloud"}
	out := res.Stdout + "\n" + res.Stderr
	if err == nil && res.Exit == 0 {
		h.URL = reCloudURL.FindString(out)
		h.CloudID = reSessionID.FindString(out)
		if h.CloudID == "" && h.URL != "" {
			h.CloudID = h.URL[strings.LastIndex(h.URL, "/")+1:]
		}
		if h.CloudID != "" {
			return h, nil
		}
		return h, fmt.Errorf("%w: exit 0 but no session id in output", ErrAmbiguous)
	}
	if err == nil && res.Exit != 0 {
		return h, &LimitError{Text: out}
	}
	return h, fmt.Errorf("%w: %v", ErrAmbiguous, err)
}

func (c *Cloud) Alive(ctx context.Context, h Handle) (Liveness, error) {
	return Indeterminate, ErrUnsupported
}

func (c *Cloud) Send(ctx context.Context, h Handle, text string) (SendResult, error) {
	res, err := c.R.Run(ctx, "", nil, c.Command, "-p", text, "--cloud", h.CloudID, "--output-format", "json")
	if err != nil {
		return SendResult{}, err
	}
	if res.Exit != 0 {
		return SendResult{}, &LimitError{Text: res.Stdout + "\n" + res.Stderr}
	}
	var r struct {
		OK        bool   `json:"ok"`
		SessionID string `json:"session_id"`
		URL       string `json:"url"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(res.Stdout)), &r); err != nil {
		return SendResult{}, fmt.Errorf("cloud send: unparseable output: %w", err)
	}
	if !r.OK {
		return SendResult{}, fmt.Errorf("cloud send: ok=false")
	}
	// "ok" proves queueing only, not that the agent acted on it.
	return SendResult{Queued: true, CloudURL: r.URL, SessionID: r.SessionID}, nil
}

// Stop cannot be confirmed: there is no documented command to end a cloud run.
func (c *Cloud) Stop(ctx context.Context, h Handle) (bool, error) { return false, ErrUnsupported }
