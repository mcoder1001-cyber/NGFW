package vpp

import (
	"errors"
	"time"

	"go.fd.io/govpp/core"
)

// TD-9 test accessors for the new Conn API. The base-first evidence (docs/status/tasks/TD-9.md) swaps
// this file for a shim that encodes the base's behaviour: no reply timeout, no ErrTimeout.

func testConn(conn *core.Connection, timeout time.Duration) *Conn {
	return &Conn{opts: ConnOptions{ReplyTimeout: timeout}, conn: conn}
}

func replyTimeoutOf(c *Conn) time.Duration { return c.opts.ReplyTimeout }

func isTimeout(err error) bool { return errors.Is(err, ErrTimeout) }
