package subsystems

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.fd.io/govpp/api"
	"golang.org/x/sys/unix"
	classifyapi "ngfw/agent/binapi/classify"
	interfaceapi "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	ipapi "ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/memclnt"
	tapapi "ngfw/agent/binapi/tapv2"
	"ngfw/agent/internal/ownertable"
	ravpn "ngfw/agent/internal/ra_vpn"
	"ngfw/agent/internal/renderers/strongswan"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
)

// This supplemental fault campaign uses the genuine canonical daemon and VPP
// in a separately provisioned PID1 guest. Only the fixed manager Stop result is
// injected. It does not prove an actual production systemd Stop failure or the
// correctness of credential recovery. Canonical RPC baseline rollback remains
// the external coordinator's responsibility after this test resumes the agent.
type actualStopManifest struct {
	Agent                bootid.Identity    `json:"agent"`
	VPP                  bootid.Identity    `json:"vpp"`
	AgentExeDev          uint64             `json:"agentExeDev"`
	AgentExeIno          uint64             `json:"agentExeIno"`
	AgentCgroup          string             `json:"agentCgroup"`
	Record               ravpn.EngineRecord `json:"record"`
	ForeignInterface     string             `json:"foreignInterface"`
	ForeignRouteTable    uint32             `json:"foreignRouteTable"`
	ForeignRoutePrefix   string             `json:"foreignRoutePrefix"`
	ForeignClassifyTable uint32             `json:"foreignClassifyTable"`
}

func actualStopReadPrivate(path string, limit int64) ([]byte, error) {
	// #nosec G304 -- integration callers use fixed guest metadata paths; portable tests use only their private TempDir.
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ravpn.ErrEngine
	}
	file := os.NewFile(uintptr(fd), "held public guest metadata")
	var info unix.Stat_t
	valid := unix.Fstat(fd, &info) == nil && info.Mode == unix.S_IFREG|0600 && info.Uid == 0 && info.Gid == 0 && info.Nlink == 1 && info.Size > 0 && info.Size <= limit
	data, readErr := io.ReadAll(io.LimitReader(file, limit+1))
	closeErr := file.Close()
	if !valid || readErr != nil || closeErr != nil || int64(len(data)) > limit {
		return nil, ravpn.ErrEngine
	}
	return data, nil
}

func actualStopDecode(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil || decoder.Decode(new(any)) != io.EOF {
		return ravpn.ErrEngine
	}
	return nil
}

func actualStopGuest() error {
	data, err := actualStopReadPrivate("/run/ngfw-ra-guest-fixture", 1024)
	if err != nil {
		return err
	}
	var marker struct {
		Owner  string `json:"owner"`
		BootID string `json:"bootId"`
	}
	if actualStopDecode(data, &marker) != nil || marker.Owner != "ngfw-ra-independent-guest" || marker.BootID != (bootid.Reader{}).BootID() {
		return ravpn.ErrEngine
	}
	manager, err := os.Readlink("/proc/1/exe")
	if err != nil || (manager != "/usr/lib/systemd/systemd" && manager != "/lib/systemd/systemd") || os.Geteuid() != 0 {
		return ravpn.ErrEngine
	}
	return nil
}

func actualStopValidateManifest(m actualStopManifest) error {
	if !m.Agent.Complete() || m.Agent.PID <= 1 || !m.VPP.Complete() || m.VPP.PID <= 1 || m.Agent == m.VPP || m.AgentExeDev == 0 || m.AgentExeIno == 0 || m.AgentCgroup != "/system.slice/ngfw-agent.service" || !m.Record.Ready || m.Record.Spec.Validate() != nil || m.Record.Spec.Owner != "ngfw" || !m.Record.Unit.Valid() || m.Record.Unit.BootID != m.Agent.BootID || m.VPP.BootID != m.Agent.BootID || m.Record.Unit.PID == m.Agent.PID || m.Record.Unit.PID == m.VPP.PID || !strings.HasPrefix(m.ForeignInterface, "loop") || len(m.ForeignInterface) > 32 {
		return ravpn.ErrEngine
	}
	for _, c := range strings.TrimPrefix(m.ForeignInterface, "loop") {
		if c < '0' || c > '9' {
			return ravpn.ErrEngine
		}
	}
	prefix, prefixErr := netip.ParsePrefix(m.ForeignRoutePrefix)
	if prefixErr != nil || prefix.Masked().String() != m.ForeignRoutePrefix || m.ForeignClassifyTable == 0 {
		return ravpn.ErrEngine
	}
	if m.ForeignInterface == "loop" {
		return ravpn.ErrEngine
	}
	return nil
}

func actualStopVerifyAgent(m actualStopManifest) error {
	if !(bootid.Reader{}).ForPID(m.Agent.PID).Equal(m.Agent) {
		return ravpn.ErrEngine
	}
	// #nosec G304 -- PID is from the bounded protected manifest and complete birth identity was positively matched above; only kernel identity paths are read.
	group, err := os.ReadFile("/proc/" + strconv.Itoa(m.Agent.PID) + "/cgroup")
	if err != nil || string(group) != "0::"+m.AgentCgroup+"\n" {
		return ravpn.ErrEngine
	}
	// #nosec G304 -- held actual image of the fully verified canonical guest agent, never a caller-supplied path.
	actual, err := os.Open("/proc/" + strconv.Itoa(m.Agent.PID) + "/exe")
	if err != nil {
		return ravpn.ErrEngine
	}
	got, statErr := actual.Stat()
	closeErr := actual.Close()
	installed, installedErr := os.Stat("/usr/sbin/ngfw-agent")
	if statErr != nil || closeErr != nil || installedErr != nil || !os.SameFile(got, installed) {
		return ravpn.ErrEngine
	}
	var image unix.Stat_t
	fd, err := unix.Open("/usr/sbin/ngfw-agent", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return ravpn.ErrEngine
	}
	valid := unix.Fstat(fd, &image) == nil && image.Uid == 0 && image.Mode&0022 == 0 && image.Nlink == 1 && uint64(image.Dev) == m.AgentExeDev && image.Ino == m.AgentExeIno
	if unix.Close(fd) != nil || !valid || !(bootid.Reader{}).ForPID(m.Agent.PID).Equal(m.Agent) {
		return ravpn.ErrEngine
	}
	sum := sha256.Sum256([]byte(m.Agent.String()))
	generation := "source-agent-" + hex.EncodeToString(sum[:])
	link, err := os.Readlink("/run/ngfw/ra/source-agent-current")
	if err != nil || link != generation {
		return ravpn.ErrEngine
	}
	// The immutable generation name is derived from the complete identity. No
	// arbitrary metadata path or source boolean substitutes for the live image.
	data, err := actualStopReadPrivate(filepath.Join("/run/ngfw/ra", generation, "identity.json"), 1024)
	if err != nil {
		return err
	}
	var source struct {
		Source  bootid.Identity
		Version int
		Owner   string
	}
	if actualStopDecode(data, &source) != nil || source.Source != m.Agent || source.Version != 1 || source.Owner != "ngfw-ra-source" {
		return ravpn.ErrEngine
	}
	return nil
}

func actualStopAgentState(identity bootid.Identity) (byte, error) {
	if !(bootid.Reader{}).ForPID(identity.PID).Equal(identity) {
		return 0, ravpn.ErrEngine
	}
	// #nosec G304 -- complete verified birth identity determines this fixed kernel stat path; no supplied path.
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(identity.PID) + "/stat")
	end := strings.LastIndex(string(raw), ") ")
	if err != nil || end < 0 || len(raw) < end+4 || !(bootid.Reader{}).ForPID(identity.PID).Equal(identity) {
		return 0, ravpn.ErrEngine
	}
	return raw[end+2], nil
}

func actualStopPause(m actualStopManifest) (func() error, error) {
	if actualStopGuest() != nil || actualStopVerifyAgent(m) != nil {
		return nil, ravpn.ErrEngine
	}
	fd, err := unix.PidfdOpen(m.Agent.PID, 0)
	if err != nil {
		return nil, ravpn.ErrEngine
	}
	paused := false
	resume := func() error {
		var signalErr error
		if paused {
			signalErr = unix.PidfdSendSignal(fd, unix.SIGCONT, nil, 0)
		}
		closeErr := unix.Close(fd)
		if signalErr != nil || closeErr != nil {
			return errors.New("owned guest agent resume/handle close failed")
		}
		return nil
	}
	state, stateErr := actualStopAgentState(m.Agent)
	if actualStopVerifyAgent(m) != nil || stateErr != nil || (state != 'S' && state != 'R' && state != 'I') {
		if err := resume(); err != nil {
			return nil, err
		}
		return nil, ravpn.ErrEngine
	}
	if unix.PidfdSendSignal(fd, unix.SIGSTOP, nil, 0) != nil {
		if err := resume(); err != nil {
			return nil, err
		}
		return nil, ravpn.ErrEngine
	}
	paused = true
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state, stateErr = actualStopAgentState(m.Agent)
		if stateErr == nil && state == 'T' && actualStopVerifyAgent(m) == nil {
			return resume, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := resume(); err != nil {
		return nil, err
	}
	return nil, ravpn.ErrEngine
}

type actualStopObserver struct {
	record   ravpn.EngineRecord
	dispatch ravpn.UnitManagerDispatch
}

func (*actualStopObserver) Preflight(context.Context) error { return ravpn.ErrEngine }
func (o *actualStopObserver) Acquire(ctx context.Context, instance string) (*ravpn.UnitProcessSnapshot, error) {
	if instance != o.record.Spec.Instance || !(bootid.Reader{}).ForPID(o.record.Unit.PID).Equal(bootid.Identity{BootID: o.record.Unit.BootID, PID: o.record.Unit.PID, StartTime: o.record.Unit.StartTicks}) {
		return nil, ravpn.ErrEngine
	}
	before, err := o.dispatch(ctx, ravpn.UnitOperationInactive, instance)
	if err != nil || before.MainPID != o.record.Unit.PID || before.ControlPID != 0 || before.ActiveState != "active" {
		return nil, ravpn.ErrEngine
	}
	pid := strconv.Itoa(before.MainPID)
	// #nosec G304 -- complete actual daemon birth and fixed manager MainPID were matched; this disposable guest-only observer opens held kernel NET/EXE handles, not supplied file paths.
	network, err := os.Open("/proc/" + pid + "/ns/net")
	if err != nil {
		return nil, ravpn.ErrEngine
	}
	// #nosec G304 -- same complete held-process observation as NET above; actual installed executable comparison is subsequently performed by production SystemdUnits.
	executable, err := os.Open("/proc/" + pid + "/exe")
	if err != nil {
		_ = network.Close()
		return nil, ravpn.ErrEngine
	}
	snapshot := &ravpn.UnitProcessSnapshot{Instance: instance, Identity: o.record.Unit, ControlGroup: before.ControlGroup, Network: network, Executable: executable}
	after, err := o.dispatch(ctx, ravpn.UnitOperationInactive, instance)
	if err != nil || before != after || !(bootid.Reader{}).ForPID(before.MainPID).Equal(bootid.Identity{BootID: o.record.Unit.BootID, PID: o.record.Unit.PID, StartTime: o.record.Unit.StartTicks}) {
		_ = snapshot.Close()
		return nil, ravpn.ErrEngine
	}
	return snapshot, nil
}

func actualStopManager(instance string, fault *atomic.Bool, attempts *atomic.Uint32) ravpn.UnitManagerDispatch {
	return func(ctx context.Context, op ravpn.UnitOperation, wanted string) (ravpn.UnitManagerState, error) {
		if wanted != instance || !ravpn.ValidInstance(instance) {
			return ravpn.UnitManagerState{}, ravpn.ErrEngine
		}
		unit := "ngfw-ra@" + instance + ".service"
		switch op {
		case ravpn.UnitOperationStop:
			attempts.Add(1)
			if fault.Load() {
				return ravpn.UnitManagerState{}, ravpn.ErrEngine
			}
			// #nosec G204 -- fixed systemctl Stop only, canonical fullinstance unit positively bound to this guest manifest; no shell/path/unit injection.
			if exec.CommandContext(ctx, "/usr/bin/systemctl", "stop", unit).Run() != nil {
				return ravpn.UnitManagerState{}, ravpn.ErrEngine
			}
			return ravpn.UnitManagerState{}, nil
		case ravpn.UnitOperationObserve, ravpn.UnitOperationInactive:
			// #nosec G204 -- fixed read-only property request for the one validated fullinstance private guest unit.
			raw, err := exec.CommandContext(ctx, "/usr/bin/systemctl", "show", "--property=MainPID,ControlPID,ActiveState,ControlGroup", unit).Output()
			if err != nil || len(raw) > 4096 {
				return ravpn.UnitManagerState{}, ravpn.ErrEngine
			}
			fields := map[string]string{}
			for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
				key, value, ok := strings.Cut(line, "=")
				if !ok || fields[key] != "" {
					return ravpn.UnitManagerState{}, ravpn.ErrEngine
				}
				fields[key] = value
			}
			main, mainErr := strconv.Atoi(fields["MainPID"])
			control, controlErr := strconv.Atoi(fields["ControlPID"])
			if len(fields) != 4 || mainErr != nil || controlErr != nil {
				return ravpn.UnitManagerState{}, ravpn.ErrEngine
			}
			return ravpn.UnitManagerState{MainPID: main, ControlPID: control, ActiveState: fields["ActiveState"], ControlGroup: fields["ControlGroup"]}, nil
		default:
			return ravpn.UnitManagerState{}, ravpn.ErrEngine
		}
	}
}

type actualStopPreparation struct {
	record   ravpn.EngineRecord
	cleanups *atomic.Uint32
}

func (*actualStopPreparation) Prepare(context.Context, ravpn.EngineSpec) (*ravpn.PreparedEngine, error) {
	return nil, ravpn.ErrEngine
}
func (p *actualStopPreparation) Recover(_ context.Context, s ravpn.EngineSpec) (*ravpn.PreparedEngine, error) {
	if !reflect.DeepEqual(s, p.record.Spec) {
		return nil, ravpn.ErrEngine
	}
	// Fixture bookkeeping only: preserve genuine canonical snapshot and loaded
	// connection throughout the Stop fault, then let public RPC rollback clean it.
	return &ravpn.PreparedEngine{Cleanup: func(context.Context) error { p.cleanups.Add(1); return nil }}, nil
}

type actualStopHandoff struct{ calls *atomic.Uint32 }

func (h actualStopHandoff) Preflight(context.Context) error { return ravpn.ErrEngine }
func (h actualStopHandoff) Export(context.Context, *ravpn.NetworkPlan) error {
	h.calls.Add(1)
	return ravpn.ErrEngine
}
func (h actualStopHandoff) Verify(context.Context, *ravpn.NetworkPlan) error {
	h.calls.Add(1)
	return ravpn.ErrEngine
}
func (h actualStopHandoff) Remove(context.Context, *ravpn.NetworkPlan) error {
	h.calls.Add(1)
	return ravpn.ErrEngine
}

type actualStopTrace struct {
	vpp.Client
	calls atomic.Uint32
}

func (c *actualStopTrace) Invoke(ctx context.Context, request, reply api.Message) error {
	c.calls.Add(1)
	return c.Client.Invoke(ctx, request, reply)
}
func (c *actualStopTrace) NewStream(ctx context.Context, options ...api.StreamOption) (api.Stream, error) {
	c.calls.Add(1)
	return c.Client.NewStream(ctx, options...)
}
func (c *actualStopTrace) WatchEvent(ctx context.Context, event api.Message) (api.Watcher, error) {
	c.calls.Add(1)
	return c.Client.WatchEvent(ctx, event)
}

func actualStopDump(ctx context.Context, client vpp.Client, request api.Message) (rows []api.Message, failure error) {
	stream, err := client.NewStream(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if stream.Close() != nil {
			failure = ravpn.ErrEngine
		}
	}()
	if stream.SendMsg(request) != nil || stream.SendMsg(&memclnt.ControlPing{}) != nil {
		return nil, ravpn.ErrEngine
	}
	for len(rows) < 8192 {
		message, err := stream.RecvMsg()
		if err != nil {
			return nil, err
		}
		if ping, done := message.(*memclnt.ControlPingReply); done {
			if ping.Retval != 0 {
				return nil, ravpn.ErrEngine
			}
			return rows, nil
		}
		expected := map[string]string{"sw_interface_dump": "sw_interface_details", "sw_interface_tap_v2_dump": "sw_interface_tap_v2_details", "ip_table_dump": "ip_table_details", "ip_route_dump": "ip_route_details"}[request.GetMessageName()]
		if expected == "" || message.GetMessageName() != expected {
			return nil, ravpn.ErrEngine
		}
		rows = append(rows, message)
	}
	return nil, ravpn.ErrEngine
}

func actualStopSnapshot(ctx context.Context, client vpp.Client, manifest actualStopManifest) ([]string, error) {
	var rows []string
	bytesRead := 0
	tableCount := 0
	appendRows := func(messages []api.Message) error {
		for _, message := range messages {
			raw, err := json.Marshal(message)
			if err != nil {
				return err
			}
			bytesRead += len(raw)
			if len(rows) >= 32768 || bytesRead > 8<<20 {
				return ravpn.ErrEngine
			}
			rows = append(rows, message.GetMessageName()+":"+string(raw))
		}
		return nil
	}
	found, foundRoute := false, false
	for _, request := range []api.Message{&interfaceapi.SwInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(^uint32(0))}, &tapapi.SwInterfaceTapV2Dump{SwIfIndex: interface_types.InterfaceIndex(^uint32(0))}, &ipapi.IPTableDump{}} {
		messages, err := actualStopDump(ctx, client, request)
		if err != nil {
			return nil, err
		}
		for _, message := range messages {
			if detail, ok := message.(*interfaceapi.SwInterfaceDetails); ok && strings.TrimRight(detail.InterfaceName, "\x00") == manifest.ForeignInterface {
				found = true
			}
			if table, ok := message.(*ipapi.IPTableDetails); ok {
				tableCount++
				if tableCount > 64 {
					return nil, ravpn.ErrEngine
				}
				routes, err := actualStopDump(ctx, client, &ipapi.IPRouteDump{Table: table.Table})
				if err != nil {
					return nil, err
				}
				for _, row := range routes {
					if route, ok := row.(*ipapi.IPRouteDetails); ok && route.Route.TableID == manifest.ForeignRouteTable && route.Route.Prefix.String() == manifest.ForeignRoutePrefix {
						foundRoute = true
					}
				}
				if appendRows(routes) != nil {
					return nil, ravpn.ErrEngine
				}
			}
		}
		if appendRows(messages) != nil {
			return nil, ravpn.ErrEngine
		}
	}
	if !found || !foundRoute {
		return nil, ravpn.ErrEngine
	}
	ids, err := classifyapi.NewServiceClient(client).ClassifyTableIds(ctx, &classifyapi.ClassifyTableIds{})
	if err != nil {
		return nil, err
	}
	if len(ids.Ids) > 1024 || !slices.Contains(ids.Ids, uint32(0)) || !slices.Contains(ids.Ids, manifest.ForeignClassifyTable) {
		return nil, ravpn.ErrEngine
	}
	if appendRows([]api.Message{ids}) != nil {
		return nil, ravpn.ErrEngine
	}
	for _, id := range ids.Ids {
		table, err := classifyapi.NewServiceClient(client).ClassifyTableInfo(ctx, &classifyapi.ClassifyTableInfo{TableID: id})
		if err != nil {
			return nil, err
		}
		if appendRows([]api.Message{table}) != nil {
			return nil, ravpn.ErrEngine
		}
	}
	sort.Strings(rows)
	return rows, nil
}

func actualStopLoaded(ctx context.Context, units ravpn.SystemdUnits, plan *ravpn.NetworkPlan, record ravpn.EngineRecord) error {
	identity, err := units.Observe(ctx, plan)
	if err != nil || identity != record.Unit {
		return ravpn.ErrEngine
	}
	client, err := strongswan.DialRAVICI(ctx, filepath.Join(ravpn.InstanceRoot, plan.Instance, "daemon", "vici.sock"), identity.PID)
	if err != nil {
		return ravpn.ErrEngine
	}
	_, observeErr := strongswan.ObserveRASessions(ctx, client, record.Spec.Profile, identity.Generation(plan.Instance), record.Spec.Configuration.GetPools())
	pools, poolErr := client.Call(ctx, "get-pools", nil)
	valid := poolErr == nil && pools != nil && len(pools.Keys()) == len(record.Spec.Configuration.GetPools())
	if valid {
		for _, pool := range record.Spec.Configuration.GetPools() {
			name, nameErr := strongswan.ConnName(pool.GetName())
			if nameErr != nil || pools.Get(name) == nil {
				valid = false
			}
		}
	}
	closeErr := client.Close()
	after, afterErr := units.Observe(ctx, plan)
	if observeErr != nil || closeErr != nil || !valid || afterErr != nil || after != identity {
		return ravpn.ErrEngine
	}
	return nil
}

func TestIntegrationActualRAStopFaultBlocksConnectedBeforeVPP(t *testing.T) {
	if os.Getenv("NGFW_INTEGRATION") != "1" || os.Getenv("NGFW_RA_ACTUAL_STOP_GUEST") != "1" {
		t.Skip("requires separately provisioned independent PID1 guest and canonical live RA generation")
	}
	if actualStopGuest() != nil {
		t.Fatal("guest ownership boundary refused")
	}
	raw, err := actualStopReadPrivate("/run/ngfw-ra-stop-fixture.json", ravpn.MaxEngineSpecBytes+16384)
	if err != nil {
		t.Fatal("protected Stop fixture manifest absent")
	}
	var manifest actualStopManifest
	if actualStopDecode(raw, &manifest) != nil || actualStopValidateManifest(manifest) != nil || actualStopVerifyAgent(manifest) != nil {
		t.Fatal("actual Stop fixture identity refused")
	}
	plan, err := ravpn.ReadAgentPlan(manifest.Record.Spec.Instance)
	if err != nil || plan.Owner != manifest.Record.Spec.Owner || plan.NamespaceInode != manifest.Record.Unit.NamespaceInode {
		t.Fatal("actual protected daemon plan refused")
	}
	resume, err := actualStopPause(manifest)
	if err != nil {
		t.Fatal("owned canonical agent pause refused")
	}
	defer func() {
		if resume() != nil {
			t.Error("held canonical agent resume failed; retain guest for diagnosis")
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	connection := vpp.Dial("/run/vpp/api.sock", vpp.ConnOptions{ReplyTimeout: 5 * time.Second})
	defer connection.Close()
	for !connection.Connected() {
		select {
		case <-ctx.Done():
			t.Fatal("private guest VPP unavailable")
		case <-time.After(10 * time.Millisecond):
		}
	}
	identity, err := bootid.Current(ctx, connection)
	if err != nil || identity != manifest.VPP {
		t.Fatal("foreign or changed VPP generation")
	}
	before, err := actualStopSnapshot(ctx, connection, manifest)
	if err != nil {
		t.Fatal("actual foreign/interface/route/TAP/classify baseline readback refused")
	}
	var fault atomic.Bool
	fault.Store(true)
	var stopAttempts, cleanups, handoffCalls atomic.Uint32
	dispatch := actualStopManager(manifest.Record.Spec.Instance, &fault, &stopAttempts)
	observer := &actualStopObserver{record: manifest.Record, dispatch: dispatch}
	units, err := ravpn.NewSystemdUnitsForManager(observer, dispatch)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := units.Observe(ctx, plan)
	if err != nil || observed != manifest.Record.Unit {
		t.Fatal("genuine private charon held NET/EXE observation refused")
	}
	if actualStopLoaded(ctx, units, plan, manifest.Record) != nil {
		t.Fatal("actual loaded connection/pools/readback refused")
	}
	trace := &actualStopTrace{Client: connection}
	state := t.TempDir()
	wiring, err := Register(scheduler.NewRegistry(), Env{Client: trace, Owner: manifest.Record.Spec.Owner, StateDir: state, GlobalsOwner: true, Owned: ownertable.NewMemory(), Log: slog.New(slog.NewTextHandler(io.Discard, nil)), IDs: IDScope{All: true}, RA: &RAControllerOptions{Units: units, Preparation: &actualStopPreparation{record: manifest.Record, cleanups: &cleanups}, Handoff: actualStopHandoff{calls: &handoffCalls}, Inventory: func(_ context.Context, owner string) ([]*ravpn.NetworkPlan, error) {
		if owner != manifest.Record.Spec.Owner {
			return nil, ravpn.ErrEngine
		}
		return []*ravpn.NetworkPlan{plan}, nil
	}}})
	if err != nil {
		t.Fatal("complete fixture wiring registration refused", err)
	}
	defer func() {
		fault.Store(false)
		if wiring.StopRA(context.Background()) != nil {
			t.Error("verified fixture daemon teardown refused; retain state")
		}
		wiring.Close()
	}()
	store, err := ravpn.NewFileEngineStore(state, manifest.Record.Spec.Owner)
	if err != nil {
		t.Fatal(err)
	}
	if store.Save(manifest.Record) != nil || store.Close() != nil {
		t.Fatal("copied public engine record refused")
	}
	trace.calls.Store(0)
	wiring.Connected(ctx)
	if trace.calls.Load() != 0 || stopAttempts.Load() != 1 || cleanups.Load() != 0 || handoffCalls.Load() != 0 {
		t.Fatal("failed actual Stop reached VPP/native/sentinel/transport or snapshot cleanup")
	}
	if RARuntimeFor(manifest.Record.Spec.Owner).TransportGuard(ctx, plan) == nil {
		t.Fatal("failed live generation lost transport mutation barrier")
	}
	afterUnit, err := units.Observe(ctx, plan)
	if err != nil || afterUnit != observed {
		t.Fatal("fault stopped or replaced actual charon")
	}
	afterBoot, bootErr := bootid.Current(ctx, connection)
	if bootErr != nil || afterBoot != manifest.VPP || actualStopVerifyAgent(manifest) != nil {
		t.Fatal("guest agent or VPP generation changed during Stop fault")
	}
	after, err := actualStopSnapshot(ctx, connection, manifest)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("actual foreign/native/route/TAP/classify state changed on failed Stop")
	}
	if actualStopLoaded(ctx, units, plan, manifest.Record) != nil {
		t.Fatal("failed Stop changed actual loaded connection/pools")
	}
	if trace.calls.Load() != 0 {
		t.Fatal("failed generation guard reached VPP")
	}
	fault.Store(false)
	if wiring.StopRA(ctx) != nil || units.Inactive(ctx, plan) != nil || cleanups.Load() != 1 {
		t.Fatal("fault removal did not permit positively verified daemon stop")
	}
	t.Log("supplemental actual-process Stop-result injection PASS: same live NET/EXE/full birth retained, zero Wiring VPP calls and unchanged actual foreign/native state; production manager Stop-failure acceptance is separate")
}

func TestActualStopPublicManifestAndPrivateReaderRefuseAmbiguity(t *testing.T) {
	own := (bootid.Reader{}).ForPID(os.Getpid())
	if _, err := actualStopAgentState(own); err != nil {
		t.Fatal("actual own process birth readback failed", err)
	}
	changed := own
	changed.StartTime++
	if _, err := actualStopAgentState(changed); err == nil {
		t.Fatal("changed process birth accepted")
	}
	if actualStopValidateManifest(actualStopManifest{}) == nil {
		t.Fatal("empty identity accepted")
	}
	var value map[string]any
	if actualStopDecode([]byte(`{"a":1} {"b":2}`), &value) == nil {
		t.Fatal("trailing JSON accepted")
	}
	file := filepath.Join(t.TempDir(), "public-marker")
	if err := os.WriteFile(file, []byte("public fixture marker"), 0600); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		if _, err := actualStopReadPrivate(file, 1024); err != nil {
			t.Fatal("protected owned marker refused", err)
		}
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	if _, err := actualStopReadPrivate(link, 1024); err == nil {
		t.Fatal("symlink accepted")
	}
	if err := os.Link(file, file+"-second"); err != nil {
		t.Fatal(err)
	}
	if _, err := actualStopReadPrivate(file, 1024); err == nil {
		t.Fatal("multiply linked metadata accepted")
	}
	var fault atomic.Bool
	fault.Store(true)
	var attempts atomic.Uint32
	dispatch := actualStopManager(strings.Repeat("a", 64), &fault, &attempts)
	if _, err := dispatch(context.Background(), ravpn.UnitOperationStop, strings.Repeat("b", 64)); err == nil || attempts.Load() != 0 {
		t.Fatal("foreign unit reached dispatcher")
	}
	if _, err := dispatch(context.Background(), ravpn.UnitOperationStart, strings.Repeat("a", 64)); err == nil {
		t.Fatal("fixture exposed unit start")
	}
	if _, err := dispatch(context.Background(), ravpn.UnitOperationStop, strings.Repeat("a", 64)); !errors.Is(err, ravpn.ErrEngine) || attempts.Load() != 1 {
		t.Fatal("narrow fixed Stop fault absent")
	}
}
