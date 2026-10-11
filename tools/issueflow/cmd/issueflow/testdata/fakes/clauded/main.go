// Fake interactive coder: banner, a trust dialog, a ready prompt, then it
// reads one pasted line, follows the brief (creates feature.txt) and reports
// through the issueflow control CLI.
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

func main() {
	in := bufio.NewReader(os.Stdin)
	fmt.Println("Welcome to fake claude")
	time.Sleep(200 * time.Millisecond)
	fmt.Println("Do you trust the files in this folder?")
	fmt.Println("  1. No, exit")
	fmt.Println("  2. Yes, I trust this folder")
	ans, _ := in.ReadString('\n')
	if strings.TrimSpace(ans) != "2" {
		fmt.Println("refused:", ans)
		os.Exit(3)
	}
	time.Sleep(200 * time.Millisecond)
	fmt.Println("? for shortcuts")
	line, _ := in.ReadString('\n')
	m := regexp.MustCompile(`Read '([^']+)' completely`).FindStringSubmatch(line)
	if m == nil {
		fmt.Println("could not find the brief path in:", line)
		os.Exit(4)
	}
	brief, err := os.ReadFile(m[1])
	if err != nil {
		fmt.Println("cannot read brief:", err)
		os.Exit(5)
	}
	fmt.Println("working on:", len(brief), "bytes of brief")
	wd, _ := os.Getwd()
	os.WriteFile(wd+"/feature.txt", []byte("done\n"), 0o644)
	// a stray background job in the same session, like a dev server would be
	exec.Command("sleep", "300").Start()
	out, err := exec.Command(os.Getenv("FAKE_ISSUEFLOW_BIN"), "--config", os.Getenv("ISSUEFLOW_CONFIG"), "report", "done", "--text", "created feature.txt; test -f feature.txt exit 0").CombinedOutput()
	fmt.Println("report:", string(out), err)
	time.Sleep(10 * time.Minute)
}
