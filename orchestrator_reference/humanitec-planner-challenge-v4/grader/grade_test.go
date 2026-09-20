package grader

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

var (
	planner = flag.String("planner", "", "absolute path to planner executable")
	cases   = flag.String("cases", "../testcases", "path to testcase directory")
	oneCase = flag.String("case", "", "optional testcase directory name")
)

func decodeJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil { return nil, err }
	if dec.More() { return nil, fmt.Errorf("more than one JSON value") }
	return v, nil
}

func TestPlanner(t *testing.T) {
	if *planner == "" { t.Fatal("pass -planner /absolute/path/to/planner") }
	entries, err := os.ReadDir(*cases)
	if err != nil { t.Fatal(err) }
	var names []string
	for _, e := range entries {
		if e.IsDir() && (*oneCase == "" || e.Name() == *oneCase) { names = append(names, e.Name()) }
	}
	sort.Strings(names)
	if len(names) == 0 { t.Fatal("no testcase selected") }

	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := filepath.Join(*cases, name)
			expectedBytes, err := os.ReadFile(filepath.Join(root, "expected", "result.json"))
			if err != nil { t.Fatal(err) }
			expected, err := decodeJSON(expectedBytes)
			if err != nil { t.Fatalf("invalid expected JSON: %v", err) }

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, *planner, "--case", filepath.Join(root, "case.yaml"))
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err = cmd.Run()
			if ctx.Err() == context.DeadlineExceeded { t.Fatal("planner timed out after 10s") }
			if err != nil { t.Fatalf("planner exited with error: %v\nstderr:\n%s", err, stderr.String()) }
			actual, err := decodeJSON(stdout.Bytes())
			if err != nil { t.Fatalf("invalid planner JSON: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String()) }
			if !reflect.DeepEqual(expected, actual) {
				e, _ := json.MarshalIndent(expected, "", "  ")
				a, _ := json.MarshalIndent(actual, "", "  ")
				t.Fatalf("result mismatch\nexpected:\n%s\nactual:\n%s\nstderr:\n%s", e, a, stderr.String())
			}
		})
	}
}

