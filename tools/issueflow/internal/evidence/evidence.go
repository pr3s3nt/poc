// Package evidence ingests, validates and publishes task evidence.
//
// Evidence is read only from a task-owned private directory, copied once into a
// private staging directory while hashing and scanning the copy, and only the
// staged copy is ever published. A manifest proves structure and integrity; it
// does not prove privacy: Codex must review frames, and that review is itself
// an input here, bound to the video hash.
package evidence

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"issueflow/internal/runner"
)

const (
	maxTextBytes  = 256 << 20
	maxVideoBytes = int64(1900) << 20
)

type File struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"` // video|captions|assertions|phases|frame-review|check-log|log
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	AssetName string `json:"asset_name"`
	Staged    string `json:"-"`
}

type Manifest struct {
	Issue       int       `json:"issue"`
	RunID       string    `json:"run_id"`
	Mode        string    `json:"mode"` // browser|cli
	Reason      string    `json:"reason,omitempty"`
	Adapter     string    `json:"adapter,omitempty"` // real|fake
	ContentHash string    `json:"content_hash,omitempty"`
	Bound       bool      `json:"bound"` // content hash verified against evidence files
	Commit      string    `json:"commit,omitempty"`
	Created     time.Time `json:"created"`
	Files       []File    `json:"files"`
	Duration    float64   `json:"video_duration_seconds,omitempty"`
	Decoded     bool      `json:"video_fully_decoded"`
	Headed      bool      `json:"headed,omitempty"`
	FrameReview bool      `json:"frame_review,omitempty"`
	ScanNote    string    `json:"scan_note"`
}

func (m *Manifest) Bytes() []byte {
	b, _ := json.MarshalIndent(m, "", "  ")
	return b
}

func (m *Manifest) Hash() string {
	s := sha256.Sum256(m.Bytes())
	return hex.EncodeToString(s[:])
}

var secretRes = []*regexp.Regexp{
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{30,}`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{30,}`),
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)client-(key|certificate)-data:\s*\S{20,}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`(?i)(password|passwd|secret|token|api[_-]?key)["']?\s*[:=]\s*["']?[A-Za-z0-9/+_=.-]{12,}`),
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._-]{20,}`),
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`),
	regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`),
}

// ScanText is a best-effort pattern scan. A clean result is not proof.
func ScanText(b []byte) []string {
	var hits []string
	for _, re := range secretRes {
		if re.Match(b) {
			hits = append(hits, re.String())
		}
	}
	return hits
}

const scanChunk = 1 << 20
const scanOverlap = 1024

// scanStream scans the whole stream in overlapping chunks and fails closed on
// read errors. Returns the first hit pattern.
func scanStream(r io.Reader) (string, error) {
	buf := make([]byte, scanChunk+scanOverlap)
	have := 0
	for {
		n, err := r.Read(buf[have : have+scanChunk])
		have += n
		if have > 0 {
			if hits := ScanText(buf[:have]); len(hits) > 0 {
				return hits[0], nil
			}
		}
		if err == io.EOF {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		keep := have
		if keep > scanOverlap {
			keep = scanOverlap
		}
		copy(buf, buf[have-keep:have])
		have = keep
	}
}

var allowedExt = map[string]string{
	".mp4": "video", ".webm": "video",
	".srt": "captions", ".vtt": "captions",
	".json": "json", ".log": "log", ".txt": "log",
}

// Confine returns dir resolved inside root, refusing symlinks and traversal.
func Confine(root, dir string) (string, error) {
	rr, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("evidence root: %w", err)
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(rr, dir)
	}
	li, err := os.Lstat(dir)
	if err != nil {
		return "", fmt.Errorf("evidence dir: %w", err)
	}
	if li.Mode()&os.ModeSymlink != 0 || !li.IsDir() {
		return "", fmt.Errorf("evidence dir must be a real directory, not a symlink: %s", dir)
	}
	rd, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rr, rd)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("evidence dir %s is outside the task evidence area %s", dir, rr)
	}
	return rd, nil
}

type Validator struct {
	R runner.Runner
}

// LogRef is a supervisor-run check log attached to every manifest.
type LogRef struct{ Name, Path string }

type Spec struct {
	Issue       int
	RunID       string
	Mode        string // browser|cli
	Reason      string
	Adapter     string
	Root        string // task-owned private evidence area (confinement root)
	InDir       string // directory under Root with coder/coordinator evidence
	StageDir    string // private staging directory, recreated per Build
	ContentHash string // accepted content hash; "" = structure-only (unbound)
	MaxGen      int
	Commit      string
	Checks      []LogRef
}

// Build ingests evidence into StageDir, validates it and returns the manifest.
func (v Validator) Build(ctx context.Context, sp Spec, now time.Time) (*Manifest, error) {
	m := &Manifest{Issue: sp.Issue, RunID: sp.RunID, Mode: sp.Mode, Reason: sp.Reason, Adapter: sp.Adapter,
		ContentHash: sp.ContentHash, Bound: sp.ContentHash != "", Commit: sp.Commit, Created: now.UTC(),
		ScanNote: "pattern scan of all staged bytes except video frames; frames need Codex review"}
	if sp.Mode != "cli" && sp.Mode != "browser" {
		return nil, fmt.Errorf("evidence mode must be browser or cli")
	}
	if err := os.RemoveAll(sp.StageDir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(sp.StageDir, 0o700); err != nil {
		return nil, err
	}
	byName := map[string]*File{}
	if sp.InDir != "" {
		dir, err := Confine(sp.Root, sp.InDir)
		if err != nil {
			return nil, err
		}
		ents, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, e := range ents {
			p := filepath.Join(dir, e.Name())
			li, err := os.Lstat(p)
			if err != nil {
				return nil, err
			}
			if li.Mode()&os.ModeSymlink != 0 || !li.Mode().IsRegular() {
				return nil, fmt.Errorf("evidence entry %s must be a regular file (no symlinks or subdirectories)", e.Name())
			}
			kind, ok := allowedExt[strings.ToLower(filepath.Ext(e.Name()))]
			if !ok {
				return nil, fmt.Errorf("evidence file %s has a type that is not allowed", e.Name())
			}
			switch e.Name() {
			case "assertions.json":
				kind = "assertions"
			case "phases.json":
				kind = "phases"
			case "frame-review.json":
				kind = "frame-review"
			default:
				if kind == "json" {
					return nil, fmt.Errorf("unexpected json evidence file %s", e.Name())
				}
			}
			f, err := stage(p, sp.StageDir, e.Name(), kind, li.Size())
			if err != nil {
				return nil, err
			}
			m.Files = append(m.Files, *f)
		}
	}
	for _, c := range sp.Checks {
		li, err := os.Lstat(c.Path)
		if err != nil || !li.Mode().IsRegular() {
			return nil, fmt.Errorf("check log %s missing or not a regular file", c.Name)
		}
		f, err := stage(c.Path, sp.StageDir, "check-"+safeName(c.Name)+".log", "check-log", li.Size())
		if err != nil {
			return nil, err
		}
		m.Files = append(m.Files, *f)
	}
	for i := range m.Files {
		byName[m.Files[i].Name] = &m.Files[i]
	}
	var checkLogs int
	for _, f := range m.Files {
		if f.Kind == "check-log" {
			checkLogs++
		}
	}
	if checkLogs == 0 {
		return nil, fmt.Errorf("no check logs: evidence must include the supervisor's actual check logs")
	}
	switch sp.Mode {
	case "cli":
		if strings.TrimSpace(sp.Reason) == "" {
			return nil, fmt.Errorf("cli-only evidence needs a reason why no browser video exists")
		}
		for _, f := range m.Files {
			if f.Kind == "video" {
				return nil, fmt.Errorf("cli-only task must not carry video evidence (%s)", f.Name)
			}
		}
	case "browser":
		if err := v.checkBrowser(ctx, sp, m, byName); err != nil {
			return nil, err
		}
	}
	sort.Slice(m.Files, func(i, j int) bool { return m.Files[i].Name < m.Files[j].Name })
	return m, nil
}

func safeName(s string) string { return unsafeName.ReplaceAllString(s, "_") }

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// stage copies src into the staging dir while hashing and scanning the copy in
// one pass, so what was scanned is exactly what was staged.
func stage(src, stageDir, name, kind string, size int64) (*File, error) {
	limit := maxTextBytes
	if kind == "video" {
		if size == 0 {
			return nil, fmt.Errorf("video %s is empty", name)
		}
		if size > maxVideoBytes {
			return nil, fmt.Errorf("video %s exceeds the size limit", name)
		}
	} else if size > int64(limit) {
		return nil, fmt.Errorf("text evidence %s is too large to scan (%d bytes); refusing", name, size)
	}
	in, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer in.Close()
	dst := filepath.Join(stageDir, safeName(name))
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("stage %s: %w", name, err)
	}
	h := sha256.New()
	var pr io.Reader = in
	var scanW *io.PipeWriter
	scanDone := make(chan struct{})
	var scanHit string
	var scanErr error
	if kind != "video" {
		pr0, pw := io.Pipe()
		scanW = pw
		go func() {
			scanHit, scanErr = scanStream(pr0)
			io.Copy(io.Discard, pr0) // keep the writer unblocked after a hit
			close(scanDone)
		}()
		pr = io.TeeReader(in, pw)
	}
	n, cerr := io.Copy(io.MultiWriter(out, h), pr)
	if scanW != nil {
		scanW.Close()
		<-scanDone
	}
	if cerr == nil {
		cerr = out.Close()
	} else {
		out.Close()
	}
	if cerr != nil {
		os.Remove(dst)
		return nil, fmt.Errorf("stage %s: %w", name, cerr)
	}
	if scanErr != nil {
		os.Remove(dst)
		return nil, fmt.Errorf("scan %s failed closed: %w", name, scanErr)
	}
	if scanHit != "" {
		os.Remove(dst)
		return nil, fmt.Errorf("possible secret in %s (pattern %s)", name, scanHit)
	}
	sum := hex.EncodeToString(h.Sum(nil))
	return &File{Name: name, Kind: kind, Size: n, SHA256: sum, Staged: dst,
		AssetName: sum[:16] + "-" + safeName(name)}, nil
}

type assertionsDoc struct {
	RunID       string `json:"run_id"`
	ContentHash string `json:"content_hash"`
	Gen         int    `json:"gen"`
	Adapter     string `json:"adapter"`
	Headed      bool   `json:"headed"`
	Overall     string `json:"overall"`
	Assertions  []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Detail string `json:"detail"`
	} `json:"assertions"`
}

type phasesDoc struct {
	RunID       string  `json:"run_id"`
	ContentHash string  `json:"content_hash"`
	Duration    float64 `json:"duration_seconds"`
	Phases      []struct {
		Name  string  `json:"name"`
		Start float64 `json:"start"`
		End   float64 `json:"end"`
	} `json:"phases"`
}

type frameReviewDoc struct {
	Reviewer    string `json:"reviewer"`
	RunID       string `json:"run_id"`
	ContentHash string `json:"content_hash"`
	VideoSHA256 string `json:"video_sha256"`
	PrivacyOK   bool   `json:"privacy_ok"`
	Frames      []struct {
		T    float64 `json:"t"`
		Note string  `json:"note"`
		OK   bool    `json:"ok"`
	} `json:"frames"`
}

func readJSON(f *File, v any) error {
	b, err := os.ReadFile(f.Staged)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (v Validator) checkBrowser(ctx context.Context, sp Spec, m *Manifest, by map[string]*File) error {
	if sp.Adapter != "real" && sp.Adapter != "fake" {
		return fmt.Errorf("browser evidence must declare adapter real|fake")
	}
	var video *File
	var caps []*File
	for i := range m.Files {
		switch m.Files[i].Kind {
		case "video":
			if video != nil {
				return fmt.Errorf("exactly one video file is expected")
			}
			video = &m.Files[i]
		case "captions":
			caps = append(caps, &m.Files[i])
		}
	}
	as, ph, fr := by["assertions.json"], by["phases.json"], by["frame-review.json"]
	for name, f := range map[string]*File{"video": video, "assertions.json": as, "phases.json": ph, "frame-review.json": fr} {
		if f == nil {
			return fmt.Errorf("browser evidence is missing %s", name)
		}
	}
	if len(caps) == 0 {
		return fmt.Errorf("browser evidence is missing captions")
	}
	bind := func(doc, runID, hash string) error {
		if runID != sp.RunID {
			return fmt.Errorf("%s belongs to run %q, not %q", doc, runID, sp.RunID)
		}
		if sp.ContentHash != "" && hash != sp.ContentHash {
			return fmt.Errorf("%s is bound to content %.12s, accepted content is %.12s", doc, hash, sp.ContentHash)
		}
		if hash == "" {
			return fmt.Errorf("%s has no content_hash", doc)
		}
		return nil
	}
	var a assertionsDoc
	if err := readJSON(as, &a); err != nil {
		return fmt.Errorf("assertions.json: %w", err)
	}
	if err := bind("assertions.json", a.RunID, a.ContentHash); err != nil {
		return err
	}
	if a.Gen < 1 || (sp.MaxGen > 0 && a.Gen > sp.MaxGen) {
		return fmt.Errorf("assertions.json gen %d is not a generation of this run", a.Gen)
	}
	if a.Adapter != sp.Adapter {
		return fmt.Errorf("assertions.json adapter %q differs from declared %q", a.Adapter, sp.Adapter)
	}
	if !a.Headed {
		return fmt.Errorf("assertions.json does not state a headed browser run")
	}
	if len(a.Assertions) == 0 || a.Overall != "pass" {
		return fmt.Errorf("assertions must be non-empty with overall \"pass\" (overall=%q)", a.Overall)
	}
	for _, x := range a.Assertions {
		if x.Name == "" || x.Status != "pass" {
			return fmt.Errorf("assertion %q has status %q", x.Name, x.Status)
		}
	}
	m.Headed = true
	var p phasesDoc
	if err := readJSON(ph, &p); err != nil {
		return fmt.Errorf("phases.json: %w", err)
	}
	if err := bind("phases.json", p.RunID, p.ContentHash); err != nil {
		return err
	}
	if len(p.Phases) == 0 || p.Duration <= 0 {
		return fmt.Errorf("phases.json needs phases and a positive duration")
	}
	last := 0.0
	for _, x := range p.Phases {
		if x.Name == "" || x.Start < 0 || x.End <= x.Start || x.Start < last-1e-6 || x.End > p.Duration+0.5 {
			return fmt.Errorf("phase %q has invalid times [%v,%v] for duration %v", x.Name, x.Start, x.End, p.Duration)
		}
		last = x.End
	}
	dur, err := v.probe(ctx, video.Staged)
	if err != nil {
		return err
	}
	if math.Abs(dur-p.Duration) > 2.0 {
		return fmt.Errorf("phases duration %.1fs does not match video duration %.1fs", p.Duration, dur)
	}
	m.Duration, m.Decoded = dur, true
	for _, c := range caps {
		if err := checkCaptions(c, dur); err != nil {
			return err
		}
	}
	var r frameReviewDoc
	if err := readJSON(fr, &r); err != nil {
		return fmt.Errorf("frame-review.json: %w", err)
	}
	if err := bind("frame-review.json", r.RunID, r.ContentHash); err != nil {
		return err
	}
	if r.Reviewer != "codex" || !r.PrivacyOK || r.VideoSHA256 != video.SHA256 {
		return fmt.Errorf("frame review must be by codex, privacy_ok, and bound to this video's sha256")
	}
	if len(r.Frames) < len(p.Phases) {
		return fmt.Errorf("frame review has %d frames for %d phases", len(r.Frames), len(p.Phases))
	}
	for _, f := range r.Frames {
		if !f.OK || strings.TrimSpace(f.Note) == "" || f.T < 0 || f.T > dur+0.5 {
			return fmt.Errorf("frame review entry at %.1fs is not acceptable", f.T)
		}
	}
	m.FrameReview = true
	return nil
}

var (
	cueRe = regexp.MustCompile(`(\d+):(\d\d):(\d\d)[,.](\d{3})\s*-->\s*(\d+):(\d\d):(\d\d)[,.](\d{3})`)
)

func secs(h, m, s, ms string) float64 {
	a, _ := strconv.Atoi(h)
	b, _ := strconv.Atoi(m)
	c, _ := strconv.Atoi(s)
	d, _ := strconv.Atoi(ms)
	return float64(a*3600+b*60+c) + float64(d)/1000
}

func checkCaptions(f *File, dur float64) error {
	b, err := os.ReadFile(f.Staged)
	if err != nil {
		return err
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	var cues int
	var text strings.Builder
	inCue, hasText := false, false
	for sc.Scan() {
		line := sc.Text()
		if mm := cueRe.FindStringSubmatch(line); mm != nil {
			st, en := secs(mm[1], mm[2], mm[3], mm[4]), secs(mm[5], mm[6], mm[7], mm[8])
			if en <= st || en > dur+1 {
				return fmt.Errorf("caption cue [%v,%v] is invalid for a %.1fs video", st, en, dur)
			}
			inCue, hasText = true, false
			cues++
			continue
		}
		if inCue && strings.TrimSpace(line) != "" {
			hasText = true
			text.WriteString(line)
			continue
		}
		if inCue && strings.TrimSpace(line) == "" {
			if !hasText {
				return fmt.Errorf("caption file %s has an empty cue", f.Name)
			}
			inCue = false
		}
	}
	if inCue && !hasText {
		return fmt.Errorf("caption file %s has an empty cue", f.Name)
	}
	if cues == 0 {
		return fmt.Errorf("caption file %s has no cues", f.Name)
	}
	// Language is a preference (Vietnamese), not a gate: content is reviewed by Codex.
	if strings.TrimSpace(text.String()) == "" {
		return fmt.Errorf("caption file %s has no text", f.Name)
	}
	return nil
}

// probe runs ffprobe and a full ffmpeg decode; both are always required.
func (v Validator) probe(ctx context.Context, path string) (float64, error) {
	res, err := v.R.Run(ctx, "", nil, "ffprobe", "-v", "error", "-show_entries", "format=duration:stream=codec_type,width,height", "-of", "json", path)
	if err != nil || res.Exit != 0 {
		return 0, fmt.Errorf("ffprobe failed: %v %s", err, strings.TrimSpace(res.Stderr))
	}
	var pr struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			Type   string `json:"codec_type"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"streams"`
	}
	if err := json.Unmarshal([]byte(res.Stdout), &pr); err != nil {
		return 0, fmt.Errorf("ffprobe output: %w", err)
	}
	dur, _ := strconv.ParseFloat(pr.Format.Duration, 64)
	if dur <= 0 {
		return 0, fmt.Errorf("video has no duration")
	}
	okVideo := false
	for _, s := range pr.Streams {
		if s.Type == "video" && s.Width >= 640 && s.Height >= 360 {
			okVideo = true
		}
	}
	if !okVideo {
		return 0, fmt.Errorf("video stream missing or below 640x360")
	}
	d, err := v.R.Run(ctx, "", nil, "ffmpeg", "-hide_banner", "-v", "error", "-xerror", "-i", path, "-f", "null", "-")
	if err != nil || d.Exit != 0 || strings.TrimSpace(d.Stderr) != "" {
		return 0, fmt.Errorf("video does not decode cleanly: %v %s", err, strings.TrimSpace(d.Stderr))
	}
	return dur, nil
}

// Preflight is the cheap structural check done when evidence is declared:
// confinement, entry types and the presence of the files a mode needs. It
// stages nothing and proves nothing about content; Build does that at publish.
func Preflight(root, inDir, mode, reason string) ([]string, error) {
	if mode != "cli" && mode != "browser" {
		return nil, fmt.Errorf("evidence mode must be browser or cli")
	}
	if mode == "cli" && strings.TrimSpace(reason) == "" {
		return nil, fmt.Errorf("cli-only evidence needs a reason why no browser video exists")
	}
	if inDir == "" {
		if mode == "browser" {
			return nil, fmt.Errorf("browser evidence needs a directory")
		}
		return nil, nil
	}
	dir, err := Confine(root, inDir)
	if err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	var names []string
	for _, e := range ents {
		li, err := os.Lstat(filepath.Join(dir, e.Name()))
		if err != nil || li.Mode()&os.ModeSymlink != 0 || !li.Mode().IsRegular() {
			return nil, fmt.Errorf("evidence entry %s must be a regular file (no symlinks or subdirectories)", e.Name())
		}
		kind, ok := allowedExt[strings.ToLower(filepath.Ext(e.Name()))]
		if !ok {
			return nil, fmt.Errorf("evidence file %s has a type that is not allowed", e.Name())
		}
		have[kind] = true
		have[e.Name()] = true
		names = append(names, e.Name())
	}
	if mode == "cli" && have["video"] {
		return nil, fmt.Errorf("cli-only task must not carry video evidence")
	}
	if mode == "browser" {
		for _, need := range []string{"video", "captions", "assertions.json", "phases.json"} {
			if !have[need] {
				return nil, fmt.Errorf("browser evidence is missing %s", need)
			}
		}
	}
	return names, nil
}
