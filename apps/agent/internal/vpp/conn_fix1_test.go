package vpp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"go.fd.io/govpp/adapter/mock"
	"go.fd.io/govpp/api"
	"go.fd.io/govpp/core"

	"ngfw/agent/binapi/vpe"
)

// TD-9 fix round 1, M1: a ReplyTimeout above govpp's global DefaultReplyTimeout really bounds Invoke.
// The global does not cut it short, and the answer is ErrTimeout, which the scheduler treats as an
// uncertain outcome, not a certain failure.
func TestInvokeHonoursAReplyTimeoutAboveGovppsGlobal(t *testing.T) {
	old := core.DefaultReplyTimeout
	core.DefaultReplyTimeout = 200 * time.Millisecond // the 30 s global, scaled down
	t.Cleanup(func() { core.DefaultReplyTimeout = old })
	const timeout = time.Second // VRX_AGENT_VPP_REPLY_TIMEOUT above the global, scaled down
	c := neverReplying(t, timeout)
	took, err := within(t, 10*time.Second, func() error {
		return c.Invoke(context.Background(), &vpe.ShowVersion{}, &vpe.ShowVersionReply{})
	})
	if !isTimeout(err) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want ErrTimeout (a govpp timeout must count as an unknown outcome)", err)
	}
	if took < timeout {
		t.Fatalf("returned after %s: govpp's global %s cut the configured %s short", took, core.DefaultReplyTimeout, timeout)
	}
}

// The bounded Invoke still decodes the reply VPP sends.
func TestInvokeDecodesTheReply(t *testing.T) {
	m := mock.NewVppAdapter()
	conn, err := core.Connect(m)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Disconnect)
	c := testConn(conn, time.Second)
	c.log = slog.New(slog.NewTextHandler(io.Discard, nil))
	c.connected.Store(true)
	m.MockReply(&vpe.ShowVersionReply{Program: "vpe", Version: "26.06-mock"})
	var rep vpe.ShowVersionReply
	if err := c.Invoke(context.Background(), &vpe.ShowVersion{}, &rep); err != nil || rep.Version != "26.06-mock" {
		t.Fatalf("reply %+v err %v", rep, err)
	}
	// A client that is not a Conn (vpp.Bounded) maps govpp's own reply timeout to ErrTimeout too.
	b := Bounded(replyTimeoutClient{}, time.Second)
	if err := b.Invoke(context.Background(), &vpe.ShowVersion{}, &vpe.ShowVersionReply{}); !isTimeout(err) {
		t.Fatalf("Bounded: err = %v, want ErrTimeout", err)
	}
}

// replyTimeoutClient answers every Invoke with govpp's own reply-timeout error.
type replyTimeoutClient struct{ Client }

func (replyTimeoutClient) Invoke(context.Context, api.Message, api.Message) error {
	return fmt.Errorf("%w %s", core.ErrReplyTimeout, 30*time.Second)
}
