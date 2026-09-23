package kea

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
)

// Kea control channel: JSON commands over the daemons' unix control sockets (the agent runs
// on the same host), or through kea-ctrl-agent's HTTP listener on loopback. No process is
// spawned for apply or retrieve.

// ErrNotRunning is returned when a daemon's control socket does not exist or refuses the
// connection.
var ErrNotRunning = errors.New("kea: daemon not running")

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
	if _, err := os.Stat(c.Socket); err != nil {
		return Response{}, fmt.Errorf("%w: %s: %w", ErrNotRunning, c.Socket, err)
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

func (r Response) err(command string) error {
	if r.Result == ResultSuccess || r.Result == ResultEmpty {
		return nil
	}
	return fmt.Errorf("%w: %s: result %d: %s", ErrCommand, command, r.Result, r.Text)
}

// HTTPClient talks to kea-ctrl-agent (loopback only). Service "" addresses the agent itself.
type HTTPClient struct {
	Host    string
	Port    uint16
	Timeout time.Duration
}

// Command sends one command, optionally forwarded to service ("dhcp4", "dhcp6").
func (c HTTPClient) Command(ctx context.Context, service, command string, args any) (Response, error) {
	req := request{Command: command, Arguments: args}
	if service != "" {
		req.Service = []string{service}
	}
	body, err := json.Marshal(req)
	if err != nil {
		return Response{}, fmt.Errorf("kea: marshal %s: %w", command, err)
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	url := "http://" + net.JoinHostPort(c.Host, strconv.Itoa(int(c.Port))) + "/"
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	hresp, err := http.DefaultClient.Do(hreq)
	if err != nil {
		return Response{}, fmt.Errorf("%w: kea-ctrl-agent %s: %w", ErrNotRunning, url, err)
	}
	defer func() { _ = hresp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(hresp.Body, MaxResponse))
	if err != nil {
		return Response{}, fmt.Errorf("kea: kea-ctrl-agent %s: %w", url, err)
	}
	// The agent answers with a list (one entry per service), itself with an object.
	var list []Response
	if err := json.Unmarshal(raw, &list); err == nil {
		if len(list) == 0 {
			return Response{}, fmt.Errorf("kea: kea-ctrl-agent: empty answer to %s", command)
		}
		return list[0], list[0].err(command)
	}
	var one Response
	if err := json.Unmarshal(raw, &one); err != nil {
		return Response{}, fmt.Errorf("kea: kea-ctrl-agent: undecodable answer to %s: %.200s", command, raw)
	}
	return one, one.err(command)
}
