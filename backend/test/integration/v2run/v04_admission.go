package v2run

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed testdata/v04-required-assertions.json
var v04RequiredAssertionsJSON []byte

// V04RequiredAssertions is shared with the browser runner. Returning a fresh
// slice prevents callers from changing the predecessor acceptance contract.
func V04RequiredAssertions() []string {
	var ids []string
	if err := json.Unmarshal(v04RequiredAssertionsJSON, &ids); err != nil || len(ids) == 0 {
		panic("invalid packaged V04 assertion contract")
	}
	return ids
}

var v04EvidenceFiles = []string{
	"run.json", "source-files.sha256", "assertions.json", "phases.json", "marks.json", "assertions.txt", "result.txt",
	"V04-raw.mp4", "V04-captioned.mp4", "captions.srt", "captions.ass", "ffprobe.json", "decode.log", "decode-captioned.log",
	"checks.txt", "cleanup.txt", "cleanup-state.txt",
}

type v04AdmissionReceipt struct {
	AcceptanceScope  string            `json:"acceptanceScope"`
	Version          int               `json:"version"`
	CompletedAt      time.Time         `json:"completedAt"`
	Scenario         string            `json:"scenario"`
	RunID            string            `json:"runId"`
	Target           string            `json:"target"`
	Source           v04SourceIdentity `json:"source"`
	EvidenceSHA256   map[string]string `json:"evidenceSha256"`
	CleanupState     string            `json:"cleanupState"`
	PrivateRemoved   bool              `json:"privateMaterialRemoved"`
	ReopenPassed     bool              `json:"sqlReopenPassed"`
	VideoPassed      bool              `json:"finalVideoPassed"`
	SecretScanPassed bool              `json:"finalSecretScanPassed"`
	Status           string            `json:"status"`
	ExitCode         *int              `json:"exitCode"`
}

type v04SourceIdentity struct {
	GitHead           string `json:"gitHead"`
	GitDiffSHA256     string `json:"gitDiffSha256"`
	SourceFilesSHA256 string `json:"sourceFilesSha256"`
}

type v04RunIdentity struct {
	Scenario string            `json:"scenario"`
	RunID    string            `json:"runId"`
	Target   string            `json:"target"`
	Fake     string            `json:"fake"`
	Source   v04SourceIdentity `json:"source"`
}

type v04Assertion struct {
	ID     string `json:"id"`
	Pass   bool   `json:"pass"`
	Source string `json:"source"`
}

func verifyIntegratedV04(dir, expectedRunID, expectedGitHead, expectedDiff, expectedSourceFiles string) error {
	validHex := func(value string, size int) bool {
		b, err := hex.DecodeString(value)
		return err == nil && len(b) == size
	}
	if !filepath.IsAbs(dir) || expectedRunID == "" || !validHex(expectedGitHead, 20) || !validHex(expectedDiff, 32) || !validHex(expectedSourceFiles, 32) {
		return errors.New("integrated V04 admission needs an absolute evidence path and expected run/source identity")
	}
	var run v04RunIdentity
	raw, err := os.ReadFile(filepath.Join(dir, "run.json"))
	if err != nil || json.Unmarshal(raw, &run) != nil || run.Scenario != "V04" || run.Target != TargetK8s4f || run.Fake != "none" || run.RunID != expectedRunID || run.Source.GitHead != expectedGitHead || run.Source.SourceFilesSHA256 != expectedSourceFiles || run.Source.GitDiffSHA256 != expectedDiff {
		return errors.New("integrated V04 run/source/target identity does not match the configured predecessor")
	}
	var receipt v04AdmissionReceipt
	raw, err = os.ReadFile(filepath.Join(dir, "v04-admission-receipt.json"))
	if err != nil || json.Unmarshal(raw, &receipt) != nil || receipt.Version != 2 || receipt.AcceptanceScope != "MAIN_FLOW_2026_10_19" || receipt.Scenario != "V04" || receipt.RunID != expectedRunID || receipt.Target != TargetK8s4f || receipt.Source != run.Source {
		return errors.New("integrated V04 final admission receipt is missing or mismatched")
	}
	// Reuse is determined by source/configuration and assertions, not by elapsed
	// wall time. Do not force a live rerun merely because a receipt is older.
	if receipt.CompletedAt.IsZero() || receipt.CompletedAt.After(time.Now().Add(time.Minute)) {
		return errors.New("integrated V04 receipt has an invalid completion time")
	}
	if receipt.Status != "PASS" || receipt.ExitCode == nil || *receipt.ExitCode != 0 || receipt.CleanupState != MarkerClosedClean || !receipt.PrivateRemoved || !receipt.ReopenPassed || !receipt.VideoPassed || !receipt.SecretScanPassed {
		return errors.New("integrated V04 did not prove complete post-cleanup acceptance")
	}
	for _, name := range []string{"private", "CLEANUP-NOT-PROVEN.txt"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			return errors.New("integrated V04 retains private recovery material or unresolved cleanup evidence")
		}
	}
	for _, name := range v04EvidenceFiles {
		expected := receipt.EvidenceSHA256[name]
		if !validHex(expected, 32) {
			return fmt.Errorf("integrated V04 receipt does not bind %s", name)
		}
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("integrated V04 evidence %s is missing", name)
		}
		h := sha256.New()
		_, copyErr := io.Copy(h, f)
		closeErr := f.Close()
		if copyErr != nil || closeErr != nil || hex.EncodeToString(h.Sum(nil)) != expected {
			return fmt.Errorf("integrated V04 evidence %s changed after final receipt or cannot be read", name)
		}
	}
	manifest, err := os.ReadFile(filepath.Join(dir, "source-files.sha256"))
	manifestHash := sha256.Sum256([]byte(strings.TrimSuffix(string(manifest), "\n")))
	if err != nil || hex.EncodeToString(manifestHash[:]) != run.Source.SourceFilesSHA256 {
		return errors.New("integrated V04 source manifest does not match its source identity")
	}
	result, err := os.ReadFile(filepath.Join(dir, "result.txt"))
	if err != nil || strings.TrimSpace(string(result)) != "PASS" {
		return errors.New("integrated V04 browser acceptance did not PASS")
	}
	cleanup, err := os.ReadFile(filepath.Join(dir, "cleanup-state.txt"))
	if err != nil || strings.TrimSpace(string(cleanup)) != "proven" {
		return errors.New("integrated V04 physical cleanup is not proven")
	}
	var assertions []v04Assertion
	assertionRaw, err := os.ReadFile(filepath.Join(dir, "assertions.json"))
	if err != nil || json.Unmarshal(assertionRaw, &assertions) != nil {
		return errors.New("integrated V04 assertions are missing or malformed")
	}
	passed := map[string]bool{}
	for _, a := range assertions {
		if !a.Pass || passed[a.ID] || (a.Source != "ui" && a.Source != "private-read" && a.Source != "api") {
			return errors.New("integrated V04 contains failed, duplicate or simulated assertions")
		}
		passed[a.ID] = true
	}
	for _, id := range V04RequiredAssertions() {
		if !passed[id] {
			return fmt.Errorf("integrated V04 required assertion %s is missing", id)
		}
	}
	return nil
}
