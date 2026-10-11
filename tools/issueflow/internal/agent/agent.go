// Package agent defines the coding-agent port and its adapters.
package agent

import (
	"context"
	"errors"
	"fmt"

	"issueflow/internal/proc"
)

type Support string

const (
	Verified Support = "verified"
	Unknown  Support = "unknown"
	None     Support = "none"
)

// Caps states what an adapter can truly do. Anything not Verified must not be
// relied on: the supervisor fails closed instead of guessing.
type Caps struct {
	Execution     string  // local|cloud
	Interactive   bool    // live session with a log the supervisor can read
	AsyncSendOnly bool    // Send only queues a message; no reply is awaited
	Status        Support // can the supervisor ask whether the run is finished?
	Result        Support // can it read the produced diff/report from the agent itself?
	ConfirmedStop bool    // Stop can prove the writer has ended
	SeesLocalFS   bool
	HasLocalEnv   bool // kind / Vault / headed browser recording
	Reports       bool // can call the issueflow report CLI
}

// ErrAmbiguous means the launch may or may not have created a session.
var ErrAmbiguous = errors.New("launch outcome ambiguous")

// LimitError carries a classified failure text from an agent command.
type LimitError struct{ Text string }

func (e *LimitError) Error() string { return "agent limit: " + e.Text }

type StartSpec struct {
	Issue   int
	RunID   string
	Gen     int
	Dir     string
	Session string
	Log     string
	Brief   string // path to brief file
	Text    string // brief text (cloud)
	Env     map[string]string
}

type Handle struct {
	Kind    string // local|cloud
	Session string
	Log     string
	CloudID string
	URL     string
	Owner   proc.Owner // leader pid + start time recorded at launch
}

type Liveness int

const (
	Dead Liveness = iota
	Alive
	Indeterminate
)

type SendResult struct {
	Queued    bool // accepted for delivery only
	Pasted    bool
	CloudURL  string
	SessionID string // session the host says it queued the message to, when reported
}

type ProbeResult struct {
	Name       string
	Available  bool
	Version    string
	Notes      []string
	Caps       Caps
	CloudFlags bool // --cloud and --teleport present in help
}

type Agent interface {
	Name() string
	Caps() Caps
	Probe(ctx context.Context) ProbeResult
	Launch(ctx context.Context, spec StartSpec) (Handle, error)
	Alive(ctx context.Context, h Handle) (Liveness, error)
	Send(ctx context.Context, h Handle, text string) (SendResult, error)
	// Stop returns confirmed=true only when the writer is proven gone.
	Stop(ctx context.Context, h Handle) (confirmed bool, err error)
}

var ErrUnsupported = fmt.Errorf("unsupported by adapter")
