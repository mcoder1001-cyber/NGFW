package unbound

import (
	"context"
	"os"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
)

// F-unbound-chrony-syslog: the singleton descriptor around the renderer.

func retrieveOne(t *testing.T, d *Descriptor) proto.Message {
	t.Helper()
	kvs, err := d.Retrieve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	switch len(kvs) {
	case 0:
		return nil
	case 1:
		if kvs[0].Key != Key {
			t.Fatalf("key %s", kvs[0].Key)
		}
		return kvs[0].Value
	}
	t.Fatalf("retrieve returned %d objects", len(kvs))
	return nil
}

func TestDescriptorLifecycle(t *testing.T) {
	ctx := context.Background()
	p := tmpPaths(t)
	rr := renderers.NewRecordingRunner().Succeed(CheckconfBin, "")
	d := NewDescriptor(New(rr, WithPaths(p)), nil)
	prepared := 0
	d.WithPrepare(func() error { prepared++; return nil })
	if d.Name() != Name || Key != "unbound.config/vrx" || d.KeyOf(nil) != Key || d.Dependencies(nil) != nil {
		t.Fatal("identity")
	}
	if v := retrieveOne(t, d); v != nil {
		t.Fatalf("nothing written yet, got %v", v)
	}
	// a foreign unbound.conf (Debian's default, no embedded input) is never reported
	if err := os.WriteFile(p.Conf(), []byte("server:\n\tverbosity: 1\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if v := retrieveOne(t, d); v != nil {
		t.Fatalf("foreign file reported as %v", v)
	}

	in := Input(dns(map[string]*vrxv1.DnsResolver{"lan": fullResolver(), "off": {Enabled: proto.Bool(false), Listen: []*vrxv1.SocketAddress{listen("127.0.0.1", 3654)}}}))
	// not running: the file is written, the start request is not a failure
	if _, err := d.Create(ctx, in); err != nil {
		t.Fatalf("create: %v", err)
	}
	if prepared != 1 {
		t.Fatalf("prepare ran %d times", prepared)
	}
	if got := retrieveOne(t, d); !proto.Equal(got, in) {
		t.Fatalf("retrieve after create:\n got %v\nwant %v", got, in)
	}
	if pend := d.Pending(ctx); len(pend) != 1 || pend[0].Action != "start" {
		t.Fatalf("pending %v", pend)
	}
	// drift: a hand edit is a Value that never equals the desired one
	conf, _ := os.ReadFile(p.Conf())
	if err := os.WriteFile(p.Conf(), []byte(strings.Replace(string(conf), "verbosity: 1", "verbosity: 5", 1)), 0o640); err != nil {
		t.Fatal(err)
	}
	drift, ok := retrieveOne(t, d).(*structpb.Struct)
	if !ok || !strings.Contains(drift.String(), "verbosity: 5") {
		t.Fatalf("drift value %v", drift)
	}
	if _, err := d.Update(ctx, drift, in, nil); err != nil {
		t.Fatal(err)
	}
	if got := retrieveOne(t, d); !proto.Equal(got, in) {
		t.Fatalf("retrieve after update: %v", got)
	}
	// wrong value type
	if _, err := d.Create(ctx, &vrxv1.NtpService{}); err == nil {
		t.Fatal("wrong value type accepted")
	}
	// delete: back to the idle rendering, which carries no input
	if err := d.Delete(ctx, in, nil); err != nil {
		t.Fatal(err)
	}
	if v := retrieveOne(t, d); v != nil {
		t.Fatalf("after delete: %v", v)
	}
	conf, _ = os.ReadFile(p.Conf())
	if Active(conf) || strings.Contains(string(conf), inputPrefix) {
		t.Fatalf("delete did not render the idle configuration:\n%s", conf)
	}
	if pend := d.Pending(ctx); len(pend) != 0 {
		t.Fatalf("pending after delete %v", pend)
	}
}

func TestDescriptorRestartPendingUntilActedOn(t *testing.T) {
	ctx := context.Background()
	p := tmpPaths(t)
	listenSocket(t, p.ControlSocket())
	fu := &fakeUnbound{pid: os.Getpid(), conf: func() []byte { b, _ := os.ReadFile(p.Conf()); return b }} //nolint:gosec // test
	rr := renderers.NewRecordingRunner().Succeed(CheckconfBin, "").On(ControlBin, fu.run)
	d := NewDescriptor(New(rr, WithPaths(p)), nil)
	in := Input(resolverOn(tcpPort(t)))
	// running, no previous file: the listen sockets change → a persisted restart request, not a failure
	if _, err := d.Create(ctx, in); err != nil {
		t.Fatalf("create: %v", err)
	}
	pend := d.Pending(ctx)
	if len(pend) != 1 || pend[0].Action != "restart" || pend[0].Unit != "unbound" {
		t.Fatalf("pending %v", pend)
	}
	// agent restart simulation: a fresh descriptor over the same paths still reports it
	d2 := NewDescriptor(New(rr, WithPaths(p)), nil)
	if pend := d2.Pending(ctx); len(pend) != 1 || pend[0].Action != "restart" {
		t.Fatalf("pending after agent restart %v", pend)
	}
	if got := retrieveOne(t, d2); !proto.Equal(got, in) {
		t.Fatalf("retrieve after agent restart: %v", got)
	}
	// unbound restarted (a process started after the request): acted on, cleared
	fu.pid = startChild(t)
	if pend := d2.Pending(ctx); len(pend) != 0 {
		t.Fatalf("pending after restart %v", pend)
	}
	if _, err := os.Stat(p.PendingFile); !os.IsNotExist(err) {
		t.Fatalf("pending file kept: %v", err)
	}
}

func TestEmbeddedInput(t *testing.T) {
	if _, ok, err := EmbeddedInput([]byte("# vrx-input: !!\n")); ok || err != nil {
		t.Fatalf("non-base64 line is not an input: %v %v", ok, err)
	}
	if _, _, err := EmbeddedInput([]byte("# vrx-input: AAAA\n# vrx-input: AAAA\n")); err == nil {
		t.Fatal("two input lines accepted")
	}
	if _, _, err := EmbeddedInput([]byte("# vrx-input: /////w==\n")); err == nil {
		t.Fatal("garbage protobuf accepted")
	}
	if Input(nil) != nil || Input(&vrxv1.DnsService{}) != nil {
		t.Fatal("no resolver → no input")
	}
	if _, err := b64("a\nb"); err == nil {
		t.Fatal("b64 accepted a newline")
	}
}
