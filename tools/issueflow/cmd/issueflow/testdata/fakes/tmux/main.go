// Fake `tmux` for the CLI smoke test. Sessions are real detached processes
// (their own session, so pane_pid is a session leader like in tmux); input
// goes through a FIFO.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

var root = filepath.Join(os.Getenv("FAKE_DIR"), "tmux")

func sdir(name string) string { return filepath.Join(root, strings.TrimPrefix(name, "=")) }

func alive(name string) (int, bool) {
	b, err := os.ReadFile(filepath.Join(sdir(name), "pid"))
	if err != nil {
		return 0, false
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if pid > 1 && syscall.Kill(pid, 0) == nil {
		// a zombie child of this very process tree still answers kill 0
		if st, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
			s := string(st)
			if i := strings.LastIndexByte(s, ')'); i > 0 && len(s) > i+2 && s[i+2] == 'Z' {
				return pid, false
			}
		}
		return pid, true
	}
	return pid, false
}

func fail(msg string) { fmt.Fprintln(os.Stderr, msg); os.Exit(1) }

func writeFifo(name, s string) {
	f, err := os.OpenFile(filepath.Join(sdir(name), "fifo"), os.O_WRONLY, 0)
	if err != nil {
		fail("can't find session: " + name)
	}
	defer f.Close()
	f.WriteString(s)
}

func main() {
	a := os.Args[1:]
	if len(a) >= 2 && a[0] == "-L" {
		a = a[2:]
	}
	if len(a) == 0 {
		return
	}
	cmd, rest := a[0], a[1:]
	flag := func(f string) string {
		for i, x := range rest {
			if x == f && i+1 < len(rest) {
				return rest[i+1]
			}
		}
		return ""
	}
	switch cmd {
	case "-V":
		fmt.Println("tmux 3.x (fake)")
	case "has-session":
		if _, ok := alive(flag("-t")); ok {
			return
		}
		fail("can't find session: " + flag("-t"))
	case "new-session":
		name := flag("-s")
		if _, ok := alive(name); ok {
			fail("duplicate session: " + name)
		}
		d := sdir(name)
		os.RemoveAll(d)
		os.MkdirAll(d, 0o755)
		var env []string
		for i, x := range rest {
			if x == "-e" && i+1 < len(rest) {
				env = append(env, rest[i+1])
			}
		}
		shell := rest[len(rest)-1]
		var c *exec.Cmd
		out, _ := os.Create(filepath.Join(d, "out"))
		if shell == "bash -i" {
			syscall.Mkfifo(filepath.Join(d, "fifo"), 0o600)
			c = exec.Command("bash", "-c", `exec 3<>"$1"; exec bash -i <&3`, "x", filepath.Join(d, "fifo"))
		} else {
			c = exec.Command("bash", "-c", shell)
		}
		c.Dir = flag("-c")
		c.Env = append(os.Environ(), env...)
		c.Stdout, c.Stderr = out, out
		c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := c.Start(); err != nil {
			fail(err.Error())
		}
		os.WriteFile(filepath.Join(d, "pid"), []byte(fmt.Sprint(c.Process.Pid)), 0o644)
		go c.Wait()
		// give the child a moment to become reachable
		c.Process.Release()
	case "pipe-pane":
		name := flag("-t")
		logp := strings.Trim(strings.TrimPrefix(rest[len(rest)-1], "cat >> "), "'")
		t := exec.Command("bash", "-c", fmt.Sprintf("tail -n +1 -F %q >> %q 2>/dev/null", filepath.Join(sdir(name), "out"), logp))
		t.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		t.Start()
		os.WriteFile(filepath.Join(sdir(name), "pipe.pid"), []byte(fmt.Sprint(t.Process.Pid)), 0o644)
		t.Process.Release()
	case "send-keys":
		name := flag("-t")
		if _, ok := alive(name); !ok {
			fail("can't find session: " + name)
		}
		lit := false
		var keys []string
		skip := false
		for i, x := range rest {
			if skip {
				skip = false
				continue
			}
			if x == "-t" {
				skip = true
				continue
			}
			if x == "-l" {
				lit = true
				continue
			}
			_ = i
			keys = append(keys, x)
		}
		for _, k := range keys {
			switch {
			case lit:
				writeFifo(name, k)
			case k == "Enter":
				writeFifo(name, "\n")
			default:
				writeFifo(name, k)
			}
		}
	case "load-buffer":
		b, err := os.ReadFile(rest[len(rest)-1])
		if err != nil {
			fail(err.Error())
		}
		os.MkdirAll(filepath.Join(root, "buffers"), 0o755)
		os.WriteFile(filepath.Join(root, "buffers", flag("-b")), b, 0o644)
	case "paste-buffer":
		b, err := os.ReadFile(filepath.Join(root, "buffers", flag("-b")))
		if err != nil {
			fail("no buffer")
		}
		writeFifo(flag("-t"), string(b))
		os.Remove(filepath.Join(root, "buffers", flag("-b")))
	case "display-message":
		name := flag("-t")
		pid, ok := alive(name)
		if !ok {
			fail("can't find session: " + name)
		}
		fmt.Println(pid)
	case "kill-session":
		name := flag("-t")
		pid, ok := alive(name)
		if !ok {
			if _, err := os.Stat(sdir(name)); err != nil {
				fail("can't find session: " + name)
			}
		}
		if pid > 1 {
			syscall.Kill(-pid, syscall.SIGKILL)
		}
		if b, err := os.ReadFile(filepath.Join(sdir(name), "pipe.pid")); err == nil {
			if p, _ := strconv.Atoi(string(b)); p > 1 {
				syscall.Kill(-p, syscall.SIGKILL)
			}
		}
		// wait briefly for the group to disappear so has-session is definite
		for i := 0; i < 50; i++ {
			if _, still := alive(name); !still {
				break
			}
			syscall.Kill(-pid, syscall.SIGKILL)
			exec.Command("sleep", "0.05").Run()
		}
		os.Remove(filepath.Join(sdir(name), "pid"))
	default:
		fail("fake tmux: unsupported " + cmd)
	}
}
