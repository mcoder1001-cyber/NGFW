package kea

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Kea control channel (D-079): JSON commands over the DHCP servers' own unix control sockets.
// No kea-ctrl-agent (deprecated in Kea 3.0, and an unauthenticated HTTP config-set endpoint),
// no HTTP, no process spawned for apply or retrieve. The socket directory must not be writable
// by group or others and the socket itself not accessible by others (checkSocket).

// ErrNotRunning is returned when a daemon's control socket does not exist or refuses the
// connection.
var ErrNotRunning = errors.New("kea: daemon not running")

// ErrInsecure is returned when the control socket or its directory is accessible to other
// users: the agent refuses to send configuration through it.
var ErrInsecure = errors.New("kea: control socket not private")

// ErrCommand wraps a Kea answer whose result is not success (0) or empty (3).
var ErrCommand = errors.New("kea: command failed")

// Result codes (Kea ARM "Management API").
const (
	ResultSuccess     = 0
	ResultError       = 1
	ResultUnsupported = 2
	ResultEmpty       = 3
	ResultConflict    = 4
)

// MaxResponse bounds one control answer (lease pages keep answers small; config-get of a big
// configuration is the largest message).
const MaxResponse = 64 << 20

// Response is one Kea control answer.
type Response struct {
	Result    int             `json:"result"`
	Text      string          `json:"text,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type request struct {
	Command   string   `json:"command"`
	Service   []string `json:"service,omitempty"`
	Arguments any      `json:"arguments,omitempty"`
}

// Client talks to one Kea daemon over its unix control socket.
type Client struct {
	Socket  string
	Timeout time.Duration
}

// Command sends one command with optional arguments (marshalled with encoding/json;
// json.RawMessage passes rendered JSON through unchanged) and decodes the answer. A result
// other than 0/3 is returned as a Response and an error wrapping ErrCommand.
func (c Client) Command(ctx context.Context, command string, args any) (Response, error) {
	body, err := json.Marshal(request{Command: command, Arguments: args})
	if err != nil {
		return Response{}, fmt.Errorf("kea: marshal %s: %w", command, err)
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := checkSocket(c.Socket); err != nil {
		return Response{}, err
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", c.Socket)
	if err != nil {
		return Response{}, fmt.Errorf("%w: %s: %w", ErrNotRunning, c.Socket, err)
	}
	defer func() { _ = conn.Close() }()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	if _, err := conn.Write(body); err != nil {
		return Response{}, fmt.Errorf("kea: %s: send %s: %w", c.Socket, command, err)
	}
	// Kea writes one JSON object and closes (or keeps the connection idle): decode exactly
	// one value, bounded.
	dec := json.NewDecoder(io.LimitReader(conn, MaxResponse))
	var resp Response
	if err := dec.Decode(&resp); err != nil {
		return Response{}, fmt.Errorf("kea: %s: read answer to %s: %w", c.Socket, command, err)
	}
	return resp, resp.err(command)
}

// checkSocket: the socket exists (else ErrNotRunning), its directory grants nothing to
// others and no write to the group, and the socket grants nothing to others (else
// ErrInsecure). Kea 3.0 creates its socket 0770 and itself refuses a directory looser than
// 0750, so a 0750 root/_kea directory makes the socket reachable by its owner and group only.
func checkSocket(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrNotRunning, path, err)
	}
	if st.Mode().Perm()&0o007 != 0 {
		return fmt.Errorf("%w: %s has mode %v", ErrInsecure, path, st.Mode().Perm())
	}
	dir, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNotRunning, err)
	}
	if dir.Mode().Perm()&0o027 != 0 { // no group write, nothing for others (D-079: private dir)
		return fmt.Errorf("%w: %s has mode %v", ErrInsecure, filepath.Dir(path), dir.Mode().Perm())
	}
	return nil
}

func (r Response) err(command string) error {
	if r.Result == ResultSuccess || r.Result == ResultEmpty {
		return nil
	}
	return fmt.Errorf("%w: %s: result %d: %s", ErrCommand, command, r.Result, r.Text)
}
