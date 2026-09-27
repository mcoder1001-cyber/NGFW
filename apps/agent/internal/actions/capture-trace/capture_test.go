package capturetrace

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/bpf_trace_filter"
	interfaces "ngfw/agent/binapi/interface"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
)

// pcapFile builds a little-endian classic pcap with n 60-byte records.
func pcapFile(n int) []byte {
	b := make([]byte, 24)
	binary.LittleEndian.PutUint32(b, 0xa1b2c3d4)
	for i := 0; i < n; i++ {
		h := make([]byte, 16)
		binary.LittleEndian.PutUint32(h[8:], 60)
		binary.LittleEndian.PutUint32(h[12:], 60)
		b = append(b, h...)
		b = append(b, make([]byte, 60)...)
	}
	return b
}

type rig struct {
	f       *dfkittest.FakeVPP
	vppDir  string
	dir     string
	mu      sync.Mutex
	running bool
	file    string
	filter  string
	fn      string
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{f: dfkittest.NewFake(dfkittest.Iface{Index: 7, Name: "loop501", Tag: "w5:loop501"}, dfkittest.Iface{Index: 8, Name: "loop601", Tag: "w6:loop601"}),
		vppDir: t.TempDir(), dir: filepath.Join(t.TempDir(), "captures"), fn: "vnet_is_packet_traced"}
	r.f.On("pcap_trace_on", func(msg api.Message) ([]api.Message, error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.running {
			return []api.Message{&interfaces.PcapTraceOnReply{Retval: int32(api.INVALID_VALUE)}}, nil
		}
		r.running, r.file = true, msg.(*interfaces.PcapTraceOn).Filename
		return []api.Message{&interfaces.PcapTraceOnReply{}}, nil
	})
	r.f.On("pcap_trace_off", func(api.Message) ([]api.Message, error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		if !r.running {
			return []api.Message{&interfaces.PcapTraceOffReply{Retval: int32(api.VALUE_EXIST)}}, nil
		}
		r.running = false
		if err := os.WriteFile(filepath.Join(r.vppDir, r.file), pcapFile(3), 0o664); err != nil { //nolint:gosec // VPP's mode
			return nil, err
		}
		return []api.Message{&interfaces.PcapTraceOffReply{}}, nil
	})
	r.f.On("pcap_set_filter_function", func(msg api.Message) ([]api.Message, error) {
		r.fn = msg.(*interfaces.PcapSetFilterFunction).FilterFunctionName
		return []api.Message{&interfaces.PcapSetFilterFunctionReply{}}, nil
	})
	r.f.On("bpf_trace_filter_set_v2", func(msg api.Message) ([]api.Message, error) {
		m := msg.(*bpf_trace_filter.BpfTraceFilterSetV2)
		if m.IsAdd {
			r.filter = m.Filter
		} else {
			r.filter = ""
		}
		return []api.Message{&bpf_trace_filter.BpfTraceFilterSetV2Reply{}}, nil
	})
	return r
}

func (r *rig) manager(t *testing.T, globals bool, boot dfkit.BootStore) *Manager {
	t.Helper()
	m, err := New(Config{Client: r.f, Owner: "w5", GlobalsOwner: globals, Dir: r.dir, VPPDir: r.vppDir, Boot: boot, Tick: 10 * time.Millisecond, MaxFiles: 2})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestValidate(t *testing.T) {
	p, err := Validate(&vrxv1.CaptureAction{Interface: "loop501"})
	if err != nil || !p.Rx || !p.Tx || p.Drop || p.MaxPackets != 1000 || p.Seconds != 30 || p.Snaplen != 9000 {
		t.Fatalf("%+v %v", p, err)
	}
	if p, _ := Validate(&vrxv1.CaptureAction{Interface: "any", Drop: true}); p.Direction() != "drop" {
		t.Fatal(p.Direction())
	}
	for field, a := range map[string]*vrxv1.CaptureAction{
		"interface":   {},
		"bpf":         {Interface: "loop501", Bpf: "icmp; rm -rf /"},
		"seconds":     {Interface: "loop501", Seconds: 601},
		"maxPackets":  {Interface: "loop501", MaxPackets: 100001},
		"snaplen":     {Interface: "loop501", Snaplen: 20},
		"errorFilter": {Interface: "loop501", ErrorFilter: "ip4-input/ttl_expired"},
	} {
		_, err := Validate(a)
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), ": "+field+": ") {
			t.Errorf("%s: %v", field, err)
		}
	}
	if _, err := Validate(&vrxv1.CaptureAction{Interface: "loop501", Bpf: `host "x"`}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func run(ctx context.Context, t *testing.T, m *Manager, a *vrxv1.CaptureAction) ([]string, *vrxv1.ActionDone, error) {
	t.Helper()
	p, err := Validate(a)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	var done *vrxv1.ActionDone
	err = m.Run(ctx, p, func(o *vrxv1.ActionOutput) error {
		if l := o.GetLine(); l != "" {
			lines = append(lines, l)
		}
		if o.GetDone() != nil {
			done = o.GetDone()
		}
		return nil
	})
	return lines, done, err
}

func TestCaptureKeepsFile0600(t *testing.T) {
	r := newRig(t)
	m := r.manager(t, true, nil)
	lines, done, err := run(context.Background(), t, m, &vrxv1.CaptureAction{Interface: "loop501", Seconds: 1, Bpf: "icmp"})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) < 2 || !strings.HasPrefix(lines[0], "capture w5-") {
		t.Fatalf("lines %v", lines)
	}
	id := done.GetStats()["id"]
	if done.GetExitCode() != 0 || done.GetStats()["packets"] != "3" || done.GetStats()["state"] != "done" {
		t.Fatalf("done %+v", done)
	}
	fi, err := os.Stat(filepath.Join(r.dir, id+".pcap"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("kept file: %v %v", fi, err)
	}
	if _, err := os.Stat(filepath.Join(r.vppDir, id+".pcap")); !os.IsNotExist(err) {
		t.Fatal("file left in VPP's dir")
	}
	if r.filter != "" || r.fn != "vnet_is_packet_traced" {
		t.Fatalf("filter not restored: %q %q", r.filter, r.fn)
	}
	var got []byte
	if err := m.Read(id, func(c *vrxv1.CaptureChunk) error { got = append(got, c.GetData()...); return nil }); err != nil || len(got) != len(pcapFile(3)) {
		t.Fatalf("read %d %v", len(got), err)
	}
	l, _ := m.List(context.Background())
	if len(l.Captures) != 1 || l.Captures[0].GetSha256() == "" || l.TraceAvailable || l.PgAvailable || l.TraceReason == "" {
		t.Fatalf("list %+v", l)
	}
	if n, err := m.Delete(id); err != nil || n == 0 {
		t.Fatal(n, err)
	}
	if _, err := os.Stat(filepath.Join(r.dir, id+".pcap")); !os.IsNotExist(err) {
		t.Fatal("file not deleted")
	}
	if _, err := m.Delete(id); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := m.Delete("../x"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestBusyAndGlobals(t *testing.T) {
	r := newRig(t)
	m := r.manager(t, false, nil)
	if _, _, err := run(context.Background(), t, m, &vrxv1.CaptureAction{Interface: "loop501", Bpf: "icmp"}); !errors.Is(err, ErrGlobals) {
		t.Fatal(err)
	}
	r.running = true // another owner's capture
	if _, _, err := run(context.Background(), t, m, &vrxv1.CaptureAction{Interface: "loop501", Seconds: 1}); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	r.running = false
	// this agent's own running capture
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	go func() {
		p, _ := Validate(&vrxv1.CaptureAction{Interface: "loop501", Seconds: 60})
		once := sync.Once{}
		_ = m.Run(ctx, p, func(*vrxv1.ActionOutput) error { once.Do(func() { close(started) }); return nil })
	}()
	<-started
	if _, _, err := run(context.Background(), t, m, &vrxv1.CaptureAction{Interface: "loop501", Seconds: 1}); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	l, _ := m.List(context.Background())
	if len(l.Captures) != 1 || l.Captures[0].State != "running" {
		t.Fatalf("%+v", l.Captures)
	}
	if _, err := m.Delete(l.Captures[0].Id); !errors.Is(err, ErrRunning) {
		t.Fatal(err)
	}
	cancel()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		l, _ = m.List(context.Background())
		if l.Captures[0].State != "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if l.Captures[0].State != "done" || l.Captures[0].Reason != "cancelled" {
		t.Fatalf("%+v", l.Captures[0])
	}
	// another owner's interface
	if _, _, err := run(context.Background(), t, m, &vrxv1.CaptureAction{Interface: "loop601", Seconds: 1}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestRetention(t *testing.T) {
	r := newRig(t)
	m := r.manager(t, false, nil)
	now := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	m.c.Now = func() time.Time { now = now.Add(time.Second); return now }
	for i := 0; i < 4; i++ {
		if _, _, err := run(context.Background(), t, m, &vrxv1.CaptureAction{Interface: "any", Seconds: 1}); err != nil {
			t.Fatal(err)
		}
	}
	l, _ := m.List(context.Background())
	if len(l.Captures) != 2 {
		t.Fatalf("retention kept %d", len(l.Captures))
	}
	pcaps, _ := filepath.Glob(filepath.Join(r.dir, "*.pcap"))
	if len(pcaps) != 2 {
		t.Fatal(pcaps)
	}
}

// TestAgentRestartDuringCapture: a record left "running" by a previous process is stopped (same VPP
// boot: pcap_trace_off) and recorded as interrupted with its file kept.
func TestAgentRestartDuringCapture(t *testing.T) {
	r := newRig(t)
	boot := dfkit.NewMemoryBootStore()
	m1 := r.manager(t, false, boot)
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	go func() {
		p, _ := Validate(&vrxv1.CaptureAction{Interface: "loop501", Seconds: 60})
		once := sync.Once{}
		// the "old process" never returns from its stream: block until the test ends
		_ = m1.Run(ctx, p, func(*vrxv1.ActionOutput) error {
			once.Do(func() { close(started) })
			<-ctx.Done()
			return ctx.Err()
		})
	}()
	<-started
	m2 := r.manager(t, false, boot) // new process, same dir and boot store
	l, err := m2.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Captures) != 1 || l.Captures[0].State != "interrupted" || l.Captures[0].Reason != "agent-restart" || l.Captures[0].Packets != 3 {
		t.Fatalf("%+v", l.Captures)
	}
	if r.running {
		t.Fatal("capture still running in VPP")
	}
	cancel()
}
