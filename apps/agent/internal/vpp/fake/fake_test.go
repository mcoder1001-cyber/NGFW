package fake

import (
	"context"
	"errors"
	"io"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/internal/vpp"
)

// Test-local message types stand in for generated binapi (P04 is not merged yet). Only the
// names matter to the fake.
type (
	pingReq     struct{}
	pingReply   struct{}
	createReq   struct{ Instance uint32 }
	createReply struct {
		Retval    int32
		SwIfIndex uint32
	}
	dumpReq      struct{}
	detailsReply struct{ SwIfIndex uint32 }
	linkEvent    struct{ SwIfIndex uint32 }
	otherReply   struct{}
)

func (*pingReq) GetMessageName() string               { return ControlPing }
func (*pingReq) GetCrcString() string                 { return "0" }
func (*pingReq) GetMessageType() api.MessageType      { return api.RequestMessage }
func (*pingReply) GetMessageName() string             { return "control_ping_reply" }
func (*pingReply) GetCrcString() string               { return "0" }
func (*pingReply) GetMessageType() api.MessageType    { return api.ReplyMessage }
func (*createReq) GetMessageName() string             { return "create_loopback_instance" }
func (*createReq) GetCrcString() string               { return "0" }
func (*createReq) GetMessageType() api.MessageType    { return api.RequestMessage }
func (*createReply) GetMessageName() string           { return "create_loopback_instance_reply" }
func (*createReply) GetCrcString() string             { return "0" }
func (*createReply) GetMessageType() api.MessageType  { return api.ReplyMessage }
func (*dumpReq) GetMessageName() string               { return "sw_interface_dump" }
func (*dumpReq) GetCrcString() string                 { return "0" }
func (*dumpReq) GetMessageType() api.MessageType      { return api.RequestMessage }
func (*detailsReply) GetMessageName() string          { return "sw_interface_details" }
func (*detailsReply) GetCrcString() string            { return "0" }
func (*detailsReply) GetMessageType() api.MessageType { return api.ReplyMessage }
func (*linkEvent) GetMessageName() string             { return "sw_interface_event" }
func (*linkEvent) GetCrcString() string               { return "0" }
func (*linkEvent) GetMessageType() api.MessageType    { return api.EventMessage }
func (*otherReply) GetMessageName() string            { return "other_reply" }
func (*otherReply) GetCrcString() string              { return "0" }
func (*otherReply) GetMessageType() api.MessageType   { return api.ReplyMessage }

func TestInvokeCannedAndRecorded(t *testing.T) {
	f := New()
	f.Reply("create_loopback_instance", &createReply{SwIfIndex: 7})
	var rep createReply
	if err := f.Invoke(context.Background(), &createReq{Instance: 200}, &rep); err != nil {
		t.Fatal(err)
	}
	if rep.SwIfIndex != 7 {
		t.Fatalf("reply not copied: %+v", rep)
	}
	calls := f.CallsNamed("create_loopback_instance")
	if len(calls) != 1 || calls[0].(*createReq).Instance != 200 {
		t.Fatalf("recorded calls = %#v", calls)
	}
	f.Reset()
	if len(f.Calls()) != 0 {
		t.Fatal("Reset did not clear calls")
	}
}

func TestInvokeErrors(t *testing.T) {
	f := New()
	ctx := context.Background()
	err := f.Invoke(ctx, &createReq{}, &createReply{})
	if !errors.Is(err, ErrNoHandler) {
		t.Fatalf("unknown message: got %v, want ErrNoHandler", err)
	}
	boom := errors.New("boom")
	f.Fail("create_loopback_instance", boom)
	if err := f.Invoke(ctx, &createReq{}, &createReply{}); !errors.Is(err, boom) {
		t.Fatalf("Fail: got %v", err)
	}
	f.Reply("create_loopback_instance", &otherReply{})
	if err := f.Invoke(ctx, &createReq{}, &createReply{}); !errors.Is(err, ErrReplyType) {
		t.Fatalf("type mismatch: got %v, want ErrReplyType", err)
	}
	f.Reply("create_loopback_instance", &createReply{}, &createReply{})
	if err := f.Invoke(ctx, &createReq{}, &createReply{}); !errors.Is(err, ErrReplyType) {
		t.Fatalf("two replies: got %v, want ErrReplyType", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	f.Reply("create_loopback_instance", &createReply{})
	if err := f.Invoke(cancelled, &createReq{}, &createReply{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ctx: got %v", err)
	}
	f.SetConnected(false)
	if f.Connected() {
		t.Fatal("SetConnected(false) ignored")
	}
	if err := f.Invoke(ctx, &createReq{}, &createReply{}); !errors.Is(err, vpp.ErrDisconnected) {
		t.Fatalf("disconnected: got %v, want vpp.ErrDisconnected", err)
	}
	if _, err := f.NewStream(ctx); !errors.Is(err, vpp.ErrDisconnected) {
		t.Fatalf("disconnected stream: got %v", err)
	}
	// Four requests reached a handler; the cancelled and the disconnected calls were refused
	// before being recorded.
	if len(f.Calls()) != 4 {
		t.Fatalf("refused calls must not be recorded; calls = %d", len(f.Calls()))
	}
}

// recvDetails mimics the generated RPCService_XxxDumpClient.Recv loop.
func recvDetails(t *testing.T, s api.Stream) []*detailsReply {
	t.Helper()
	var out []*detailsReply
	for {
		msg, err := s.RecvMsg()
		if err != nil {
			t.Fatalf("RecvMsg: %v", err)
		}
		switch m := msg.(type) {
		case *detailsReply:
			out = append(out, m)
		case *pingReply:
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			return out
		default:
			t.Fatalf("unexpected message %T", m)
		}
	}
}

func TestDumpStreamPattern(t *testing.T) {
	f := New(WithControlPingReply(&pingReply{}))
	f.Reply("sw_interface_dump", &detailsReply{SwIfIndex: 1}, &detailsReply{SwIfIndex: 2})
	ctx := context.Background()
	s, err := f.NewStream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s.Context() != ctx {
		t.Fatal("stream context not propagated")
	}
	if err := s.SendMsg(&dumpReq{}); err != nil {
		t.Fatal(err)
	}
	if err := s.SendMsg(&pingReq{}); err != nil {
		t.Fatal(err)
	}
	got := recvDetails(t, s)
	if len(got) != 2 || got[0].SwIfIndex != 1 || got[1].SwIfIndex != 2 {
		t.Fatalf("details = %+v", got)
	}
	if _, err := s.RecvMsg(); !errors.Is(err, ErrStreamClosed) {
		t.Fatalf("after Close: got %v", err)
	}
	if names := f.Calls(); len(names) != 2 || names[1].GetMessageName() != ControlPing {
		t.Fatalf("calls = %v", names)
	}
}

func TestDumpWithoutPingReplyFailsLoudly(t *testing.T) {
	f := New()
	f.Reply("sw_interface_dump")
	s, err := f.NewStream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SendMsg(&dumpReq{}); err != nil {
		t.Fatal(err)
	}
	if err := s.SendMsg(&pingReq{}); !errors.Is(err, ErrNoHandler) {
		t.Fatalf("control_ping without reply: got %v, want ErrNoHandler", err)
	}
	if _, err := s.RecvMsg(); !errors.Is(err, ErrNoReply) {
		t.Fatalf("empty queue: got %v, want ErrNoReply (never io.EOF: %v)", err, io.EOF)
	}
}

func TestStatefulHandler(t *testing.T) {
	// Handlers keep state in closures to model VPP: created loopbacks appear in the dump.
	var table []*detailsReply
	next := uint32(10)
	f := New(WithControlPingReply(&pingReply{}))
	f.On("create_loopback_instance", func(api.Message) ([]api.Message, error) {
		idx := next
		next++
		table = append(table, &detailsReply{SwIfIndex: idx})
		return []api.Message{&createReply{SwIfIndex: idx}}, nil
	})
	f.On("sw_interface_dump", func(api.Message) ([]api.Message, error) {
		out := make([]api.Message, 0, len(table))
		for _, d := range table {
			out = append(out, d)
		}
		return out, nil
	})
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		var rep createReply
		if err := f.Invoke(ctx, &createReq{}, &rep); err != nil {
			t.Fatal(err)
		}
	}
	s, _ := f.NewStream(ctx)
	_ = s.SendMsg(&dumpReq{})
	_ = s.SendMsg(&pingReq{})
	if got := recvDetails(t, s); len(got) != 2 || got[1].SwIfIndex != 11 {
		t.Fatalf("dump after two creates = %+v", got)
	}
}

func TestWatchEventAndEmit(t *testing.T) {
	f := New()
	ctx, cancel := context.WithCancel(context.Background())
	w, err := f.WatchEvent(ctx, &linkEvent{})
	if err != nil {
		t.Fatal(err)
	}
	if n := f.Emit(&linkEvent{SwIfIndex: 3}); n != 1 {
		t.Fatalf("Emit delivered to %d watchers, want 1", n)
	}
	if n := f.Emit(&otherReply{}); n != 0 {
		t.Fatalf("unrelated event delivered to %d watchers", n)
	}
	ev := <-w.Events()
	if ev.(*linkEvent).SwIfIndex != 3 {
		t.Fatalf("event = %+v", ev)
	}
	cancel()
	for range w.Events() {
		// drain until closed by the context
	}
	if n := f.Emit(&linkEvent{}); n != 0 {
		t.Fatalf("closed watcher still received: %d", n)
	}
	w.Close() // idempotent
	if _, err := New(WithDisconnected()).WatchEvent(context.Background(), &linkEvent{}); !errors.Is(err, vpp.ErrDisconnected) {
		t.Fatalf("disconnected WatchEvent: got %v", err)
	}
}
