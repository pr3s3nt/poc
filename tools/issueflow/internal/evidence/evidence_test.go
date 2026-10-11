package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"issueflow/internal/gh"
	"issueflow/internal/runner"
)

const hashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type rig struct {
	t     *testing.T
	root  string
	in    string
	stage string
	logs  []LogRef
	rt    *runner.Router
	v     Validator
	video []byte
}

func newRig(t *testing.T) *rig {
	root := t.TempDir()
	r := &rig{t: t, root: root, in: filepath.Join(root, "in"), stage: filepath.Join(root, "stage"), video: []byte("not really a video but non empty")}
	os.MkdirAll(r.in, 0o700)
	logp := filepath.Join(root, "unit.log")
	os.WriteFile(logp, []byte("$ go test\nok\nexit 0\n"), 0o600)
	r.logs = []LogRef{{Name: "unit", Path: logp}}
	r.rt = runner.NewRouter(runner.OS{})
	r.rt.Handle("ffprobe", func(c runner.Call) (runner.Result, error, bool) {
		return runner.Result{Stdout: `{"format":{"duration":"12.5"},"streams":[{"codec_type":"video","width":1440,"height":900}]}`}, nil, true
	})
	r.rt.Handle("ffmpeg", func(c runner.Call) (runner.Result, error, bool) { return runner.Result{}, nil, true })
	r.v = Validator{R: r.rt}
	return r
}

func (r *rig) write(name, content string) {
	r.t.Helper()
	if err := os.WriteFile(filepath.Join(r.in, name), []byte(content), 0o600); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) js(name string, v any) {
	b, _ := json.MarshalIndent(v, "", " ")
	r.write(name, string(b))
}

func (r *rig) validBrowser() {
	sum := sha256.Sum256(r.video)
	vsha := hex.EncodeToString(sum[:])
	r.write("run.mp4", string(r.video))
	r.write("captions.srt", "1\n00:00:01,000 --> 00:00:04,000\nBước 1: đăng nhập vào Web Console\n\n2\n00:00:05,000 --> 00:00:09,000\nBước 2: lưu cấu hình\n")
	r.js("assertions.json", map[string]any{"run_id": "r1", "content_hash": hashA, "gen": 1, "adapter": "real", "headed": true, "overall": "pass",
		"assertions": []map[string]string{{"name": "login visible", "status": "pass"}, {"name": "saved", "status": "pass"}}})
	r.js("phases.json", map[string]any{"run_id": "r1", "content_hash": hashA, "duration_seconds": 12.0,
		"phases": []map[string]any{{"name": "login", "start": 0.5, "end": 5.0}, {"name": "save", "start": 5.0, "end": 11.5}}})
	r.js("frame-review.json", map[string]any{"reviewer": "codex", "run_id": "r1", "content_hash": hashA, "video_sha256": vsha, "privacy_ok": true,
		"frames": []map[string]any{{"t": 2.0, "note": "login form, no secret", "ok": true}, {"t": 8.0, "note": "saved banner", "ok": true}}})
}

func (r *rig) spec(mode string) Spec {
	return Spec{Issue: 7, RunID: "r1", Mode: mode, Reason: "docs only", Adapter: "real", Root: r.root, InDir: r.in, StageDir: r.stage,
		ContentHash: hashA, MaxGen: 3, Commit: "abc", Checks: r.logs}
}

func (r *rig) build(mode string) (*Manifest, error) {
	return r.v.Build(context.Background(), r.spec(mode), time.Now())
}

func TestBrowserEvidenceAccepted(t *testing.T) {
	r := newRig(t)
	r.validBrowser()
	m, err := r.build("browser")
	if err != nil {
		t.Fatal(err)
	}
	if !m.Decoded || !m.Headed || !m.FrameReview || !m.Bound || m.Duration != 12.5 {
		t.Fatalf("manifest %+v", m)
	}
	kinds := map[string]bool{}
	for _, f := range m.Files {
		kinds[f.Kind] = true
		if !strings.HasPrefix(f.AssetName, f.SHA256[:16]+"-") {
			t.Errorf("asset name not content-addressed: %s", f.AssetName)
		}
	}
	for _, k := range []string{"video", "captions", "assertions", "phases", "frame-review", "check-log"} {
		if !kinds[k] {
			t.Errorf("manifest lacks %s", k)
		}
	}
	if len(r.rt.CallsTo("ffprobe")) != 1 || len(r.rt.CallsTo("ffmpeg")) != 1 {
		t.Fatal("ffprobe and a full ffmpeg decode are always required")
	}
}

func TestBrowserEvidenceFailures(t *testing.T) {
	cases := map[string]struct {
		mut  func(r *rig)
		want string
	}{
		"empty assertions": {func(r *rig) {
			r.js("assertions.json", map[string]any{"run_id": "r1", "content_hash": hashA, "gen": 1, "adapter": "real", "headed": true, "overall": "pass", "assertions": []any{}})
		}, "non-empty"},
		"failing assertion": {func(r *rig) {
			r.js("assertions.json", map[string]any{"run_id": "r1", "content_hash": hashA, "gen": 1, "adapter": "real", "headed": true, "overall": "pass",
				"assertions": []map[string]string{{"name": "saved", "status": "fail"}}})
		}, "status \"fail\""},
		"overall fail": {func(r *rig) {
			r.js("assertions.json", map[string]any{"run_id": "r1", "content_hash": hashA, "gen": 1, "adapter": "real", "headed": true, "overall": "fail",
				"assertions": []map[string]string{{"name": "saved", "status": "pass"}}})
		}, "overall"},
		"not headed": {func(r *rig) {
			r.js("assertions.json", map[string]any{"run_id": "r1", "content_hash": hashA, "gen": 1, "adapter": "real", "headed": false, "overall": "pass",
				"assertions": []map[string]string{{"name": "saved", "status": "pass"}}})
		}, "headed"},
		"wrong content hash": {func(r *rig) {
			r.js("phases.json", map[string]any{"run_id": "r1", "content_hash": strings.Repeat("b", 64), "duration_seconds": 12.0,
				"phases": []map[string]any{{"name": "p", "start": 0.5, "end": 5.0}}})
		}, "bound to content"},
		"wrong run": {func(r *rig) {
			r.js("phases.json", map[string]any{"run_id": "other", "content_hash": hashA, "duration_seconds": 12.0,
				"phases": []map[string]any{{"name": "p", "start": 0.5, "end": 5.0}}})
		}, "belongs to run"},
		"future generation": {func(r *rig) {
			r.js("assertions.json", map[string]any{"run_id": "r1", "content_hash": hashA, "gen": 99, "adapter": "real", "headed": true, "overall": "pass",
				"assertions": []map[string]string{{"name": "saved", "status": "pass"}}})
		}, "generation"},
		"phase beyond video": {func(r *rig) {
			r.js("phases.json", map[string]any{"run_id": "r1", "content_hash": hashA, "duration_seconds": 12.0,
				"phases": []map[string]any{{"name": "p", "start": 0.5, "end": 40.0}}})
		}, "invalid times"},
		"overlapping phases": {func(r *rig) {
			r.js("phases.json", map[string]any{"run_id": "r1", "content_hash": hashA, "duration_seconds": 12.0,
				"phases": []map[string]any{{"name": "a", "start": 0, "end": 6}, {"name": "b", "start": 3, "end": 9}}})
		}, "invalid times"},
		"duration mismatch": {func(r *rig) {
			r.js("phases.json", map[string]any{"run_id": "r1", "content_hash": hashA, "duration_seconds": 3.0,
				"phases": []map[string]any{{"name": "a", "start": 0, "end": 2}}})
		}, "does not match video"},
		"empty captions": {func(r *rig) { r.write("captions.srt", "") }, "no cues"},
		"empty cue":      {func(r *rig) { r.write("captions.srt", "1\n00:00:01,000 --> 00:00:02,000\n\n") }, "empty cue"},
		"cue past video": {func(r *rig) {
			r.write("captions.srt", "1\n00:00:01,000 --> 00:00:40,000\nBước một\n")
		}, "invalid"},
		"no frame review": {func(r *rig) { os.Remove(filepath.Join(r.in, "frame-review.json")) }, "frame-review.json"},
		"frame review wrong video": {func(r *rig) {
			r.js("frame-review.json", map[string]any{"reviewer": "codex", "run_id": "r1", "content_hash": hashA, "video_sha256": strings.Repeat("c", 64), "privacy_ok": true,
				"frames": []map[string]any{{"t": 1.0, "note": "x", "ok": true}, {"t": 2.0, "note": "y", "ok": true}}})
		}, "bound to this video"},
		"frame review privacy not ok": {func(r *rig) {
			sum := sha256.Sum256(r.video)
			r.js("frame-review.json", map[string]any{"reviewer": "codex", "run_id": "r1", "content_hash": hashA, "video_sha256": hex.EncodeToString(sum[:]), "privacy_ok": false,
				"frames": []map[string]any{{"t": 1.0, "note": "x", "ok": true}, {"t": 2.0, "note": "y", "ok": true}}})
		}, "privacy_ok"},
		"too few frames": {func(r *rig) {
			sum := sha256.Sum256(r.video)
			r.js("frame-review.json", map[string]any{"reviewer": "codex", "run_id": "r1", "content_hash": hashA, "video_sha256": hex.EncodeToString(sum[:]), "privacy_ok": true,
				"frames": []map[string]any{{"t": 1.0, "note": "x", "ok": true}}})
		}, "frames for"},
		"reviewer not codex": {func(r *rig) {
			sum := sha256.Sum256(r.video)
			r.js("frame-review.json", map[string]any{"reviewer": "the coder", "run_id": "r1", "content_hash": hashA, "video_sha256": hex.EncodeToString(sum[:]), "privacy_ok": true,
				"frames": []map[string]any{{"t": 1.0, "note": "x", "ok": true}, {"t": 2.0, "note": "y", "ok": true}}})
		}, "by codex"},
		"empty video":     {func(r *rig) { r.write("run.mp4", "") }, "empty"},
		"extra file type": {func(r *rig) { r.write("notes.pdf", "x") }, "not allowed"},
		"unknown json":    {func(r *rig) { r.write("other.json", "{}") }, "unexpected json"},
		"secret in captions": {func(r *rig) {
			r.write("captions.srt", "1\n00:00:01,000 --> 00:00:04,000\nBước 1 token=ghp_abcdefghijklmnopqrstuvwxyz0123456789ab\n")
		}, "possible secret"},
		"second video": {func(r *rig) { r.write("two.webm", "more video") }, "exactly one video"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			r.validBrowser()
			tc.mut(r)
			if _, err := r.build("browser"); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestVideoMustDecodeCleanlyAndProbe(t *testing.T) {
	r := newRig(t)
	r.validBrowser()
	r.rt.Handle("ffmpeg", func(c runner.Call) (runner.Result, error, bool) {
		return runner.Result{Stderr: "error: truncated"}, nil, true
	})
	if _, err := r.build("browser"); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("truncated video accepted: %v", err)
	}
	r = newRig(t)
	r.validBrowser()
	r.rt.Handle("ffprobe", func(c runner.Call) (runner.Result, error, bool) {
		return runner.Result{Stdout: `{"format":{"duration":"12.5"},"streams":[{"codec_type":"video","width":320,"height":200}]}`}, nil, true
	})
	if _, err := r.build("browser"); err == nil || !strings.Contains(err.Error(), "640x360") {
		t.Fatalf("tiny video accepted: %v", err)
	}
	r = newRig(t)
	r.validBrowser()
	r.rt.Handle("ffprobe", func(c runner.Call) (runner.Result, error, bool) {
		return runner.Result{Exit: 1, Stderr: "boom"}, nil, true
	})
	if _, err := r.build("browser"); err == nil {
		t.Fatal("ffprobe failure ignored")
	}
}

func TestCLIEvidenceNeedsReasonLogsAndNoVideo(t *testing.T) {
	r := newRig(t)
	sp := r.spec("cli")
	sp.InDir = ""
	m, err := r.v.Build(context.Background(), sp, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Files) != 1 || m.Files[0].Kind != "check-log" || m.Decoded || m.Headed {
		t.Fatalf("cli manifest should carry the check log only: %+v", m)
	}
	sp.Reason = "  "
	if _, err := r.v.Build(context.Background(), sp, time.Now()); err == nil {
		t.Fatal("cli without a reason accepted")
	}
	sp = r.spec("cli")
	sp.Checks = nil
	if _, err := r.v.Build(context.Background(), sp, time.Now()); err == nil || !strings.Contains(err.Error(), "check logs") {
		t.Fatalf("cli without check logs accepted: %v", err)
	}
	sp = r.spec("cli")
	r.write("sneaky.mp4", "video")
	if _, err := r.v.Build(context.Background(), sp, time.Now()); err == nil || !strings.Contains(err.Error(), "must not carry video") {
		t.Fatalf("cli with a video accepted: %v", err)
	}
	// check logs are scanned, too
	os.Remove(filepath.Join(r.in, "sneaky.mp4"))
	os.WriteFile(r.logs[0].Path, []byte("export TOKEN=ghp_abcdefghijklmnopqrstuvwxyz0123456789ab\n"), 0o600)
	if _, err := r.v.Build(context.Background(), r.spec("cli"), time.Now()); err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("secret in a check log published: %v", err)
	}
}

func TestEvidenceConfinement(t *testing.T) {
	r := newRig(t)
	r.validBrowser()
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "credentials"), []byte("x"), 0o600)
	sp := r.spec("browser")
	sp.InDir = outside
	if _, err := r.v.Build(context.Background(), sp, time.Now()); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("outside dir accepted: %v", err)
	}
	sp.InDir = filepath.Join(r.in, "..", "..")
	if _, err := r.v.Build(context.Background(), sp, time.Now()); err == nil {
		t.Fatal("path traversal accepted")
	}
	// symlinked directory inside the area
	link := filepath.Join(r.root, "link")
	os.Symlink(outside, link)
	sp.InDir = link
	if _, err := r.v.Build(context.Background(), sp, time.Now()); err == nil {
		t.Fatal("symlinked evidence dir accepted")
	}
	// symlinked file inside a valid dir
	r2 := newRig(t)
	r2.validBrowser()
	os.Remove(filepath.Join(r2.in, "captions.srt"))
	os.Symlink(filepath.Join(outside, "credentials"), filepath.Join(r2.in, "captions.srt"))
	if _, err := r2.build("browser"); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("symlinked file accepted: %v", err)
	}
	// subdirectory
	r3 := newRig(t)
	r3.validBrowser()
	os.MkdirAll(filepath.Join(r3.in, "sub"), 0o700)
	if _, err := r3.build("browser"); err == nil {
		t.Fatal("subdirectory accepted")
	}
}

func TestLargeTextScannedBeyondFirstChunkFailsClosed(t *testing.T) {
	r := newRig(t)
	r.validBrowser()
	big := strings.Repeat("a harmless line of text\n", 100000) // ~2.3MB, beyond one scan chunk
	secret := "password=hunter2hunter2hunter2\n"
	r.write("captions.srt", "1\n00:00:01,000 --> 00:00:04,000\nBước một\n\n"+big+secret)
	if _, err := r.build("browser"); err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("secret after the first megabyte was not found: %v", err)
	}
	// a secret straddling a chunk boundary is still found
	r2 := newRig(t)
	r2.validBrowser()
	pad := strings.Repeat("x", scanChunk-10)
	r2.write("captions.srt", "1\n00:00:01,000 --> 00:00:04,000\nBước một\n"+pad+"ghp_abcdefghijklmnopqrstuvwxyz0123456789ab\n")
	if _, err := r2.build("browser"); err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("boundary-straddling secret missed: %v", err)
	}
}

func TestStagedCopyIsWhatGetsHashed(t *testing.T) {
	r := newRig(t)
	r.validBrowser()
	m, err := r.build("browser")
	if err != nil {
		t.Fatal(err)
	}
	// the source changes after staging: the manifest still describes the copy
	r.write("run.mp4", "REPLACED AFTER STAGING")
	for _, f := range m.Files {
		b, err := os.ReadFile(f.Staged)
		if err != nil {
			t.Fatal(err)
		}
		s := sha256.Sum256(b)
		if hex.EncodeToString(s[:]) != f.SHA256 {
			t.Fatalf("%s: staged bytes differ from manifest", f.Name)
		}
	}
}

func publisherRig(t *testing.T) (*rig, *Manifest, *gh.Fake, Publisher) {
	r := newRig(t)
	r.validBrowser()
	m, err := r.build("browser")
	if err != nil {
		t.Fatal(err)
	}
	f := gh.NewFake(time.Now)
	return r, m, f, Publisher{GH: f, Prefix: "evidence"}
}

func TestPublishIsIdempotentContentAddressedAndNeverOverwrites(t *testing.T) {
	r, m, f, p := publisherRig(t)
	work := filepath.Join(r.root, "up")
	links, err := p.Publish(context.Background(), m, work)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1+len(m.Files)+1 {
		t.Fatalf("links %v", links)
	}
	n := len(f.WritesOf("upload"))
	if _, err := p.Publish(context.Background(), m, work); err != nil {
		t.Fatal(err)
	}
	if len(f.WritesOf("upload")) != n {
		t.Fatal("republish uploaded again")
	}
	// an existing asset with the right name but other content is never touched
	tag := p.Tag(m.Issue, m.RunID)
	f.Assets[tag][1].Size++
	if _, err := p.Publish(context.Background(), m, work); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("size-mismatched asset tolerated: %v", err)
	}
	f.Assets[tag][1].Size--
	f.Assets[tag][1].Digest = "sha256:" + strings.Repeat("0", 64)
	if _, err := p.Publish(context.Background(), m, work); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("digest-mismatched asset tolerated: %v", err)
	}
}

func TestPublishRetriesAfterPartialUploadWithoutDuplicates(t *testing.T) {
	r, m, f, p := publisherRig(t)
	work := filepath.Join(r.root, "up")
	f.FailOn["UploadAsset"] = 2 // first two uploads work? no: fail the next two calls
	if _, err := p.Publish(context.Background(), m, work); err == nil {
		t.Fatal("expected upload failure")
	}
	f.FailOn["UploadAsset"] = 0
	if _, err := p.Publish(context.Background(), m, work); err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, a := range f.Assets[p.Tag(m.Issue, m.RunID)] {
		seen[a.Name]++
	}
	for n, c := range seen {
		if c != 1 {
			t.Fatalf("asset %s present %d times", n, c)
		}
	}
	if len(f.WritesOf("release")) != 1 {
		t.Fatal("release created more than once")
	}
}

func TestPublishDetectsStagedFileTamper(t *testing.T) {
	r, m, _, p := publisherRig(t)
	for i := range m.Files {
		if m.Files[i].Kind == "video" {
			os.WriteFile(m.Files[i].Staged, []byte("tampered after validation"), 0o600)
		}
	}
	if _, err := p.Publish(context.Background(), m, filepath.Join(r.root, "up")); err == nil || !strings.Contains(err.Error(), "changed after validation") {
		t.Fatalf("tamper not detected: %v", err)
	}
}

func TestConfineAndPreflight(t *testing.T) {
	root := t.TempDir()
	if _, err := Preflight(root, "", "cli", ""); err == nil {
		t.Fatal("cli without reason")
	}
	if _, err := Preflight(root, "", "browser", ""); err == nil {
		t.Fatal("browser without dir")
	}
	if _, err := Preflight(root, "", "other", "x"); err == nil {
		t.Fatal("bad mode")
	}
	if _, err := Confine(root, filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing dir")
	}
}

// Vietnamese captions are preferred, not enforced by a heuristic.
func TestCaptionLanguageIsNotAHardGate(t *testing.T) {
	r := newRig(t)
	r.validBrowser()
	r.write("captions.srt", "1\n00:00:01,000 --> 00:00:04,000\nBuoc 1 dang nhap khong dau\n\n2\n00:00:05,000 --> 00:00:09,000\nStep two in English\n")
	if _, err := r.build("browser"); err != nil {
		t.Fatalf("captions without diacritics must not be rejected by a language guess: %v", err)
	}
}
