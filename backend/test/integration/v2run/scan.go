package v2run

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`ASIA[0-9A-Z]{16}`),
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)client-key-data\s*:`),
	regexp.MustCompile(`(?i)(?:secret_?access_?key|aws_secret)["'\s:=]+[A-Za-z0-9/+=]{30,}`),
	regexp.MustCompile(`hvs\.[A-Za-z0-9_-]{20,}`),
	regexp.MustCompile(`postgres(?:ql)?://[^\s:@/]+:[^\s@/]{6,}@`),
}

var sensitiveKey = regexp.MustCompile(`(?i)(token|password|secret|key|data|credential)`)

// SecretValues extracts the secret values of private handle files: whole
// trimmed contents, URL passwords and X-Vault-Token headers. Values shorter
// than 8 characters are ignored (they would match anything).
func SecretValues(files []string) ([]string, error) {
	set := map[string]bool{}
	for _, f := range files {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("secret handle %s unreadable", filepath.Base(f))
		}
		text := strings.TrimSpace(string(raw))
		if u, err := url.Parse(text); err == nil && u.User != nil {
			if pw, ok := u.User.Password(); ok {
				set[pw] = true
			}
		}
		if m := regexp.MustCompile(`X-Vault-Token:\s*(\S+)`).FindStringSubmatch(text); m != nil {
			set[m[1]] = true
		}
		var doc any
		if json.Unmarshal([]byte(text), &doc) == nil {
			// JSON handles (for example {"accessKeyId":..., "secretAccessKey":...}): every string under a sensitive key.
			collectJSONSecrets(doc, "", set)
		}
		if !strings.ContainsAny(text, "\n ") {
			set[text] = true
		}
		for _, line := range strings.Split(text, "\n") {
			if i := strings.IndexAny(line, "=:"); i > 0 && len(line) > i+9 && sensitiveKey.MatchString(line[:i]) {
				set[strings.Trim(strings.TrimSpace(line[i+1:]), `"'`)] = true
			}
		}
	}
	var out []string
	for v := range set {
		if len(v) >= 8 {
			out = append(out, v)
		}
	}
	return out, nil
}

func collectJSONSecrets(v any, key string, set map[string]bool) {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			collectJSONSecrets(child, k, set)
		}
	case []any:
		for _, child := range x {
			collectJSONSecrets(child, key, set)
		}
	case string:
		if key != "" && sensitiveKey.MatchString(key) {
			set[x] = true
		}
	}
}

// ScanPublic fails when any public text file (logs, JSON, captions, manifests)
// contains a known secret value or a credential-looking pattern. It returns
// the number of files scanned and never echoes a value. The private directory
// must not be inside the scanned tree.
func ScanPublic(dir string, secretFiles []string) (int, error) {
	values, err := SecretValues(secretFiles)
	if err != nil {
		return 0, err
	}
	if len(values) == 0 {
		return 0, errors.New("the secret scan has no secret values to look for; it would prove nothing")
	}
	textual := regexp.MustCompile(`\.(log|txt|json|srt|ass|tsv|md|env|yaml|yml|tf|tfvars|html|mjs|js)$`)
	scanned := 0
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			if d.Name() == "private" {
				return filepath.SkipDir
			}
			return nil
		}
		if !textual.MatchString(d.Name()) {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		text := string(raw)
		rel, _ := filepath.Rel(dir, path)
		for _, v := range values {
			if strings.Contains(text, v) {
				return fmt.Errorf("secret material found in %s", rel)
			}
		}
		for _, p := range secretPatterns {
			if p.MatchString(text) {
				return fmt.Errorf("credential pattern found in %s", rel)
			}
		}
		return nil
	})
	return scanned, err
}

// Run marker states. The marker is written before the first create of a run
// and is the only thing that proves a run directory belongs to this runner when
// the private spec or status files are missing or were lost.
const (
	MarkerStarted         = "STARTED"
	MarkerArmed           = "ARMED"
	MarkerClosedClean     = "CLOSED_CLEAN"
	MarkerClosedNoEffects = "CLOSED_NO_EFFECTS"
)

// RunMarkerName is the marker file inside a run evidence directory.
const RunMarkerName = "run-marker.json"

// RunMarker is the immutable-start record of a run. Only the state advances.
type RunMarker struct {
	RunID   string `json:"runId"`
	Target  string `json:"target"`
	State   string `json:"state"`
	Started string `json:"started"`
	Reason  string `json:"reason,omitempty"`
}

// WriteRunMarker creates (STARTED) or advances the marker of dir. Advancing to
// a CLOSED state without a prior marker is refused, and a closed marker is
// never reopened.
func WriteRunMarker(dir string, m RunMarker) error {
	path := filepath.Join(dir, RunMarkerName)
	switch m.State {
	case MarkerStarted, MarkerArmed, MarkerClosedClean, MarkerClosedNoEffects:
	default:
		return fmt.Errorf("unknown run marker state %q", m.State)
	}
	prev, err := ReadRunMarker(dir)
	switch {
	case err == nil:
		if strings.HasPrefix(prev.State, "CLOSED") {
			return errors.New("run marker is closed and cannot be changed")
		}
		if prev.RunID != m.RunID {
			return errors.New("run marker belongs to another run")
		}
		m.Started = prev.Started
	case os.IsNotExist(err):
		if m.State != MarkerStarted {
			return errors.New("run marker must start in state STARTED")
		}
	default:
		return err
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return WriteFilePrivate(path, raw)
}

// ReadRunMarker reads the marker of a run directory.
func ReadRunMarker(dir string) (RunMarker, error) {
	var m RunMarker
	raw, err := os.ReadFile(filepath.Join(dir, RunMarkerName))
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, fmt.Errorf("run marker unreadable: %w", err)
	}
	return m, nil
}

// DirtyRuns lists run directories under root that are not verified clean. A
// known run is clean only after an explicit CLOSED_CLEAN marker (the runner
// verified cleanup and local teardown) or CLOSED_NO_EFFECTS marker (explicit
// operator action). Supervisor CLEAN alone is insufficient because it does not
// prove the runner completed physical, sentinel, and local cleanup checks. A
// legacy supervisor spec without a marker stays dirty until operator closure;
// unrelated directories with neither file are excluded.
func DirtyRuns(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var dirty []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		_, markerErr := os.Lstat(filepath.Join(dir, RunMarkerName))
		_, specErr := os.Lstat(filepath.Join(dir, "private", "supervisor-spec.json"))
		if os.IsNotExist(markerErr) && os.IsNotExist(specErr) {
			continue // not a run directory of this runner
		}
		if markerErr == nil {
			m, err := ReadRunMarker(dir)
			if err == nil && (m.State == MarkerClosedNoEffects || m.State == MarkerClosedClean) {
				continue
			}
		}
		// A present, malformed, unreadable, or legacy run marker/spec without
		// explicit closure remains dirty. Supervisor status cannot override it.
		dirty = append(dirty, e.Name())
	}
	return dirty, nil
}
