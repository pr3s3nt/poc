// Package state holds the durable task model and its on-disk store.
package state

import (
	"fmt"
	"time"
)

type State string

const (
	Ready      State = "READY"
	Running    State = "RUNNING"
	Review     State = "REVIEW"
	Validating State = "VALIDATING"
	Publishing State = "PUBLISHING"
	Done       State = "DONE"
	WaitUser   State = "WAIT_USER"
	WaitQuota  State = "WAIT_QUOTA"
	WaitAuth   State = "WAIT_AUTH"
	Failed     State = "FAILED"
	Aborted    State = "ABORTED"
)

func (s State) Terminal() bool { return s == Done || s == Aborted }

// Blocking states keep the whole queue stopped; so does FAILED.
func (s State) Waiting() bool { return s == WaitUser || s == WaitQuota || s == WaitAuth }

var allowed = map[State][]State{
	Ready:      {Running, WaitUser, Failed, Aborted},
	Running:    {Review, Validating, WaitUser, WaitQuota, WaitAuth, Failed, Aborted},
	Review:     {Running, Validating, WaitUser, WaitQuota, WaitAuth, Failed, Aborted},
	Validating: {Review, Running, Publishing, WaitUser, WaitQuota, WaitAuth, Failed, Aborted},
	Publishing: {Review, Running, Done, WaitUser, WaitQuota, WaitAuth, Failed, Aborted},
	WaitUser:   {Running, Review, Validating, Publishing, WaitQuota, WaitAuth, Failed, Aborted},
	WaitQuota:  {Running, Review, Validating, Publishing, WaitUser, WaitAuth, Failed, Aborted},
	WaitAuth:   {Running, Review, Validating, Publishing, WaitUser, WaitQuota, Failed, Aborted},
	Failed:     {Running, Review, Aborted},
}

func CanTransition(from, to State) bool {
	if from == to {
		return true
	}
	for _, s := range allowed[from] {
		if s == to {
			return true
		}
	}
	return false
}

type Session struct {
	Name       string    `json:"name,omitempty"`
	Log        string    `json:"log,omitempty"`
	Status     string    `json:"status,omitempty"` // none|launching|running|stopped|ambiguous
	LaunchedAt time.Time `json:"launched_at,omitempty"`
	LogOffset  int64     `json:"log_offset,omitempty"`
	LastGrowth time.Time `json:"last_growth,omitempty"`
	LastSize   int64     `json:"last_size,omitempty"`
	PID        int       `json:"pid,omitempty"`       // pane leader recorded at launch
	PIDStart   uint64    `json:"pid_start,omitempty"` // its /proc start time
	// Brief delivery to an interactive agent: trust -> ready -> pasted -> entered -> running.
	Stage         string    `json:"stage,omitempty"`
	StageAt       time.Time `json:"stage_at,omitempty"`
	StageSize     int64     `json:"stage_size,omitempty"`
	TrustAnswered bool      `json:"trust_answered,omitempty"`
	EnterRetried  bool      `json:"enter_retried,omitempty"`
}

// Turn is one deterministic `codex exec` / `exec resume` run inside tmux. The
// supervisor decides when to start it; completion is the exit file, not the pane.
type Turn struct {
	Status    string    `json:"status,omitempty"` // idle|launching|running|ambiguous
	Session   string    `json:"session,omitempty"`
	Log       string    `json:"log,omitempty"`
	ExitFile  string    `json:"exit_file,omitempty"`
	LastMsg   string    `json:"last_msg,omitempty"`
	CodexID   string    `json:"codex_session_id,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	PID       int       `json:"pid,omitempty"`
	PIDStart  uint64    `json:"pid_start,omitempty"`
	PrevSeq   int       `json:"prev_seq"`  // delivered cursor before this turn (redelivery on failure)
	Delivered int       `json:"delivered"` // highest event seq in this turn's prompt
	Applied   int       `json:"applied"`   // commands applied while this gen was current
}

type Coder struct {
	Adapter       string    `json:"adapter,omitempty"`
	Kind          string    `json:"kind,omitempty"` // local|cloud
	Gen           int       `json:"gen"`
	Session       Session   `json:"session"`
	CloudID       string    `json:"cloud_id,omitempty"`
	CloudURL      string    `json:"cloud_url,omitempty"`
	Queued        int       `json:"queued_messages,omitempty"`
	Brief         string    `json:"brief,omitempty"`      // coordinator brief body (stable across relaunch)
	BriefFile     string    `json:"brief_file,omitempty"` // wrapped brief for this gen
	Done          bool      `json:"done"`
	Pending       []string  `json:"pending,omitempty"` // feedback files waiting for an idle coder
	PendingFail   string    `json:"pending_fail,omitempty"`
	PendingFailAt time.Time `json:"pending_fail_at,omitempty"`
}

type Lease struct {
	Holder        string    `json:"holder,omitempty"` // ""|local|cloud
	Gen           int       `json:"gen"`
	StopConfirmed bool      `json:"stop_confirmed"`
	Evidence      string    `json:"evidence,omitempty"`
	Since         time.Time `json:"since,omitempty"`
}

type Wake struct {
	At       time.Time `json:"at"`
	Role     string    `json:"role"` // coordinator|coder|both
	Class    string    `json:"class"`
	Attempt  int       `json:"attempt"`
	Reason   string    `json:"reason"`
	Computed string    `json:"computed"` // reset|backoff
}

type Question struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"` // ask|cloud-completion|launch|stop-failed|cloud-writer|agent-not-ready|other
	Text      string    `json:"text"`
	CommentID int64     `json:"comment_id,omitempty"`
	AskedAt   time.Time `json:"asked_at"`
	Answered  bool      `json:"answered"`
	Answer    string    `json:"answer,omitempty"`
	By        string    `json:"by,omitempty"`
}

type Intent struct {
	Status string `json:"status"` // pending|done
	Detail string `json:"detail,omitempty"`
}

type Event struct {
	Seq       int       `json:"seq"`
	Kind      string    `json:"kind"`
	Text      string    `json:"text"`
	At        time.Time `json:"at"`
	Role      string    `json:"role,omitempty"`
	Gen       int       `json:"gen,omitempty"`
	Delivered bool      `json:"delivered"`
}

type Command struct {
	ID        string              `json:"id"`
	Kind      string              `json:"kind"`
	Role      string              `json:"role"`
	Run       string              `json:"run"`
	Gen       int                 `json:"gen"`
	Args      map[string]string   `json:"args,omitempty"`
	Lists     map[string][]string `json:"lists,omitempty"`
	CreatedAt time.Time           `json:"created_at"`
	Status    string              `json:"status,omitempty"` // pending|applied|rejected
	Result    string              `json:"result,omitempty"`
}

type CheckResult struct {
	Name   string    `json:"name"`
	Cmd    string    `json:"cmd"`
	Exit   int       `json:"exit"`
	Log    string    `json:"log"`
	Hash   string    `json:"hash"`
	Head   string    `json:"head"`
	At     time.Time `json:"at"`
	Passed bool      `json:"passed"`
}

type Acceptance struct {
	Hash   string    `json:"hash"`
	Note   string    `json:"note"`
	Checks []string  `json:"checks"`
	At     time.Time `json:"at"`
	Gen    int       `json:"gen"`
}

type EvidenceState struct {
	Required     string   `json:"required,omitempty"` // browser when the Issue demands a browser scenario
	Mode         string   `json:"mode,omitempty"`     // browser|cli
	Reason       string   `json:"reason,omitempty"`
	Dir          string   `json:"dir,omitempty"`
	Adapter      string   `json:"adapter,omitempty"` // real|fake
	Manifest     string   `json:"manifest,omitempty"`
	ManifestHash string   `json:"manifest_hash,omitempty"`
	Links        []string `json:"links,omitempty"`
	Published    bool     `json:"published"`
}

type Handoff struct {
	Status       string `json:"status"` // await-push|ready|created
	Branch       string `json:"branch"`
	CloudBranch  string `json:"cloud_branch"`
	SHA          string `json:"sha,omitempty"`
	ConfirmedSHA string `json:"confirmed_sha,omitempty"` // user-attested cloud completion
}

type Publish struct {
	Commit      string    `json:"commit,omitempty"`
	PR          string    `json:"pr,omitempty"`
	Attempts    int       `json:"attempts"`
	NextAttempt time.Time `json:"next_attempt,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
	AwaitCommit bool      `json:"await_commit"`
	CommitAsked bool      `json:"commit_asked"`
}

// CloudRun tracks an automatic cloud session. Completion is learned only from
// a validated idle report on the reserved path of the cloud branch; the report
// is a trusted agent's acknowledgment, never an observed process stop.
type CloudRun struct {
	Mode       string    `json:"mode"`
	SessionID  string    `json:"session_id"`
	URL        string    `json:"url,omitempty"`
	Branch     string    `json:"branch"`
	Gen        int       `json:"gen"`               // current round (report generation awaited)
	RoundBase  string    `json:"round_base"`        // sha the current round started from
	Status     string    `json:"status"`            // running|reported|quota-wait|send-ambiguous
	Outcome    string    `json:"outcome,omitempty"` // done|blocked of the last valid report
	PinnedHead string    `json:"pinned_head,omitempty"`
	PinnedCode string    `json:"pinned_code,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	NextPoll   time.Time `json:"next_poll,omitempty"`
	LastReject string    `json:"last_reject,omitempty"`
	// A follow-up in flight. Everything needed to recognise its report and to
	// resend exactly the same message is persisted BEFORE the external send.
	SendGen  int       `json:"send_gen,omitempty"`
	SendMsg  string    `json:"send_msg,omitempty"`  // file with the exact message
	SendBase string    `json:"send_base,omitempty"` // round base once that generation starts
	SendAt   time.Time `json:"send_at,omitempty"`
	Reports  int       `json:"reports"`
}

type Progress struct {
	NextDigestAt  time.Time `json:"next_digest_at,omitempty"`
	NextPollAt    time.Time `json:"next_poll_at,omitempty"`
	CommentCursor int64     `json:"comment_cursor"`
	CloudSince    time.Time `json:"cloud_since,omitempty"`
	LastDigestSig string    `json:"last_digest_sig,omitempty"`
	BodyHashSeen  string    `json:"body_hash_seen,omitempty"`
}

type Task struct {
	Schema    int    `json:"schema"`
	Issue     int    `json:"issue"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	BodyHash  string `json:"body_hash"`
	Author    string `json:"author"`
	RunID     string `json:"run_id"`
	State     State  `json:"state"`
	PrevState State  `json:"prev_state,omitempty"`
	Worktree  string `json:"worktree"`
	Branch    string `json:"branch"`
	BaseSHA   string `json:"base_sha"`
	LocalOnly bool   `json:"local_only"`

	CoordGen       int       `json:"coord_gen"`   // turn counter; each coordinator turn gets a new gen
	CoordEpoch     int       `json:"coord_epoch"` // first gen whose commands are still valid
	CoordSeq       int       `json:"coord_seq"`   // highest inbox event delivered to a turn
	Coordinator    Turn      `json:"coordinator"`
	Stalls         int       `json:"stalls"`
	AbortWhy       string    `json:"abort_why,omitempty"`
	ParkedWriters  bool      `json:"parked_writers,omitempty"`
	Cloud          *CloudRun `json:"cloud,omitempty"`
	AllowExternal  bool      `json:"allow_external,omitempty"`
	CoordCtxResets int       `json:"coord_ctx_resets"`
	CoderCtxResets int       `json:"coder_ctx_resets"`
	Coder          Coder     `json:"coder"`
	CoderDone      bool      `json:"-"`
	Lease          Lease     `json:"lease"`

	Wake      *Wake              `json:"wake,omitempty"`
	Attempts  map[string]int     `json:"attempts,omitempty"` // quota/transient attempts per role
	Question  *Question          `json:"question,omitempty"`
	Intents   map[string]*Intent `json:"intents,omitempty"`
	Events    []Event            `json:"events,omitempty"`
	NextSeq   int                `json:"next_seq"`
	Commands  []Command          `json:"commands,omitempty"`
	Applied   map[string]bool    `json:"applied,omitempty"`
	Checks    []CheckResult      `json:"checks,omitempty"`
	Accept    *Acceptance        `json:"accept,omitempty"`
	Evidence  EvidenceState      `json:"evidence"`
	Handoff   *Handoff           `json:"handoff,omitempty"`
	Publish   Publish            `json:"publish"`
	Progress  Progress           `json:"progress"`
	LastError string             `json:"last_error,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}

func (t *Task) SetState(to State, now time.Time) error {
	if !CanTransition(t.State, to) {
		return fmt.Errorf("illegal transition %s -> %s", t.State, to)
	}
	t.State = to
	t.UpdatedAt = now
	return nil
}

func (t *Task) AddEvent(kind, text, role string, gen int, now time.Time) {
	t.NextSeq++
	t.Events = append(t.Events, Event{Seq: t.NextSeq, Kind: kind, Text: text, At: now, Role: role, Gen: gen})
}

type Queue struct {
	Halted bool   `json:"halted"`
	Reason string `json:"reason,omitempty"`
	Issue  int    `json:"issue,omitempty"`
}
