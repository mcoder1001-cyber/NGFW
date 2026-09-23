package renderers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Runner errors.
var (
	// ErrNotAllowed is returned when Command.Path is not in the runner's Allowlist.
	ErrNotAllowed = errors.New("renderers: binary not in allowlist")
	// ErrBadCommand is returned for malformed commands (relative path, NUL bytes, ...).
	ErrBadCommand = errors.New("renderers: bad command")
)

// DefaultTimeout bounds a command when Command.Timeout is zero.
const DefaultTimeout = 30 * time.Second

// DefaultMaxOutput caps captured stdout and stderr (each) when SystemRunner.MaxOutput is
// zero.
const DefaultMaxOutput = 4 << 20

// Allowlist is the set of absolute binary paths a SystemRunner may execute. Each renderer
// package declares its own (`var binaries = renderers.NewAllowlist("/usr/bin/vtysh", ...)`)
// and lists every entry in internal/renderers/ALLOWLIST.md; TestAllowlistDocumented fails
// otherwise.
type Allowlist map[string]struct{}

// NewAllowlist builds an Allowlist. It panics on a path that is not absolute and clean,
// because allowlists are package-level constants and a bad entry is a programming error.
func NewAllowlist(paths ...string) Allowlist {
	a := make(Allowlist, len(paths))
	for _, p := range paths {
		if !filepath.IsAbs(p) || filepath.Clean(p) != p {
			panic(fmt.Sprintf("renderers: allowlist entry %q must be an absolute, clean path", p))
		}
		a[p] = struct{}{}
	}
	return a
}

// Allowed reports whether path may be executed.
func (a Allowlist) Allowed(path string) bool {
	_, ok := a[path]
	return ok
}

// Paths returns the entries sorted.
func (a Allowlist) Paths() []string {
	out := make([]string, 0, len(a))
	for p := range a {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// Command is one process invocation with a fixed argument vector. There is no shell: Path is
// executed directly and every element of Args reaches the program as one argv entry,
// whatever it contains. Rendered content travels through files or Stdin, never through argv.
type Command struct {
	// Path is the absolute path of the binary; it must be in the runner's Allowlist.
	Path string
	// Args are argv[1:], passed verbatim.
	Args []string
	// Stdin is written to the process's standard input (nil: /dev/null).
	Stdin []byte
	// Dir is the working directory ("" inherits).
	Dir string
	// Timeout bounds the run; zero means DefaultTimeout. On expiry the process is killed.
	Timeout time.Duration
}

// String renders the command for logs (argv joined by spaces; never put secrets in Args).
func (c Command) String() string {
	return strings.Join(append([]string{c.Path}, c.Args...), " ")
}

// Output is what a finished process produced.
type Output struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// ExitError is returned by Run when the process exits non-zero; Output holds what it wrote.
type ExitError struct {
	Command Command
	Output  Output
}

func (e *ExitError) Error() string {
	msg := strings.TrimSpace(string(e.Output.Stderr))
	if len(msg) > 512 {
		msg = msg[:512] + "..."
	}
	return fmt.Sprintf("renderers: %s exited %d: %s", e.Command.Path, e.Output.ExitCode, msg)
}

// Runner executes Commands. Renderers take a Runner so unit tests inject a RecordingRunner.
type Runner interface {
	Run(ctx context.Context, cmd Command) (Output, error)
}

// SystemRunner executes allow-listed binaries with a fixed argv, a minimal environment, a
// timeout and bounded output capture.
type SystemRunner struct {
	Allow Allowlist
	// Env replaces the environment; nil means DefaultEnv. The agent's own environment is
	// never inherited.
	Env []string
	// MaxOutput caps captured stdout and stderr in bytes each; zero means DefaultMaxOutput.
	MaxOutput int
}

// DefaultEnv is the environment child processes get unless SystemRunner.Env is set.
var DefaultEnv = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL=C", "LANG=C"}

var _ Runner = (*SystemRunner)(nil)

// NewSystemRunner returns a runner restricted to allow.
func NewSystemRunner(allow Allowlist) *SystemRunner {
	return &SystemRunner{Allow: allow}
}

// Run executes cmd. It returns ErrNotAllowed for unlisted binaries, ErrBadCommand for
// malformed commands, *ExitError for a non-zero exit, and the context error on timeout.
func (r *SystemRunner) Run(ctx context.Context, cmd Command) (Output, error) {
	if err := checkCommand(cmd); err != nil {
		return Output{}, err
	}
	if !r.Allow.Allowed(cmd.Path) {
		return Output{}, fmt.Errorf("%w: %s", ErrNotAllowed, cmd.Path)
	}
	timeout := cmd.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	limit := r.MaxOutput
	if limit <= 0 {
		limit = DefaultMaxOutput
	}
	stdout, stderr := &limitedBuffer{limit: limit}, &limitedBuffer{limit: limit}

	// Fixed argv from an allow-listed absolute path; no shell is involved.
	proc := exec.CommandContext(ctx, cmd.Path, cmd.Args...) //nolint:gosec // allow-listed fixed argv
	proc.Env = r.Env
	if proc.Env == nil {
		proc.Env = DefaultEnv
	}
	proc.Dir = cmd.Dir
	if cmd.Stdin != nil {
		proc.Stdin = bytes.NewReader(cmd.Stdin)
	}
	proc.Stdout, proc.Stderr = stdout, stderr
	proc.WaitDelay = 2 * time.Second

	err := proc.Run()
	out := Output{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if proc.ProcessState != nil {
		out.ExitCode = proc.ProcessState.ExitCode()
	}
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return out, fmt.Errorf("renderers: %s: %w", cmd.Path, ctxErr)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return out, &ExitError{Command: cmd, Output: out}
		}
		return out, fmt.Errorf("renderers: %s: %w", cmd.Path, err)
	}
	return out, nil
}

func checkCommand(cmd Command) error {
	switch {
	case cmd.Path == "":
		return fmt.Errorf("%w: empty path", ErrBadCommand)
	case !filepath.IsAbs(cmd.Path) || filepath.Clean(cmd.Path) != cmd.Path:
		return fmt.Errorf("%w: path %q must be absolute and clean", ErrBadCommand, cmd.Path)
	case strings.ContainsRune(cmd.Path, 0):
		return fmt.Errorf("%w: NUL in path", ErrBadCommand)
	}
	for i, a := range cmd.Args {
		if strings.ContainsRune(a, 0) {
			return fmt.Errorf("%w: NUL in argv[%d]", ErrBadCommand, i+1)
		}
	}
	return nil
}

// limitedBuffer keeps at most limit bytes and discards the rest. The buffer is a field, not
// embedded: an embedded bytes.Buffer would promote ReadFrom and let io.Copy bypass Write.
type limitedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	room := b.limit - b.buf.Len()
	if room <= 0 {
		b.truncated = true
		return n, nil
	}
	if len(p) > room {
		b.truncated = true
		p = p[:room]
	}
	_, _ = b.buf.Write(p)
	return n, nil // report the full length so the child never sees EPIPE
}

// Bytes returns what was kept.
func (b *limitedBuffer) Bytes() []byte { return b.buf.Bytes() }

// RecordingRunner is the Runner for unit tests: it records every Command and answers with
// the response registered for the binary path (or ErrNotAllowed when none is registered).
type RecordingRunner struct {
	mu        sync.Mutex
	calls     []Command
	responses map[string]func(Command) (Output, error)
}

var _ Runner = (*RecordingRunner)(nil)

// NewRecordingRunner returns an empty recording runner.
func NewRecordingRunner() *RecordingRunner {
	return &RecordingRunner{responses: make(map[string]func(Command) (Output, error))}
}

// On registers the response for path.
func (r *RecordingRunner) On(path string, fn func(Command) (Output, error)) *RecordingRunner {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.responses[path] = fn
	return r
}

// Succeed makes path succeed with stdout.
func (r *RecordingRunner) Succeed(path string, stdout string) *RecordingRunner {
	return r.On(path, func(Command) (Output, error) {
		return Output{Stdout: []byte(stdout)}, nil
	})
}

// FailWith makes path exit with code and stderr.
func (r *RecordingRunner) FailWith(path string, code int, stderr string) *RecordingRunner {
	return r.On(path, func(cmd Command) (Output, error) {
		out := Output{Stderr: []byte(stderr), ExitCode: code}
		return out, &ExitError{Command: cmd, Output: out}
	})
}

// Run implements Runner.
func (r *RecordingRunner) Run(ctx context.Context, cmd Command) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if err := checkCommand(cmd); err != nil {
		return Output{}, err
	}
	r.mu.Lock()
	r.calls = append(r.calls, cmd)
	fn, ok := r.responses[cmd.Path]
	r.mu.Unlock()
	if !ok {
		return Output{}, fmt.Errorf("%w: %s (no response registered)", ErrNotAllowed, cmd.Path)
	}
	return fn(cmd)
}

// Calls returns the recorded commands in order.
func (r *RecordingRunner) Calls() []Command {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Command, len(r.calls))
	copy(out, r.calls)
	return out
}

// Reset forgets the recorded commands.
func (r *RecordingRunner) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = nil
}
