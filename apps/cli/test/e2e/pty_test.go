package e2e

// A minimal `expect` in Go: the host has no expect/tcl (and workers install no packages), so the e2e drives the
// REPL through a real pseudo-terminal opened with golang.org/x/sys/unix — raw mode, Tab and `?` are exercised
// exactly as a user at a terminal would.

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

type session struct {
	t      *testing.T
	master *os.File
	cmd    *exec.Cmd
	mu     sync.Mutex
	out    []byte
	mark   int // expect() searches from here
	sent   int // len(out) when input was last sent: prompt() looks for a prompt printed after it
	done   chan struct{}
}

func openPTY() (*os.File, *os.File, error) {
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}
	if err := unix.IoctlSetPointerInt(int(m.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		_ = m.Close()
		return nil, nil, err
	}
	n, err := unix.IoctlGetInt(int(m.Fd()), unix.TIOCGPTN)
	if err != nil {
		_ = m.Close()
		return nil, nil, err
	}
	s, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		_ = m.Close()
		return nil, nil, err
	}
	_ = unix.IoctlSetWinsize(int(s.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: 50, Col: 200})
	return m, s, nil
}

// spawn starts bin with args on a new pty.
func spawn(t *testing.T, env []string, bin string, args ...string) *session {
	t.Helper()
	m, s, err := openPTY()
	if err != nil {
		t.Fatalf("pty: %v", err)
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = s, s, s
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", bin, err)
	}
	_ = s.Close()
	se := &session{t: t, master: m, cmd: cmd, done: make(chan struct{})}
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := m.Read(buf)
			if n > 0 {
				se.mu.Lock()
				se.out = append(se.out, buf[:n]...)
				se.mu.Unlock()
			}
			if err != nil {
				close(se.done)
				return
			}
		}
	}()
	t.Cleanup(func() {
		if cmd.ProcessState == nil && cmd.Process != nil {
			_ = cmd.Process.Kill() // only the process this test started
			_, _ = cmd.Process.Wait()
		}
		_ = m.Close()
	})
	return se
}

func (s *session) send(text string) {
	s.t.Helper()
	s.mu.Lock()
	s.sent = len(s.out)
	s.mu.Unlock()
	if _, err := s.master.WriteString(text); err != nil {
		s.t.Fatalf("write to pty: %v", err)
	}
}

// line sends text + Enter.
func (s *session) line(text string) { s.send(text + "\r") }

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// expect waits until re matches the (ANSI-stripped) output after the last match.
func (s *session) expect(re string, timeout time.Duration) []string {
	s.t.Helper()
	rx := regexp.MustCompile(re)
	deadline := time.Now().Add(timeout)
	for {
		s.mu.Lock()
		tail := ansi.ReplaceAllString(string(s.out[s.mark:]), "")
		if loc := rx.FindStringSubmatchIndex(tail); loc != nil {
			m := rx.FindStringSubmatch(tail)
			// advance the mark past the match (in raw bytes: find the same text again in the raw tail)
			s.mark += rawOffset(string(s.out[s.mark:]), loc[1])
			s.mu.Unlock()
			return m
		}
		s.mu.Unlock()
		if time.Now().After(deadline) {
			s.t.Fatalf("timeout after %s waiting for /%s/; output since last match:\n%s", timeout, re, screen(tail))
		}
		select {
		case <-s.done:
			s.mu.Lock()
			tail = ansi.ReplaceAllString(string(s.out[s.mark:]), "")
			s.mu.Unlock()
			if !rx.MatchString(tail) {
				s.t.Fatalf("process exited while waiting for /%s/; output:\n%s", re, screen(tail))
			}
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// rawOffset maps an offset in the ANSI-stripped text back to the raw text.
func rawOffset(raw string, strippedOff int) int {
	seen := 0
	i := 0
	for i < len(raw) && seen < strippedOff {
		if loc := ansi.FindStringIndex(raw[i:]); loc != nil && loc[0] == 0 {
			i += loc[1]
			continue
		}
		i++
		seen++
	}
	return i
}

var promptAtEnd = regexp.MustCompile(`@\S+[#>] $`)

// prompt waits until the terminal output ends with a REPL prompt (the shell is idle), so the next command is not
// typed ahead into the previous one.
func (s *session) prompt() {
	s.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		s.mu.Lock()
		all := ansi.ReplaceAllString(string(s.out[s.sent:]), "")
		if promptAtEnd.MatchString(all) {
			s.mark = len(s.out)
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		if time.Now().After(deadline) {
			s.t.Fatalf("no prompt within 30 s; output:\n%s", screen(all))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// command waits for the prompt, then types text + Enter.
func (s *session) command(text string) {
	s.t.Helper()
	s.prompt()
	s.line(text)
}

// wait for the process to exit and return its exit code.
func (s *session) wait(timeout time.Duration) int {
	s.t.Helper()
	ch := make(chan error, 1)
	go func() { ch <- s.cmd.Wait() }()
	select {
	case <-ch:
		return s.cmd.ProcessState.ExitCode()
	case <-time.After(timeout):
		s.t.Fatalf("process did not exit within %s", timeout)
		return -1
	}
}

// transcript renders everything the terminal showed as plain lines (carriage returns, erase-line and cursor-left
// applied), for the evidence file.
func (s *session) transcript() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return screen(string(s.out))
}

var csi = regexp.MustCompile(`^\x1b\[([0-9;]*)([A-Za-z])`)

func screen(raw string) string {
	var lines []string
	cur := []rune{}
	col := 0
	put := func(r rune) {
		if col < len(cur) {
			cur[col] = r
		} else {
			cur = append(cur, r)
		}
		col++
	}
	for i := 0; i < len(raw); {
		if m := csi.FindStringSubmatch(raw[i:]); m != nil {
			switch m[2] {
			case "K":
				cur = cur[:col]
			case "D":
				n := 1
				_, _ = fmt.Sscanf(m[1], "%d", &n)
				col -= n
				if col < 0 {
					col = 0
				}
			case "J":
				lines = nil
			}
			i += len(m[0])
			continue
		}
		r, size := utf8.DecodeRuneInString(raw[i:])
		i += size
		switch r {
		case '\r':
			col = 0
		case '\n':
			lines = append(lines, strings.TrimRight(string(cur), " "))
			cur, col = []rune{}, 0
		case '\a':
		default:
			put(r)
		}
	}
	if len(cur) > 0 {
		lines = append(lines, string(cur))
	}
	return strings.Join(lines, "\n")
}
