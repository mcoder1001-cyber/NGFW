package chrony

import (
	"context"
	"net"
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
	if len(kvs) > 1 || (len(kvs) == 1 && kvs[0].Key != Key) {
		t.Fatalf("retrieve %v", kvs)
	}
	if len(kvs) == 0 {
		return nil
	}
	return kvs[0].Value
}

func TestDescriptorLifecycle(t *testing.T) {
	ctx := context.Background()
	p := tmpPaths(t)
	rr := renderers.NewRecordingRunner().Succeed(ChronydBin, "").Succeed(ChronycBin, "200 OK")
	d := NewDescriptor(New(rr, WithPaths(p), WithSecrets(resolver)), nil)
	if d.Name() != Name || Key != "chrony.config/vrx" || d.KeyOf(nil) != Key || d.Dependencies(nil) != nil {
		t.Fatal("identity")
	}
	if v := retrieveOne(t, d); v != nil {
		t.Fatalf("nothing written: %v", v)
	}
	in := Input(clientNTP())
	if in == nil || Input(&vrxv1.NtpService{Enabled: proto.Bool(false)}) != nil {
		t.Fatal("Input: enabled only")
	}
	// not running: files written, start request (not an error)
	if _, err := d.Create(ctx, in); err != nil {
		t.Fatal(err)
	}
	if got := retrieveOne(t, d); !proto.Equal(got, in) {
		t.Fatalf("retrieve after create:\n got %v\nwant %v", got, in)
	}
	src, _ := os.ReadFile(p.Sources())
	conf, _ := os.ReadFile(p.Conf())
	if !strings.Contains(string(src), inputPrefix) || strings.Contains(string(conf), inputPrefix) {
		t.Fatal("the input belongs in vrx.sources (reloadable), never in chrony.conf")
	}
	if pend := d.Pending(ctx); len(pend) != 1 || pend[0].Action != "start" || pend[0].Unit != "chrony" {
		t.Fatalf("pending %v", pend)
	}
	// a hand-edited keys file is drift, and the drift value never quotes it
	if err := os.WriteFile(p.Keys(), []byte("1 SHA256 HEX:DEADBEEF\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	drift, ok := retrieveOne(t, d).(*structpb.Struct)
	if !ok || strings.Contains(drift.String(), "DEADBEEF") || !strings.Contains(drift.String(), "chrony.keys") {
		t.Fatalf("drift %v", drift)
	}
	if _, err := d.Update(ctx, drift, in, nil); err != nil {
		t.Fatal(err)
	}
	if got := retrieveOne(t, d); !proto.Equal(got, in) {
		t.Fatalf("after update: %v", got)
	}
	if err := d.Delete(ctx, in, nil); err != nil {
		t.Fatal(err)
	}
	if v := retrieveOne(t, d); v != nil {
		t.Fatalf("after delete: %v", v)
	}
	if pend := d.Pending(ctx); len(pend) != 0 {
		t.Fatalf("disabled rendering, not running: pending %v", pend)
	}
}

func TestDescriptorRestartPending(t *testing.T) {
	ctx := context.Background()
	p := tmpPaths(t)
	c, err := net.ListenPacket("unixgram", p.Socket())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	writePid(t, p, os.Getpid())
	rr := renderers.NewRecordingRunner().Succeed(ChronydBin, "").Succeed(ChronycBin, "200 OK")
	d := NewDescriptor(New(rr, WithPaths(p), WithSecrets(resolver)), nil)
	if _, err := d.Create(ctx, Input(clientNTP())); err != nil {
		t.Fatal(err)
	}
	for _, dx := range []*Descriptor{d, NewDescriptor(New(rr, WithPaths(p), WithSecrets(resolver)), nil)} { // agent restart
		if pend := dx.Pending(ctx); len(pend) != 1 || pend[0].Action != "restart" {
			t.Fatalf("pending %v", pend)
		}
	}
	writePid(t, p, startChild(t))
	if pend := d.Pending(ctx); len(pend) != 0 {
		t.Fatalf("pending after the restart: %v", pend)
	}
}
