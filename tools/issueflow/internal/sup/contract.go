package sup

import (
	"encoding/json"
	"fmt"
	"strings"

	"issueflow/internal/state"
)

// EvidenceContract is the machine-readable contract for browser evidence. It
// is what the validator enforces; briefs and `issueflow evidence-schema` print
// the same facts so an agent never has to guess. The tool does NOT ship a
// browser scenario or a producer: an existing Playwright runner has to be
// adapted per task, inside that task's scope.
func EvidenceContract(t *state.Task, area, cliPrefix string) map[string]any {
	run := "<run-id>"
	if t != nil && t.RunID != "" {
		run = t.RunID
	}
	return map[string]any{
		"applies_to":      "tasks whose Issue requires a browser scenario (label agent:browser) or whose coordinator declared `report evidence --mode browser`",
		"not_provided":    "issueflow ships NO browser scenario and NO producer of these files. Adapt an existing Playwright runner (for example frontend/test/e2e/*.mjs with backend/test/integration/video-lib.sh) inside the task's allowed paths. A fake recording or a fabricated file is a defect.",
		"directory":       area,
		"directory_rules": "regular files only, no subdirectories, no symlinks; allowed types: .mp4 .webm .srt .vtt .json .log .txt; everything is scanned for secrets",
		"binding": map[string]string{
			"run_id":       run,
			"content_hash": "the content hash the coordinator accepted; get the current value with `" + cliPrefix + " evidence-hash`; ANY change to the tree afterwards (including untracked files) invalidates the evidence and the scenario must be recorded again",
			"gen":          "an integer >= 1 and not above the highest coder/coordinator generation of this run",
		},
		"files": []map[string]any{
			{"name": "<any>.mp4 or <any>.webm", "required": true, "count": "exactly one", "rules": "headed browser full-window recording, at least 640x360, must decode completely (ffprobe + ffmpeg are always run)"},
			{"name": "<any>.srt or <any>.vtt", "required": true, "rules": "captions: at least one cue, every cue non-empty, cue end <= video duration + 1s; Vietnamese is preferred (step, purpose, action, result) but language is reviewed by Codex, not enforced"},
			{"name": "assertions.json", "required": true, "producer": "the scenario runner",
				"schema": map[string]any{"run_id": "string = run id", "content_hash": "string", "gen": "int", "adapter": "real|fake (must equal the adapter declared with `report evidence`)", "headed": "true", "overall": "\"pass\"",
					"assertions": []map[string]string{{"name": "non-empty string", "status": "\"pass\"", "detail": "optional"}}},
				"rules": "at least one assertion; every status must be pass; a fake adapter proves no live deploy and is labelled as such in the Issue comment"},
			{"name": "phases.json", "required": true, "producer": "the scenario runner",
				"schema": map[string]any{"run_id": "string", "content_hash": "string", "duration_seconds": "number > 0, within 2s of the video duration",
					"phases": []map[string]any{{"name": "non-empty", "start": "seconds >= 0", "end": "seconds > start"}}},
				"rules": "phases are ordered, do not overlap and end within the duration"},
			{"name": "frame-review.json", "required": true, "producer": "the Codex coordinator, after looking at the frames itself",
				"schema": map[string]any{"reviewer": "\"codex\"", "run_id": "string", "content_hash": "string", "video_sha256": "sha256 of the video file (sha256sum)", "privacy_ok": "true only if no secret or private data is visible",
					"frames": []map[string]any{{"t": "seconds within the video", "note": "what the frame shows", "ok": "true"}}},
				"rules": "at least one reviewed frame per phase; extract frames with ffmpeg and view them; a manifest never proves privacy"},
		},
		"cli_only_tasks": "no video and no browser files: `report evidence --mode cli --reason TEXT`; the supervisor attaches the real check logs. Not allowed when the Issue requires a browser scenario.",
		"sample_assertions": map[string]any{"run_id": run, "content_hash": "<evidence-hash>", "gen": 1, "adapter": "real", "headed": true, "overall": "pass",
			"assertions": []map[string]string{{"name": "console list is visible after sign-in", "status": "pass"}}},
		"sample_phases": map[string]any{"run_id": run, "content_hash": "<evidence-hash>", "duration_seconds": 42.5,
			"phases": []map[string]any{{"name": "sign-in", "start": 0.5, "end": 9.0}, {"name": "save", "start": 9.0, "end": 41.0}}},
	}
}

func EvidenceContractJSON(t *state.Task, area, cliPrefix string) []byte {
	b, _ := json.MarshalIndent(EvidenceContract(t, area, cliPrefix), "", "  ")
	return b
}

// evidenceBriefSection is the human-readable contract placed in agent briefs.
func (s *Sup) evidenceBriefSection(t *state.Task, forCoder bool) string {
	cli := s.cli(t)
	var b strings.Builder
	fmt.Fprintf(&b, "\n## Browser evidence contract (this task needs a browser scenario)\n")
	fmt.Fprintf(&b, "Write evidence ONLY into this directory (no subdirectories, no symlinks): %s\n", shq(s.evidenceRoot(t)))
	fmt.Fprintf(&b, "Run id: %s. Full machine-readable schema and samples: `%s evidence-schema`. Current content hash: `%s evidence-hash`.\n", t.RunID, cli, cli)
	b.WriteString(`Required files, all bound to this run_id and to the content_hash of the reviewed tree:
- one headed full-window .mp4/.webm recording (>= 640x360, decodes fully);
- captions .srt/.vtt (Vietnamese preferred; steps, purpose, action, result; no secrets, must not hide the UI);
- assertions.json {run_id, content_hash, gen, adapter: real|fake, headed: true, overall: "pass", assertions: [{name, status: "pass"}]};
- phases.json {run_id, content_hash, duration_seconds, phases: [{name, start, end}]} (ordered, non-overlapping, matches the video length);
- frame-review.json {reviewer: "codex", run_id, content_hash, video_sha256, privacy_ok: true, frames: [{t, note, ok: true}]} written by the Codex coordinator after it looked at the frames itself.
issueflow ships NO browser scenario and NO producer: adapt an existing Playwright runner (frontend/test/e2e/, backend/test/integration/video-lib.sh) inside the paths this task allows. Do not fabricate a recording or a file; a failing scenario stays a failure. Any change to the tree after recording (even an untracked file) means the scenario must be recorded again.
`)
	if forCoder {
		fmt.Fprintf(&b, "As the coder: produce the recording, captions, assertions.json and phases.json. Compute content_hash right before the final recording with `%s evidence-hash`. Do NOT write frame-review.json.\n", cli)
	} else {
		fmt.Fprintf(&b, "As the coordinator: after the coder's recording, extract frames with ffmpeg, look at them, compute `sha256sum` of the video, write frame-review.json, declare `%s report evidence --mode browser --adapter real|fake`, then accept. Accept binds the content hash; evidence made for other content fails at publish.\n", cli)
	}
	return b.String()
}

func (s *Sup) needsBrowserEvidence(t *state.Task) bool {
	return t.Evidence.Required == "browser" || t.Evidence.Mode == "browser"
}
