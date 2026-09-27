// Package capturetrace is the F-capture-trace action (WBS D8.2): a pcap capture through VPP's
// built-in dispatch capture (DF-8 descriptors pcap.capture, pcap.filter-function and
// trace.bpf-filter), driven by the Action RPC — not a config domain. The agent keeps the files:
// VPP writes /tmp/<owner>-….pcap world-readable (0664); after pcap_trace_off the file is moved to
// the capture directory (default /var/lib/vrx/captures), chmod 0600, hashed and counted, and a
// retention policy (count and bytes caps, oldest first) is applied.
//
// One capture per VPP: a second one is ErrBusy (this agent's own, or another owner's —
// pcap.ErrCaptureBusy). Another owner's capture is never stopped (pcap.capture's Delete only
// stops what this owner started on the running VPP process, D-076).
//
// Packet trace (tracedump) and packet-generator streams are not built: D-128/TD-20 bans trace on
// the shared VPP, the tracedump binapi does not exist here (V18) and VPP has no binary API for a PG
// stream. List reports both as unavailable with the reason (never fake data).
package capturetrace

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/pcap"
	"ngfw/agent/internal/descriptors/trace"
	"ngfw/agent/internal/vpp"
)

// Typed errors (mapped onto gRPC codes by internal/agent/rpc_capture_trace.go).
var (
	ErrInvalid  = errors.New("invalid capture request")                                     // INVALID_ARGUMENT
	ErrBusy     = errors.New("busy: a pcap capture is already running on this VPP")         // ABORTED (API: 409)
	ErrGlobals  = errors.New("a BPF filter needs the globals-owner agent (D-071)")          // FAILED_PRECONDITION
	ErrNotFound = errors.New("no such capture")                                             // NOT_FOUND
	ErrRunning  = errors.New("the capture is running: cancel its action stream to stop it") // FAILED_PRECONDITION
)

// Reasons shown for trace / PG (CaptureListResponse).
const (
	TraceReason = "packet trace is not available on this build: trace is banned on the shared VPP (D-128/TD-20, " +
		"docs/lab/shared-host-rules.md §11) and the tracedump/tracenode plugins and binapi are not built (V18)"
	PGReason = "packet-generator streams cannot be defined through the VPP binary API (no stream message; only " +
		"cli_inband, which is not allowed)"
)

// Limits (CaptureAction).
const (
	DefaultMaxPackets = 1000
	MaxMaxPackets     = 100000
	DefaultSeconds    = 30
	MaxSeconds        = 600
	DefaultSnaplen    = 9000
	MinSnaplen        = 32
	MaxSnaplen        = 9000
	DefaultMaxFiles   = 10
	DefaultMaxBytes   = 500 << 20
	chunkSize         = 64 << 10
)

// Plan is a validated CaptureAction.
type Plan struct {
	Interface   string
	Rx, Tx      bool
	Drop        bool
	BPF         string
	ErrorFilter string
	MaxPackets  uint32
	Seconds     uint32
	Snaplen     uint32
}

func invalid(field, format string, a ...any) error {
	return fmt.Errorf("%w: %s: %s", ErrInvalid, field, fmt.Sprintf(format, a...))
}

func nameOK(v string, extra string) bool {
	for _, r := range v {
		ok := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '-' || r == '_' || r == '.'
		if !ok && !strings.ContainsRune(extra, r) {
			return false
		}
	}
	return true
}

// Validate checks a CaptureAction and fills the defaults. Error text starts with the field name
// ("bpf: …"), which the API turns into a problem pointer.
func Validate(a *vrxv1.CaptureAction) (Plan, error) {
	p := Plan{Interface: a.GetInterface(), BPF: strings.TrimSpace(a.GetBpf()), ErrorFilter: a.GetErrorFilter(),
		MaxPackets: a.GetMaxPackets(), Seconds: a.GetSeconds(), Snaplen: a.GetSnaplen(), Drop: a.GetDrop()}
	switch {
	case p.Interface == "":
		return p, invalid("interface", "required (%q = every interface)", pcap.AnyInterface)
	case len(p.Interface) > 63 || !nameOK(p.Interface, "/:"):
		return p, invalid("interface", "%q is not an interface name", p.Interface)
	}
	switch a.GetDirection() {
	case vrxv1.CaptureDirection_CAPTURE_DIRECTION_RX:
		p.Rx = true
	case vrxv1.CaptureDirection_CAPTURE_DIRECTION_TX:
		p.Tx = true
	case vrxv1.CaptureDirection_CAPTURE_DIRECTION_BOTH:
		p.Rx, p.Tx = true, true
	case vrxv1.CaptureDirection_CAPTURE_DIRECTION_UNSPECIFIED:
		p.Rx, p.Tx = !p.Drop, !p.Drop // drop alone = only drops
	default:
		return p, invalid("direction", "unknown value %d", a.GetDirection())
	}
	if p.MaxPackets == 0 {
		p.MaxPackets = DefaultMaxPackets
	}
	if p.MaxPackets > MaxMaxPackets {
		return p, invalid("maxPackets", "%d above %d", p.MaxPackets, MaxMaxPackets)
	}
	if p.Seconds == 0 {
		p.Seconds = DefaultSeconds
	}
	if p.Seconds > MaxSeconds {
		return p, invalid("seconds", "%d above %d", p.Seconds, MaxSeconds)
	}
	if p.Snaplen == 0 {
		p.Snaplen = DefaultSnaplen
	}
	if p.Snaplen < MinSnaplen || p.Snaplen > MaxSnaplen {
		return p, invalid("snaplen", "%d outside %d..%d", p.Snaplen, MinSnaplen, MaxSnaplen)
	}
	if p.BPF != "" {
		if err := (trace.BPFFilter{Expression: p.BPF}).Validate(); err != nil {
			return p, invalid("bpf", "%s", strings.TrimPrefix(err.Error(), dfkit.ErrSpec.Error()+": "))
		}
	}
	if p.ErrorFilter != "" {
		if !p.Drop {
			return p, invalid("errorFilter", "needs the drop capture")
		}
		if len(p.ErrorFilter) > 127 || !nameOK(p.ErrorFilter, "/") || strings.Count(p.ErrorFilter, "/") != 1 {
			return p, invalid("errorFilter", "%q is not \"node/error\"", p.ErrorFilter)
		}
	}
	return p, nil
}

// Direction renders rx/tx/drop as stored in the record.
func (p Plan) Direction() string {
	var d []string
	if p.Rx {
		d = append(d, "rx")
	}
	if p.Tx {
		d = append(d, "tx")
	}
	if p.Drop {
		d = append(d, "drop")
	}
	return strings.Join(d, ",")
}

// Config configures a Manager.
type Config struct {
	Client       vpp.Client
	Owner        string
	GlobalsOwner bool
	// Dir keeps the files and their records (0700). Default /var/lib/vrx/captures.
	Dir string
	// VPPDir is where VPP writes capture files (pcap.FileDir; tests: a temp dir).
	VPPDir string
	// Boot is the persisted D-076 store of the running capture (default Dir/.boot.json).
	Boot     dfkit.BootStore
	MaxFiles int
	MaxBytes int64
	Now      func() time.Time
	// Tick is the progress-line interval (default 1 s).
	Tick time.Duration
}

// Record is the stored metadata of one capture (Dir/<id>.json).
type Record struct {
	ID         string    `json:"id"`
	State      string    `json:"state"`
	Interface  string    `json:"interface"`
	Direction  string    `json:"direction"`
	BPF        string    `json:"bpf,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	StoppedAt  time.Time `json:"stopped_at,omitzero"`
	Size       uint64    `json:"size"`
	Packets    uint64    `json:"packets"`
	Sha256     string    `json:"sha256,omitempty"`
	MaxPackets uint32    `json:"max_packets"`
	Seconds    uint32    `json:"seconds"`
	Snaplen    uint32    `json:"snaplen"`
	Reason     string    `json:"reason,omitempty"`
}

// Manager runs captures and keeps their files.
type Manager struct {
	c       Config
	capture *pcap.CaptureDescriptor

	mu        sync.Mutex
	running   *Record
	seq       int
	recovered bool
}

// New returns a Manager; it creates Dir (0700).
func New(c Config) (*Manager, error) {
	if c.Dir == "" {
		c.Dir = "/var/lib/vrx/captures"
	}
	if c.VPPDir == "" {
		c.VPPDir = pcap.FileDir
	}
	if c.MaxFiles <= 0 {
		c.MaxFiles = DefaultMaxFiles
	}
	if c.MaxBytes <= 0 {
		c.MaxBytes = DefaultMaxBytes
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.Tick <= 0 {
		c.Tick = time.Second
	}
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("capture dir: %w", err)
	}
	if c.Boot == nil {
		b, err := dfkit.NewFileBootStore(filepath.Join(c.Dir, ".boot-"+c.Owner+".json"))
		if err != nil {
			return nil, err
		}
		c.Boot = b
	}
	return &Manager{c: c, capture: pcap.NewCapture(c.Client, c.Owner, c.Boot)}, nil
}

func (m *Manager) path(id, ext string) string { return filepath.Join(m.c.Dir, id+ext) }

func validID(id string) bool {
	return id != "" && len(id) <= 100 && !strings.HasPrefix(id, ".") && nameOK(id, "")
}

func (m *Manager) save(r *Record) error {
	b, _ := json.MarshalIndent(r, "", "  ")
	tmp := m.path(r.ID, ".json.tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.path(r.ID, ".json"))
}

func (m *Manager) load() ([]*Record, error) {
	ents, err := os.ReadDir(m.c.Dir)
	if err != nil {
		return nil, err
	}
	var out []*Record
	for _, e := range ents {
		n := e.Name()
		if strings.HasPrefix(n, ".") || !strings.HasSuffix(n, ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(m.c.Dir, n)) //nolint:gosec // the agent's own directory
		if err != nil {
			continue
		}
		var r Record
		if json.Unmarshal(b, &r) != nil || r.ID+".json" != n {
			continue
		}
		out = append(out, &r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out, nil
}

func (m *Manager) captureSpec(p Plan, file string) pcap.Capture {
	return pcap.Capture{Rx: p.Rx, Tx: p.Tx, Drop: p.Drop, Interface: p.Interface, MaxPackets: p.MaxPackets,
		MaxBytesPerPacket: p.Snaplen, Filter: p.BPF != "", Error: p.ErrorFilter, File: file}
}

func (m *Manager) setFilter(ctx context.Context, expr string) error {
	g := dfkit.GlobalsOwner(true)
	if _, err := trace.NewBPFFilter(m.c.Client, g).Create(ctx, trace.BPFFilter{Expression: expr}.Proto()); err != nil {
		return fmt.Errorf("%w: bpf: %v", ErrInvalid, err)
	}
	ff := pcap.NewFilterFunction(m.c.Client, pcap.WithGlobals(g))
	_, err := ff.Create(ctx, pcap.FilterFunction{Name: trace.Plugin}.Proto())
	return err
}

func (m *Manager) clearFilter(ctx context.Context) error {
	g := dfkit.GlobalsOwner(true)
	err := pcap.NewFilterFunction(m.c.Client, pcap.WithGlobals(g)).Delete(ctx, nil, nil)
	return errors.Join(err, trace.NewBPFFilter(m.c.Client, g).Delete(ctx, nil, nil))
}

// Run starts one capture, streams progress lines, stops it on timeout or ctx cancellation, keeps
// the file and ends with a done. The first line is "capture <id> started".
func (m *Manager) Run(ctx context.Context, p Plan, send func(*vrxv1.ActionOutput) error) error {
	if err := m.Recover(ctx); err != nil {
		return err
	}
	if p.BPF != "" && !m.c.GlobalsOwner {
		return ErrGlobals
	}
	m.mu.Lock()
	if m.running != nil {
		m.mu.Unlock()
		return fmt.Errorf("%w (capture %s)", ErrBusy, m.running.ID)
	}
	now := m.c.Now().UTC()
	m.seq++
	r := &Record{ID: fmt.Sprintf("%s-%s-%d", m.c.Owner, now.Format("20060102T150405"), m.seq), State: "running",
		Interface: p.Interface, Direction: p.Direction(), BPF: p.BPF, StartedAt: now,
		MaxPackets: p.MaxPackets, Seconds: p.Seconds, Snaplen: p.Snaplen}
	m.running = r
	m.mu.Unlock()
	release := func() {
		m.mu.Lock()
		m.running = nil
		m.mu.Unlock()
	}
	if len(r.ID)+len(".pcap") > 63 {
		release()
		return invalid("interface", "owner %q too long for a VPP capture file name", m.c.Owner)
	}
	if err := m.save(r); err != nil {
		release()
		return err
	}
	fail := func(err error) error {
		_ = os.Remove(m.path(r.ID, ".json"))
		release()
		return err
	}
	if p.BPF != "" {
		if err := m.setFilter(ctx, p.BPF); err != nil {
			_ = m.clearFilter(context.WithoutCancel(ctx))
			return fail(err)
		}
	}
	if _, err := m.capture.Create(ctx, m.captureSpec(p, r.ID+".pcap").Proto()); err != nil {
		if p.BPF != "" {
			_ = m.clearFilter(context.WithoutCancel(ctx))
		}
		switch {
		case errors.Is(err, pcap.ErrCaptureBusy):
			err = fmt.Errorf("%w (another owner's capture: %v)", ErrBusy, err)
		case errors.Is(err, dfkit.ErrSpec), errors.Is(err, dfkit.ErrNotOwned):
			err = fmt.Errorf("%w: interface: %v", ErrInvalid, err)
		}
		return fail(err)
	}

	reason := "timeout"
	if err := send(&vrxv1.ActionOutput{Output: &vrxv1.ActionOutput_Line{Line: "capture " + r.ID + " started"}}); err != nil {
		reason = "cancelled"
	}
	timer := time.NewTimer(time.Duration(p.Seconds) * time.Second)
	tick := time.NewTicker(m.c.Tick)
	for reason == "timeout" {
		select {
		case <-ctx.Done():
			reason = "cancelled"
		case <-tick.C:
			line := fmt.Sprintf("capturing %s: %ds / %ds", r.ID, int(m.c.Now().Sub(r.StartedAt).Seconds()), p.Seconds)
			if send(&vrxv1.ActionOutput{Output: &vrxv1.ActionOutput_Line{Line: line}}) != nil {
				reason = "cancelled"
			}
			continue
		case <-timer.C:
		}
		break
	}
	timer.Stop()
	tick.Stop()

	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	stopErr := m.capture.Delete(stopCtx, nil, nil)
	if p.BPF != "" {
		stopErr = errors.Join(stopErr, m.clearFilter(stopCtx))
	}
	finErr := m.finish(r, reason)
	release()
	m.retain()
	if err := errors.Join(stopErr, finErr); err != nil {
		return err
	}
	code := int32(0)
	if reason == "cancelled" {
		code = 1
	}
	_ = send(&vrxv1.ActionOutput{Output: &vrxv1.ActionOutput_Done{Done: &vrxv1.ActionDone{
		Summary:  fmt.Sprintf("capture %s: %d packets, %d bytes (%s)", r.ID, r.Packets, r.Size, reason),
		ExitCode: code,
		Stats: map[string]string{"id": r.ID, "packets": fmt.Sprint(r.Packets), "bytes": fmt.Sprint(r.Size),
			"sha256": r.Sha256, "state": r.State, "reason": reason},
	}}})
	return nil
}

// finish moves VPP's file into Dir (0600), counts and hashes it and saves the record.
func (m *Manager) finish(r *Record, reason string) error {
	r.StoppedAt = m.c.Now().UTC()
	r.Reason = reason
	r.State = "done"
	if reason == "agent-restart" || reason == "vpp-restart" {
		r.State = "interrupted"
	}
	src := filepath.Join(m.c.VPPDir, r.ID+".pcap")
	dst := m.path(r.ID, ".pcap")
	if err := moveFile(src, dst); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return errors.Join(err, m.save(r))
		}
		if r.State == "done" {
			r.State = "empty"
		}
		return m.save(r)
	}
	size, pkts, sum, err := inspect(dst)
	if err != nil {
		return errors.Join(err, m.save(r))
	}
	r.Size, r.Packets, r.Sha256 = size, pkts, sum
	return m.save(r)
}

// moveFile renames src to dst (copy + remove across file systems) and leaves dst at 0600.
func moveFile(src, dst string) error {
	if _, err := os.Lstat(src); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err != nil {
		in, err := os.Open(src) //nolint:gosec // VPP's capture file, name built by the agent
		if err != nil {
			return err
		}
		defer func() { _ = in.Close() }()
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600) //nolint:gosec // agent dir
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			_ = out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		if err := os.Remove(src); err != nil {
			return err
		}
	}
	return os.Chmod(dst, 0o600)
}

// inspect returns size, pcap record count and hex sha256 of a pcap file.
func inspect(path string) (uint64, uint64, string, error) {
	b, err := os.ReadFile(path) //nolint:gosec // the agent's own file
	if err != nil {
		return 0, 0, "", err
	}
	sum := sha256.Sum256(b)
	return uint64(len(b)), countPackets(b), hex.EncodeToString(sum[:]), nil
}

// countPackets walks the classic pcap records (either byte order); 0 for anything else.
func countPackets(b []byte) uint64 {
	if len(b) < 24 {
		return 0
	}
	var bo binary.ByteOrder
	switch binary.LittleEndian.Uint32(b) {
	case 0xa1b2c3d4, 0xa1b23c4d:
		bo = binary.LittleEndian
	case 0xd4c3b2a1, 0x4d3cb2a1:
		bo = binary.BigEndian
	default:
		return 0
	}
	var n uint64
	for off := 24; off+16 <= len(b); n++ {
		incl := int(bo.Uint32(b[off+8:]))
		if incl < 0 || off+16+incl > len(b) {
			break
		}
		off += 16 + incl
	}
	return n
}

// retain removes the oldest kept files beyond MaxFiles / MaxBytes (never the running capture).
func (m *Manager) retain() {
	recs, err := m.load()
	if err != nil {
		return
	}
	var files int
	var bytes int64
	for _, r := range recs { // newest first
		if r.State == "running" {
			continue
		}
		files++
		bytes += int64(r.Size) //nolint:gosec // file sizes
		if files > m.c.MaxFiles || bytes > m.c.MaxBytes {
			_ = os.Remove(m.path(r.ID, ".pcap"))
			_ = os.Remove(m.path(r.ID, ".json"))
		}
	}
}

// Recover runs once per process: a record still "running" was left by a previous agent process
// (agent restart during a capture). Its capture is stopped when it is still this owner's on the
// running VPP (pcap.capture's D-076 record), else VPP restarted; either way the file VPP wrote is
// kept and the record becomes "interrupted".
func (m *Manager) Recover(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recovered {
		return nil
	}
	recs, err := m.load()
	if err != nil {
		return err
	}
	for _, r := range recs {
		if r.State != "running" {
			continue
		}
		started, err := dfkit.StartedThisBoot(ctx, m.c.Client, m.c.Boot, pcap.KeyCapture)
		if err != nil {
			return err
		}
		reason := "vpp-restart"
		if started {
			reason = "agent-restart"
		}
		if err := m.capture.Delete(ctx, nil, nil); err != nil {
			return err
		}
		if r.BPF != "" && m.c.GlobalsOwner && started {
			_ = m.clearFilter(ctx)
		}
		if err := m.finish(r, reason); err != nil {
			return err
		}
	}
	m.recovered = true
	return nil
}

func (r *Record) proto() *vrxv1.CaptureFile {
	f := &vrxv1.CaptureFile{Id: r.ID, State: r.State, Interface: r.Interface, Direction: r.Direction, Bpf: r.BPF,
		StartedAt: timestamppb.New(r.StartedAt), Size: r.Size, Packets: r.Packets, Sha256: r.Sha256,
		MaxPackets: r.MaxPackets, Seconds: r.Seconds, Snaplen: r.Snaplen, Reason: r.Reason}
	if !r.StoppedAt.IsZero() {
		f.StoppedAt = timestamppb.New(r.StoppedAt)
	}
	return f
}

// List returns the running capture (first) and the kept files, newest first.
func (m *Manager) List(ctx context.Context) (*vrxv1.CaptureListResponse, error) {
	if err := m.Recover(ctx); err != nil {
		return nil, err
	}
	recs, err := m.load()
	if err != nil {
		return nil, err
	}
	out := &vrxv1.CaptureListResponse{MaxFiles: uint32(m.c.MaxFiles), MaxBytes: uint64(m.c.MaxBytes), //nolint:gosec // positive caps
		TraceReason: TraceReason, PgReason: PGReason}
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].State == "running" && recs[j].State != "running" })
	for _, r := range recs {
		out.Captures = append(out.Captures, r.proto())
	}
	return out, nil
}

func (m *Manager) get(id string) (*Record, error) {
	if !validID(id) {
		return nil, fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	b, err := os.ReadFile(m.path(id, ".json"))
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	var r Record
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// Read streams a kept file in chunks.
func (m *Manager) Read(id string, send func(*vrxv1.CaptureChunk) error) error {
	r, err := m.get(id)
	if err != nil {
		return err
	}
	if r.State == "running" {
		return ErrRunning
	}
	f, err := os.Open(m.path(id, ".pcap"))
	if err != nil {
		return fmt.Errorf("%w: %s has no file (%s)", ErrNotFound, id, r.State)
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, chunkSize)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			if serr := send(&vrxv1.CaptureChunk{Data: append([]byte(nil), buf[:n]...)}); serr != nil {
				return serr
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// Delete removes a kept file and its record; returns the bytes freed.
func (m *Manager) Delete(id string) (uint64, error) {
	r, err := m.get(id)
	if err != nil {
		return 0, err
	}
	if r.State == "running" {
		return 0, ErrRunning
	}
	if err := os.Remove(m.path(id, ".pcap")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	return r.Size, os.Remove(m.path(id, ".json"))
}
