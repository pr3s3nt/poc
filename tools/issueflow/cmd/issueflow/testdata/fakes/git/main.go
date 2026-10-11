// Fake `git` shim: reports an https origin with embedded credentials (so the
// repo-identity preflight runs for real) and delegates everything else to git.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func main() {
	a := os.Args[1:]
	if strings.Join(a, " ") == "remote get-url origin" {
		fmt.Println("https://user:not-a-real-token@github.com/o/r.git")
		return
	}
	real := os.Getenv("FAKE_REAL_GIT")
	if _, err := exec.LookPath(real); err != nil {
		fmt.Fprintln(os.Stderr, "fake git: FAKE_REAL_GIT missing")
		os.Exit(2)
	}
	syscall.Exec(real, append([]string{"git"}, a...), os.Environ())
}
