package vpp

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"go.fd.io/govpp/adapter/mock"
	"go.fd.io/govpp/api"
	"go.fd.io/govpp/core"

	"ngfw/agent/binapi/vpe"
)

// neverReplying is a real govpp connection over govpp's mock adapter whose VPP never answers: every reply
// carries a message id govpp does not know, so it is dropped (TD-9: the "VPP dies mid-request" case).
func neverReplying(t *testing.T, timeout time.Duration) *Conn {
	t.Helper()
	m := mock.NewVppAdapter()
	m.MockReplyHandler(func(mock.MessageDTO) ([]byte, uint16, bool) { return nil, 0xffff, true })
	conn, err := core.Connect(m)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Disconnect)
	c := testConn(conn, timeout)
	c.log = slog.New(slog.NewTextHandler(io.Discard, nil))
	c.connected.Store(true)
	return c
}

// within runs f and fails the test when it does not return within d (the base blocks forever).
func within(t *testing.T, d time.Duration, f func() error) (time.Duration, error) {
	t.Helper()
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- f() }()
	select {
	case err := <-done:
		return time.Since(start), err
	case <-time.After(d):
		t.Fatalf("the call did not return within %s: VPP never replies and nothing bounds the wait", d)
		return 0, nil
	}
}

func TestDefaultReplyTimeoutIsBounded(t *testing.T) {
	if core.DefaultReplyTimeout <= 0 {
		t.Fatalf("core.DefaultReplyTimeout = %v: govpp's 0 waits forever for a reply", core.DefaultReplyTimeout)
	}
	c := Dial("/nonexistent/vrx-test-api.sock", ConnOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	defer c.Close()
	if d := replyTimeoutOf(c); d != core.DefaultReplyTimeout {
		t.Fatalf("Dial's default reply timeout = %v, want %v", d, core.DefaultReplyTimeout)
	}
}

// TD-9 (review 1.1): an Invoke whose reply never comes returns ErrTimeout after ReplyTimeout, with a
// deadline-less ctx (the agent's own transaction context has none per call).
func TestInvokeNeverRepliedTimesOut(t *testing.T) {
	const timeout = 200 * time.Millisecond
	c := neverReplying(t, timeout)
	took, err := within(t, 5*time.Second, func() error {
		return c.Invoke(context.Background(), &vpe.ShowVersion{}, &vpe.ShowVersionReply{})
	})
	if !isTimeout(err) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
	if took < timeout || took > timeout+2*time.Second {
		t.Fatalf("returned after %s, want ≈ %s", took, timeout)
	}
	// A caller's earlier deadline wins and stays the caller's error.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = within(t, 5*time.Second, func() error { return c.Invoke(ctx, &vpe.ShowVersion{}, &vpe.ShowVersionReply{}) })
	if !errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
		t.Fatalf("caller deadline: err = %v, want the caller's context.DeadlineExceeded", err)
	}
}

// Every message of a stream (a dump) is bounded too; the caller's own option overrides the default.
func TestStreamNeverRepliedTimesOut(t *testing.T) {
	const timeout = 200 * time.Millisecond
	c := neverReplying(t, timeout)
	recv := func(opts ...api.StreamOption) func() error {
		return func() error {
			st, err := c.NewStream(context.Background(), opts...)
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()
			if err := st.SendMsg(&vpe.ShowVersion{}); err != nil {
				return err
			}
			_, err = st.RecvMsg()
			return err
		}
	}
	took, err := within(t, 5*time.Second, recv())
	if !isTimeout(err) || !errors.Is(err, core.ErrReplyTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
	if took < timeout || took > timeout+2*time.Second {
		t.Fatalf("returned after %s, want ≈ %s", took, timeout)
	}
	took, _ = within(t, 5*time.Second, recv(core.WithReplyTimeout(600*time.Millisecond)))
	if took < 600*time.Millisecond {
		t.Fatalf("the caller's WithReplyTimeout did not override the default: returned after %s", took)
	}
}
