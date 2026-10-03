package capturetrace

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
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
	// plant, when set, replaces VPP's write of the capture file (S-capture-file-safety tests: a
	// symlink or a foreign-uid file at VPP's path).
	plant func(path string) error
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
		if r.plant != nil {
			if err := r.plant(filepath.Join(r.vppDir, r.file)); err != nil {
				return nil, err
			}
			return []api.Message{&interfaces.PcapTraceOffReply{}}, nil
		}
		if err := os.WriteFile(filepath.Join(r.vppDir, r.file), pcapFile(3), 0o664); err != nil { //nolint:gosec // VPP's mode
			return nil, err
		}
		return []api.Message{&interfaces.PcapTraceOffReply{}}, nil
	})
	r.f.On("pcap_set_filter_function", func(msg api.Message) ([]api.Message, error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.fn = msg.(*interfaces.PcapSetFilterFunction).FilterFunctionName
		return []api.Message{&interfaces.PcapSetFilterFunctionReply{}}, nil
	})
	r.f.On("bpf_trace_filter_set_v2", func(msg api.Message) ([]api.Message, error) {
		r.mu.Lock()
		defer r.mu.Unlock()
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

// setRunning / vppRunning / filters access the fake VPP state under the rig lock (the handlers
// run on the capture goroutine).
func (r *rig) setRunning(v bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running = v
}

func (r *rig) vppRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

func (r *rig) filters() (string, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.filter, r.fn
}

// background runs a capture on its own goroutine until it has sent its first line and returns a
// stop function that cancels it and waits until Run has returned (record saved, file moved,
// retention applied, m.running released). stop is also registered with t.Cleanup; it is
// registered after the rig's t.TempDir calls, so it runs before their RemoveAll (TD-H7). block
// makes the send callback block until cancellation (a stream that never returns).
func background(t *testing.T, m *Manager, a *vrxv1.CaptureAction, block bool) (stop func() error) {
	t.Helper()
	p, err := Validate(a)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		once := sync.Once{}
		done <- m.Run(ctx, p, func(*vrxv1.ActionOutput) error {
			once.Do(func() { close(started) })
			if block {
				<-ctx.Done()
				return ctx.Err()
			}
			return nil
		})
	}()
	select {
	case <-started:
	case err := <-done:
		cancel()
		t.Fatalf("background capture ended before it started: %v", err)
	}
	var once sync.Once
	var runErr error
	stop = func() error {
		once.Do(func() {
			cancel()
			select {
			case runErr = <-done:
			case <-time.After(30 * time.Second):
				runErr = errors.New("background capture did not return within 30s")
			}
		})
		return runErr
	}
	t.Cleanup(func() { _ = stop() })
	return stop
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
	if filter, fn := r.filters(); filter != "" || fn != "vnet_is_packet_traced" {
		t.Fatalf("filter not restored: %q %q", filter, fn)
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
	r.setRunning(true) // another owner's capture
	if _, _, err := run(context.Background(), t, m, &vrxv1.CaptureAction{Interface: "loop501", Seconds: 1}); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	r.setRunning(false)
	// this agent's own running capture
	stop := background(t, m, &vrxv1.CaptureAction{Interface: "loop501", Seconds: 60}, false)
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
	// Await Run's return, not just the record's state: finish() saves "done" before Run releases
	// m.running, so polling List raced the next Run into ErrBusy (the F-bruteforce-block-host
	// failure "busy … (capture w5-…-2)" was this test's own capture, not a foreign one).
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	l, _ = m.List(context.Background())
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
	// the "old process" never returns from its stream: its send blocks until stop
	stop := background(t, m1, &vrxv1.CaptureAction{Interface: "loop501", Seconds: 60}, true)
	m2 := r.manager(t, false, boot) // new process, same dir and boot store
	l, err := m2.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Captures) != 1 || l.Captures[0].State != "interrupted" || l.Captures[0].Reason != "agent-restart" || l.Captures[0].Packets != 3 {
		t.Fatalf("%+v", l.Captures)
	}
	if r.vppRunning() {
		t.Fatal("capture still running in VPP")
	}
	// TD-H7: the old process's Run still stops (pcap_trace_off → VALUE_EXIST, already stopped),
	// rewrites its record and applies retention in Dir; await it before t.TempDir's RemoveAll.
	// Its error is expected (the capture was already stopped by m2's Recover) and not checked.
	_ = stop()
}

// --- S-capture-file-safety (RV-C R2 #1, #7, #8) ---

var idRE = regexp.MustCompile(`^w5-\d{8}T\d{6}-[0-9a-f]{24}$`)

// TestIDUnpredictable: the id (= the name VPP writes under /tmp) carries 96 random bits, so a
// local user cannot pre-plant /tmp/<next id>.pcap.
func TestIDUnpredictable(t *testing.T) {
	r := newRig(t)
	m := r.manager(t, false, nil)
	seen := map[string]bool{}
	for i := 0; i < 3; i++ {
		_, done, err := run(context.Background(), t, m, &vrxv1.CaptureAction{Interface: "any", Seconds: 1})
		if err != nil {
			t.Fatal(err)
		}
		id := done.GetStats()["id"]
		if !idRE.MatchString(id) || seen[id] {
			t.Fatalf("id %q (seen %v)", id, seen)
		}
		seen[id] = true
	}
}

// refused runs a capture whose file VPP "writes" through plant and asserts the foreign-file refusal:
// Run fails with ErrForeignFile, the record is state "error" with the reason, nothing is kept in
// Dir and Read finds no file.
func refused(t *testing.T, plant func(path string) error) (id string, r *rig) {
	t.Helper()
	r = newRig(t)
	r.plant = plant
	m := r.manager(t, false, nil)
	_, _, runErr := run(context.Background(), t, m, &vrxv1.CaptureAction{Interface: "any", Seconds: 1})
	if !errors.Is(runErr, ErrForeignFile) {
		t.Fatalf("Run: %v", runErr)
	}
	l, err := m.List(context.Background())
	if err != nil || len(l.Captures) != 1 {
		t.Fatalf("%+v %v", l, err)
	}
	c := l.Captures[0]
	if c.State != "error" || !strings.Contains(c.Reason, "foreign file") || c.Size != 0 || c.Sha256 != "" {
		t.Fatalf("record %+v", c)
	}
	t.Logf("refused: Run err=%v; record id=%s state=%s size=%d sha256=%q reason=%q; planted %s left in place",
		runErr, c.Id, c.State, c.Size, c.Sha256, c.Reason, filepath.Join(r.vppDir, c.Id+".pcap"))
	if _, err := os.Lstat(filepath.Join(r.dir, c.Id+".pcap")); !os.IsNotExist(err) {
		t.Fatalf("something kept in Dir: %v", err)
	}
	if err := m.Read(c.Id, func(*vrxv1.CaptureChunk) error { return nil }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Read: %v", err)
	}
	return c.Id, r
}

// TestSymlinkSrcRefused: /tmp/<id>.pcap → a secret file. The agent must neither chmod the target,
// nor move it, nor serve its bytes.
func TestSymlinkSrcRefused(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "shadow")
	if err := os.WriteFile(secret, []byte("secret-bytes"), 0o644); err != nil { //nolint:gosec // the attacker's target
		t.Fatal(err)
	}
	id, r := refused(t, func(path string) error { return os.Symlink(secret, path) })
	fi, err := os.Stat(secret)
	if err != nil || fi.Mode().Perm() != 0o644 {
		t.Fatalf("target touched: %v %v", fi, err)
	}
	b, _ := os.ReadFile(secret) //nolint:gosec // test fixture
	if string(b) != "secret-bytes" {
		t.Fatal("target changed")
	}
	// the planted link is left where it is (not ours to remove)
	if li, err := os.Lstat(filepath.Join(r.vppDir, id+".pcap")); err != nil || li.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("planted link: %v %v", li, err)
	}
}

// TestForeignUIDSrcRefused: a pre-created file owned by another uid at VPP's path (the attacker
// reads the live capture through it). Needs root for chown (CI runs as root).
func TestForeignUIDSrcRefused(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("chown to another uid needs root")
	}
	const nobody = 65534
	id, r := refused(t, func(path string) error {
		if err := os.WriteFile(path, pcapFile(3), 0o666); err != nil { //nolint:gosec // the attacker's mode
			return err
		}
		return os.Chown(path, nobody, nobody)
	})
	fi, err := os.Stat(filepath.Join(r.vppDir, id+".pcap"))
	if err != nil || fi.Mode().Perm() == 0o600 || fi.Sys().(*syscall.Stat_t).Uid != nobody { // never fchmod'ed, never chowned
		t.Fatalf("foreign file touched: %v %v", fi, err)
	}
}

// TestHardLinkSrcRefused: a second link to a root-owned file is refused too (nlink != 1).
func TestHardLinkSrcRefused(t *testing.T) {
	other := filepath.Join(t.TempDir(), "other")
	if err := os.WriteFile(other, pcapFile(1), 0o600); err != nil {
		t.Fatal(err)
	}
	refused(t, func(path string) error { return os.Link(other, path) })
}

// TestReadNoFollow: a kept file that is a symlink (planted in Dir) is never streamed.
func TestReadNoFollow(t *testing.T) {
	r := newRig(t)
	m := r.manager(t, false, nil)
	secret := filepath.Join(t.TempDir(), "shadow")
	if err := os.WriteFile(secret, pcapFile(2), 0o600); err != nil {
		t.Fatal(err)
	}
	rec := &Record{ID: "w5-20260928T000000-000000000000000000000000", State: "done", StartedAt: time.Now()}
	if err := m.save(rec); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, m.path(rec.ID, ".pcap")); err != nil {
		t.Fatal(err)
	}
	var got int
	err := m.Read(rec.ID, func(c *vrxv1.CaptureChunk) error { got += len(c.GetData()); return nil })
	if !errors.Is(err, ErrNotFound) || got != 0 {
		t.Fatalf("Read followed the link: %d %v", got, err)
	}
}

// TestCopyFromVerifiedFD: moveFile's cross-file-system path copies from the verified fd into a
// new 0600 file (never through a link at dst) and removes the source; hash and count are read from
// the same fd. openVPPFile has already made the source 0600 through the fd.
func TestCopyFromVerifiedFD(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.pcap")
	if err := os.WriteFile(src, pcapFile(4), 0o664); err != nil { //nolint:gosec // VPP's mode
		t.Fatal(err)
	}
	f, err := openVPPFile(src)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if fi, _ := f.Stat(); fi.Mode().Perm() != 0o600 {
		t.Fatalf("fchmod: %v", fi.Mode())
	}
	dst := filepath.Join(dir, "dst.pcap")
	if err := os.Symlink(filepath.Join(dir, "elsewhere"), dst); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(f, src, dst); !errors.Is(err, os.ErrExist) {
		t.Fatalf("copy through a link at dst: %v", err)
	}
	_ = os.Remove(dst)
	if err := copyFile(f, src, dst); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(dst); err != nil || fi.Mode().Perm() != 0o600 || fi.Size() != int64(len(pcapFile(4))) {
		t.Fatalf("dst: %v %v", fi, err)
	}
	if _, err := os.Lstat(src); !os.IsNotExist(err) {
		t.Fatal("src left behind")
	}
	size, pkts, sum, err := inspect(f)
	if err != nil || size != uint64(len(pcapFile(4))) || pkts != 4 || sum == "" {
		t.Fatalf("%d %d %q %v", size, pkts, sum, err)
	}
	// a truncated last record: counted up to it, sized and hashed whole
	trunc := filepath.Join(dir, "trunc.pcap")
	if err := os.WriteFile(trunc, pcapFile(2)[:24+76+30], 0o600); err != nil {
		t.Fatal(err)
	}
	g, err := openVPPFile(trunc)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = g.Close() }()
	if size, pkts, _, err := inspect(g); err != nil || size != 24+76+30 || pkts != 1 {
		t.Fatalf("truncated: %d %d %v", size, pkts, err)
	}
}

// TestClearFilterRestoresOwned ([R4] R2 #8): after a filtered capture the config-owned BPF
// expression and filter function are re-applied instead of deleted.
func TestClearFilterRestoresOwned(t *testing.T) {
	r := newRig(t)
	m := r.manager(t, true, nil)
	m.c.OwnedFilter = func(context.Context) (string, string) { return "tcp port 179", "bpf_trace_filter" }
	if _, _, err := run(context.Background(), t, m, &vrxv1.CaptureAction{Interface: "loop501", Seconds: 1, Bpf: "icmp"}); err != nil {
		t.Fatal(err)
	}
	if filter, fn := r.filters(); filter != "tcp port 179" || fn != "bpf_trace_filter" {
		t.Fatalf("owned filter not restored: %q %q", filter, fn)
	}
	// a non-owner never touches the globals (ErrGlobals before any VPP call)
	m2 := r.manager(t, false, nil)
	if _, _, err := run(context.Background(), t, m2, &vrxv1.CaptureAction{Interface: "loop501", Bpf: "icmp"}); !errors.Is(err, ErrGlobals) {
		t.Fatal(err)
	}
}

func TestRetentionKeepsNewestOversizeFile(t *testing.T) {
	r := newRig(t)
	m := r.manager(t, false, nil)
	m.c.MaxBytes = 1
	now := time.Now()
	for i, id := range []string{"w5-new", "w5-old"} {
		rec := &Record{ID: id, State: "done", StartedAt: now.Add(-time.Duration(i) * time.Hour), Size: 100}
		if err := m.save(rec); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(m.path(id, ".pcap"), pcapFile(1), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m.retain()
	if _, err := os.Stat(m.path("w5-new", ".pcap")); err != nil {
		t.Fatal("newest capture lost", err)
	}
	if _, err := os.Stat(m.path("w5-old", ".pcap")); !os.IsNotExist(err) {
		t.Fatal("old capture retained", err)
	}
}

func TestOversizePlanRefusedBeforeVPP(t *testing.T) {
	r := newRig(t)
	m := r.manager(t, false, nil)
	m.c.MaxBytes = 100
	_, _, err := run(context.Background(), t, m, &vrxv1.CaptureAction{Interface: "any", MaxPackets: 2, Snaplen: 32, Seconds: 1})
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "maxPackets") {
		t.Fatal(err)
	}
	if r.vppRunning() {
		t.Fatal("oversize plan started VPP capture")
	}
	if records, _ := m.load(); len(records) != 0 {
		t.Fatal("oversize plan wrote records")
	}
}

func TestFIFORefusedWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.pcap")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		f, err := openVPPFile(path)
		if f != nil {
			_ = f.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrForeignFile) || strings.Contains(err.Error(), filepath.Dir(path)) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("FIFO open blocked")
	}
}

func TestRecoveryContinuesAfterForeignLeftover(t *testing.T) {
	r := newRig(t)
	m := r.manager(t, false, nil)
	for _, id := range []string{"w5-foreign", "w5-valid"} {
		if err := m.save(&Record{ID: id, State: "running", StartedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("foreign"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(r.vppDir, "w5-foreign.pcap")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.vppDir, "w5-valid.pcap"), pcapFile(1), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	bad, _ := m.get("w5-foreign")
	good, _ := m.get("w5-valid")
	if bad.State != "error" || good.State != "interrupted" || good.Packets != 1 || !m.recovered {
		t.Fatalf("bad=%+v good=%+v", bad, good)
	}
	if b, err := os.ReadFile(target); err != nil || string(b) != "foreign" {
		t.Fatal("foreign target modified")
	}
}

func TestRecoveryDoesNotHideForeignRecordSaveFailure(t *testing.T) {
	r := newRig(t)
	m := r.manager(t, false, nil)
	id := "w5-save-failure"
	if err := m.save(&Record{ID: id, State: "running", StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("foreign"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(r.vppDir, id+".pcap")); err != nil {
		t.Fatal(err)
	}
	m.c.Now = func() time.Time {
		_ = os.Remove(m.path(id, ".json"))
		_ = os.Mkdir(m.path(id, ".json"), 0700)
		return time.Now()
	}
	if err := m.Recover(context.Background()); err == nil || m.recovered {
		t.Fatal("record save failure was hidden", err)
	}
}

func TestStopFailureRemainsRecoverable(t *testing.T) {
	r := newRig(t)
	m := r.manager(t, false, nil)
	calls := 0
	r.f.On("pcap_trace_off", func(api.Message) ([]api.Message, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("temporary stop failure")
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		r.running = false
		if err := os.WriteFile(filepath.Join(r.vppDir, r.file), pcapFile(3), 0600); err != nil {
			return nil, err
		}
		return []api.Message{&interfaces.PcapTraceOffReply{}}, nil
	})
	stop := background(t, m, &vrxv1.CaptureAction{Interface: "any", Seconds: 60}, false)
	if err := stop(); err == nil {
		t.Fatal("failed stop reported success")
	}
	records, _ := m.load()
	if len(records) != 1 || records[0].State != "running" || m.running == nil || m.recovered {
		t.Fatal("failed stop lost recovery ownership")
	}
	if err := m.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	rec, _ := m.get(records[0].ID)
	if rec.State != "interrupted" || r.vppRunning() || m.running != nil {
		t.Fatal("retry did not repair stop", rec)
	}
}

func TestFilterRestoreFailureRetriesAfterManagerRestart(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(map[bool]string{false: "same-boot", true: "new-boot"}[restart], func(t *testing.T) {
			r := newRig(t)
			boot := dfkit.NewMemoryBootStore()
			m := r.manager(t, true, boot)
			failed := false
			r.f.On("pcap_set_filter_function", func(msg api.Message) ([]api.Message, error) {
				name := msg.(*interfaces.PcapSetFilterFunction).FilterFunctionName
				if name == "vnet_is_packet_traced" && !failed {
					failed = true
					return nil, errors.New("temporary filter failure")
				}
				r.mu.Lock()
				r.fn = name
				r.mu.Unlock()
				return []api.Message{&interfaces.PcapSetFilterFunctionReply{}}, nil
			})
			stop := background(t, m, &vrxv1.CaptureAction{Interface: "any", Bpf: "icmp", Seconds: 60}, false)
			if err := stop(); err == nil {
				t.Fatal("failed restoration reported success")
			}
			records, _ := m.load()
			if len(records) != 1 || records[0].State != "running" {
				t.Fatal("restoration lost recovery record")
			}
			if restart {
				r.f.RestartVPP()
				r.mu.Lock()
				r.filter, r.fn = "new-owner-filter", "new-owner-function"
				r.mu.Unlock()
			}
			m2 := r.manager(t, true, boot)
			if err := m2.Recover(context.Background()); err != nil {
				t.Fatal(err)
			}
			rec, _ := m2.get(records[0].ID)
			bf, fn := r.filters()
			wantBPF, wantFunction := "", "vnet_is_packet_traced"
			if restart {
				wantBPF, wantFunction = "new-owner-filter", "new-owner-function"
			}
			if rec.State != "interrupted" || bf != wantBPF || fn != wantFunction {
				t.Fatalf("restore failed rec=%+v bpf=%q fn=%q", rec, bf, fn)
			}
			if _, ok := boot.Get("pcap.capture-filter-restore"); ok {
				t.Fatal("pending filter marker retained after success")
			}

		})
	}
}

func TestRecoveryAfterFileMoveBeforeMetadataCommit(t *testing.T) {
	r := newRig(t)
	m := r.manager(t, false, nil)
	id := "w5-moved"
	if err := m.save(&Record{ID: id, State: "running", StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.path(id, ".pcap"), pcapFile(2), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	rec, _ := m.get(id)
	if rec.State != "interrupted" || rec.Packets != 2 || rec.Size == 0 || rec.Sha256 == "" {
		t.Fatalf("lost already-kept file: %+v", rec)
	}
}

// A crash can leave only a kept path and a running record. Refusing that path
// must persist a terminal state; an unsuccessful metadata write stays retryable.
func TestRecoveryRefusesForeignKeptFile(t *testing.T) {
	for _, failSave := range []bool{false, true} {
		t.Run(fmt.Sprint("saveFailure=", failSave), func(t *testing.T) {
			r := newRig(t)
			m := r.manager(t, false, nil)
			id := "w5-foreign-kept"
			record := &Record{ID: id, State: "running", StartedAt: time.Now()}
			if err := m.save(record); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(t.TempDir(), "private-target")
			original := []byte("never serve or modify this target")
			if err := os.WriteFile(target, original, 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, m.path(id, ".pcap")); err != nil {
				t.Fatal(err)
			}
			if failSave {
				m.c.Now = func() time.Time {
					_ = os.Remove(m.path(id, ".json"))
					_ = os.Mkdir(m.path(id, ".json"), 0700)
					return time.Now()
				}
			}
			err := m.Recover(context.Background())
			if failSave {
				if err == nil || m.recovered {
					t.Fatal("failed metadata save was hidden", err)
				}
				if err := os.Remove(m.path(id, ".json")); err != nil {
					t.Fatal(err)
				}
				m.c.Now = time.Now
				if err := m.save(record); err != nil {
					t.Fatal(err)
				}
				if err := m.Recover(context.Background()); err != nil {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			got, err := m.get(id)
			if err != nil || got.State != "error" || !m.recovered {
				t.Fatalf("refused kept file did not reach terminal state: %+v %v", got, err)
			}
			data, err := os.ReadFile(target)
			if err != nil || string(data) != string(original) {
				t.Fatal("target changed", err)
			}
			info, err := os.Stat(target)
			if err != nil || info.Mode().Perm() != 0644 {
				t.Fatal("target permissions changed", err)
			}
			called := false
			if err := m.Read(id, func(*vrxv1.CaptureChunk) error { called = true; return nil }); err == nil || called {
				t.Fatal("refused kept file was served")
			}
		})
	}
}
