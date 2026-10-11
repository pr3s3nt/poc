// Fake `codex exec`: reads the turn prompt on stdin and acts through the
// issueflow control CLI exactly like a coordinator would, based only on the
// event kinds listed in the prompt. No model is involved.
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func sh(dir string, name string, args ...string) (string, error) {
	c := exec.Command(name, args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func main() {
	args := os.Args[1:]
	fmt.Println(`{"type":"thread.started","thread_id":"019aaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}`)
	b, _ := bufio.NewReader(os.Stdin).ReadString(0)
	prompt := string(b)
	bin := os.Getenv("FAKE_ISSUEFLOW_BIN")
	cfg := os.Getenv("ISSUEFLOW_CONFIG")
	wt, _ := os.Getwd()
	cli := func(a ...string) error {
		out, err := sh(wt, bin, append([]string{"--config", cfg}, a...)...)
		fmt.Printf(`{"type":"cli","args":%q,"out":%q}`+"\n", strings.Join(a, " "), out)
		return err
	}
	tmp := filepath.Join(os.Getenv("FAKE_DIR"), "coord-notes")
	os.MkdirAll(tmp, 0o755)
	kinds := regexp.MustCompile(`(?m)^### #\d+ (\S+) at`).FindAllStringSubmatch(prompt, -1)
	has := map[string]bool{}
	for _, k := range kinds {
		has[k[1]] = true
	}
	// quoted control command lines must contain the quoted binary and config
	if !strings.Contains(prompt, "agent start --brief-file") {
		fmt.Println(`{"type":"error","msg":"prompt lacks control commands"}`)
		os.Exit(3)
	}
	switch {
	case has["checks-passed"]:
		if _, err := sh(wt, "git", "add", "-A"); err != nil {
			os.Exit(4)
		}
		sh(wt, "git", "-c", "user.name=codex", "-c", "user.email=c@c", "commit", "-m", "task work")
		br, _ := sh(wt, "git", "rev-parse", "--abbrev-ref", "HEAD")
		if out, err := sh(wt, "git", "push", "origin", br); err != nil {
			fmt.Println(out)
			os.Exit(5)
		}
		sha, _ := sh(wt, "git", "rev-parse", "HEAD")
		cli("report", "published", "--sha", sha, "--pr", "https://fake.invalid/pr/1")
	case has["coder-done"]:
		cli("report", "evidence", "--mode", "cli", "--reason", "CLI-only fake task: no browser flow")
		cli("report", "accept", "--note", "reviewed", "--check", "test -f feature.txt")
	case has["task-start"]:
		brief := filepath.Join(tmp, "brief.md")
		os.WriteFile(brief, []byte("Create feature.txt containing the word done. Run no other commands."), 0o644)
		cli("agent", "start", "--brief-file", brief)
	}
	_ = args
	os.WriteFile(os.Getenv("FAKE_DIR")+"/codex-ran-"+os.Getenv("ISSUEFLOW_GEN"), []byte(strings.Join(os.Args[1:], " ")), 0o644)
}
