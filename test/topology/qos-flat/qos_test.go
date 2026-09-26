package qosflat

// F-qos-flat host test (FAST-MODE definition of done): the real vrx-agent binary (owner = the slot prefix, the slot's
// id range) on the host VPP, driven through its gRPC API — no API/DB needed for the data-plane evidence. Only the
// slot's loopbacks carry QoS (loop<N>71..73), no packets are sent (no rig, no V19 preflight), policer names carry the
// slot prefix twice (VPP name "w<N>:w<N>-gold"), egress map ids come from the slot range (<N>000–<N>999).
//
//  1. commit → Retrieve equals the document (minus the write-only attachments, D-063) and `vppctl show policer`,
//     `show qos egress map`, `show qos mark`, `show qos record`, `show qos store` reflect it; every policer attachment
//     appears exactly once in `show interface features` (D-076);
//  2. QosPolicerState lists the policers and the shaper with counters; QosPolicerReset refills one;
//  3. restart safety without restarting VPP: stop the agent, delete a policer and a map behind its back (binary API),
//     start it → both back within 30 s, the attachments still exactly once;
//  4. rollback: nothing of ours remains (Retrieve + vppctl), no out-of-bounds un-apply, NRestarts unchanged.
//
// Only show commands are used (never a packet trace, D-128); every process is ours and stopped by PID.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	govpp "go.fd.io/govpp"
	vppapi "go.fd.io/govpp/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/policer"
	"ngfw/agent/binapi/qos"
	vrxv1 "ngfw/agent/gen/vrx/v1"
)

const (
	labLock   = "/run/lock/vrx-lab.lock"
	apiSocket = "/run/vpp/api.sock"
)

type slot struct {
	prefix  string
	num     int
	metrics string
	runDir  string
	socket  string
	base    int // VRX_VPP_TABLE_BASE
}

func slotFromEnv(t *testing.T) slot {
	t.Helper()
	p := os.Getenv("VRX_TEST_PREFIX")
	m := regexp.MustCompile(`^w([0-9]{1,2})$`).FindStringSubmatch(p)
	if m == nil {
		t.Fatalf("VRX_TEST_PREFIX=%q: eval \"$(tools/lab env <N>)\" first", p)
	}
	n, _ := strconv.Atoi(m[1])
	env := func(k, def string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return def
	}
	base, err := strconv.Atoi(env("VRX_VPP_TABLE_BASE", strconv.Itoa(1000*n)))
	if err != nil {
		t.Fatal(err)
	}
	s := slot{prefix: p, num: n, metrics: env("VRX_METRICS_PORT", strconv.Itoa(9100+10*n+1)), runDir: "/run/vrx-test/" + p, base: base}
	s.socket = env("VRX_AGENT_SOCKET", s.runDir+"/agent.sock")
	return s
}

func sharedLock(t *testing.T) {
	t.Helper()
	f, err := os.OpenFile(labLock, os.O_RDONLY|os.O_CREATE, 0o666) //nolint:gosec // the lab lock is shared by design
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil { //nolint:gosec // G115: an fd
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }) //nolint:gosec // G115: an fd
}

func run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(out), err
}

func vppctl(t *testing.T, args ...string) string {
	t.Helper()
	if args[0] != "show" {
		t.Fatalf("only show commands on the shared VPP (D-128): %v", args)
	}
	out, err := run("vppctl", args...)
	if err != nil {
		t.Fatalf("vppctl %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func nRestarts(t *testing.T) int {
	t.Helper()
	out, err := run("systemctl", "show", "vpp", "-p", "NRestarts")
	if err != nil {
		t.Fatalf("systemctl: %v %s", err, out)
	}
	n, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(out), "NRestarts="))
	if err != nil {
		t.Fatalf("NRestarts %q", out)
	}
	return n
}

type proc struct {
	cmd  *exec.Cmd
	done chan struct{}
}

func startAgent(t *testing.T, s slot, work string) *proc {
	t.Helper()
	bin := os.Getenv("VRX_QOS_AGENT_BIN")
	if bin == "" {
		t.Fatal("VRX_QOS_AGENT_BIN unset (use run.sh)")
	}
	f, err := os.OpenFile(filepath.Join(work, "agent.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin) //nolint:gosec // the agent binary run.sh built from this tree
	cmd.Env = append(os.Environ()[:0:0], "PATH="+os.Getenv("PATH"),
		"VRX_AGENT_SOCKET="+s.socket, "VRX_OWNER="+s.prefix, "VRX_GLOBALS_OWNER=0", // D-071: a slot never owns globals
		"VRX_AGENT_STATE_DIR="+filepath.Join(work, "state"), "VRX_METRICS_PORT="+s.metrics, "VRX_SOCKET_GROUP=root",
		"VRX_VPP_TABLE_BASE="+strconv.Itoa(s.base), "VRX_LOG_LEVEL=info")
	cmd.Stdout, cmd.Stderr = f, f
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := &proc{cmd: cmd, done: make(chan struct{})}
	go func() { _ = cmd.Wait(); _ = f.Close(); close(p.done) }()
	t.Logf("started vrx-agent pid %d (owner %s, log %s)", cmd.Process.Pid, s.prefix, filepath.Join(work, "agent.log"))
	return p
}

func (p *proc) stop(t *testing.T) {
	select {
	case <-p.done:
		return
	default:
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-p.done:
	case <-time.After(15 * time.Second):
		_ = p.cmd.Process.Kill()
		<-p.done
	}
	t.Logf("stopped vrx-agent pid %d", p.cmd.Process.Pid)
}

func dial(t *testing.T, socket string) vrxv1.DataplaneClient {
	t.Helper()
	cc, err := grpc.NewClient("unix:"+socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cc.Close() })
	return vrxv1.NewDataplaneClient(cc)
}

func waitFor(timeout time.Duration, f func() bool) bool {
	end := time.Now().Add(timeout)
	for time.Now().Before(end) {
		if f() {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return f()
}

// qosDoc is the document of this slot: three loopbacks and one of every flat QoS object.
func qosDoc(s slot) string {
	return strings.NewReplacer("{P}", s.prefix, "{N}", strconv.Itoa(s.num), "{ID}", strconv.Itoa(s.base+1)).Replace(`{
  "interfaces": {"loop{N}71": {}, "loop{N}72": {}, "loop{N}73": {}},
  "services": {"qos": {
    "policers": {
      "{P}-gold": {"description": "customer", "type": "2r3c-rfc2698", "rateUnit": "kbps", "cir": 20000, "eir": 40000, "cb": "25000", "eb": "50000",
                   "round": "closest", "colorAware": false, "conformAction": {"action": "transmit"},
                   "exceedAction": {"action": "mark-and-transmit", "dscp": 10}, "violateAction": {"action": "drop"}},
      "{P}-pps": {"type": "1r2c", "rateUnit": "pps", "cir": 1000, "cb": "100", "round": "closest", "colorAware": false,
                  "conformAction": {"action": "transmit"}, "exceedAction": {"action": "drop"}, "violateAction": {"action": "drop"}}
    },
    "shapers": {"{P}-up": {"description": "drop-based", "rateKbps": 50000}},
    "maps": {"{P}-remark": {"id": {ID}, "rows": {"ip": [{"from": 46, "to": 34}]}}, "{P}-pcp": {"rows": {"ip": [{"from": 46, "to": 5}]}}},
    "interfaces": {
      "loop{N}71": {"description": "customer", "policer": {"input": "{P}-gold"}, "shaper": "{P}-up", "record": "vlan", "store": {"source": "ip", "value": 0}},
      "loop{N}72": {"policer": {"output": "{P}-pps"}, "record": "ip", "mark": {"map": "{P}-remark", "output": "ip"}},
      "loop{N}73": {"policer": {"input": "{P}-pps"}}
    }
  }}
}`)
}

// retrievedQoS is what Retrieve must report for qosDoc: the write-only attachments are not reported (loop<N>73 had
// only one), the auto-numbered map has no id.
func retrievedQoS(s slot) string {
	return strings.NewReplacer("{P}", s.prefix, "{N}", strconv.Itoa(s.num), "{ID}", strconv.Itoa(s.base+1)).Replace(`{"services": {"qos": {
  "policers": {
    "{P}-gold": {"description": "customer", "type": "2r3c-rfc2698", "rateUnit": "kbps", "cir": 20000, "eir": 40000, "cb": "25000", "eb": "50000",
                 "round": "closest", "colorAware": false, "conformAction": {"action": "transmit"},
                 "exceedAction": {"action": "mark-and-transmit", "dscp": 10}, "violateAction": {"action": "drop"}},
    "{P}-pps": {"type": "1r2c", "rateUnit": "pps", "cir": 1000, "cb": "100", "round": "closest", "colorAware": false,
                "conformAction": {"action": "transmit"}, "exceedAction": {"action": "drop"}, "violateAction": {"action": "drop"}}
  },
  "shapers": {"{P}-up": {"description": "drop-based", "rateKbps": 50000}},
  "maps": {"{P}-remark": {"id": {ID}, "rows": {"ip": [{"from": 46, "to": 34}]}}, "{P}-pcp": {"rows": {"ip": [{"from": 46, "to": 5}]}}},
  "interfaces": {
    "loop{N}71": {"description": "customer", "record": "vlan", "store": {"source": "ip", "value": 0}},
    "loop{N}72": {"record": "ip", "mark": {"map": "{P}-remark", "output": "ip"}}
  }
}}}`)
}

func parse(t *testing.T, js string) *vrxv1.DesiredState {
	t.Helper()
	ds := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(js), ds); err != nil {
		t.Fatal(err)
	}
	return ds
}

func applyDoc(t *testing.T, c vrxv1.DataplaneClient, s slot, id, js string) *vrxv1.ApplyResponse {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	resp, err := c.Apply(ctx, &vrxv1.ApplyRequest{TxnId: fmt.Sprintf("%s-qos-%s-%d", s.prefix, id, time.Now().UnixNano()), Owner: s.prefix,
		DesiredState: parse(t, js), Subsystems: []string{"interfaces", "services"}})
	if err != nil {
		t.Fatalf("apply %s: %v", id, err)
	}
	return resp
}

func retrieveQoS(t *testing.T, c vrxv1.DataplaneClient, s slot) *vrxv1.QosService {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	r, err := c.Retrieve(ctx, &vrxv1.RetrieveRequest{Owner: s.prefix, Subsystems: []string{"services"}})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	return r.GetDesiredState().GetServices().GetQos()
}

// features counts the policer feature instances on an interface (`show interface features`): one apply enables
// policer-input on the ip4-unicast and ip6-unicast arcs (policer-output on ip4-output / ip6-output, policer_op.c), so
// a stacked double apply (D-076) doubles the count. The L2 bitmap names ("l2-policer-input") are not counted.
func features(t *testing.T, ifName, node string) int {
	t.Helper()
	return len(regexp.MustCompile(`(?m)^\s*`+regexp.QuoteMeta(node)+`(\s|$)`).FindAllString(vppctl(t, "show", "interface", "features", ifName), -1))
}

// policerIndex walks policer_dump_v2 per index (policer_details carries no index) for the VPP name.
func policerIndex(t *testing.T, conn vppapi.Connection, name string) (uint32, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dump := func(idx uint32) []*policer.PolicerDetails {
		st, err := policer.NewServiceClient(conn).PolicerDumpV2(ctx, &policer.PolicerDumpV2{PolicerIndex: idx})
		if err != nil {
			t.Fatal(err)
		}
		var out []*policer.PolicerDetails
		for {
			d, err := st.Recv()
			if errors.Is(err, io.EOF) {
				return out
			}
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, d)
		}
	}
	total := len(dump(^uint32(0)))
	seen := 0
	for idx := uint32(0); seen < total && idx < 1<<16; idx++ {
		d := dump(idx)
		if len(d) == 0 {
			continue
		}
		seen++
		if strings.TrimRight(d[0].Name, "\x00") == name {
			return idx, true
		}
	}
	return 0, false
}

func TestQoSFlatHost(t *testing.T) {
	if os.Getenv("VRX_INTEGRATION") != "1" {
		t.Skip("host test (VRX_INTEGRATION=1 via run.sh); host runs on the shared VPP wait for TD-25")
	}
	s := slotFromEnv(t)
	sharedLock(t)
	before := nRestarts(t)
	t.Logf("systemctl show vpp -p NRestarts (before): NRestarts=%d", before)
	t.Cleanup(func() {
		after := nRestarts(t)
		t.Logf("systemctl show vpp -p NRestarts (after): NRestarts=%d", after)
		if after != before {
			t.Errorf("VPP restarted during the test (NRestarts %d → %d): stop host runs and write it down", before, after)
		}
	})
	work := filepath.Join(s.runDir, "qos-flat")
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	agent := startAgent(t, s, work)
	t.Cleanup(func() { agent.stop(t) })
	c := dial(t, s.socket)
	if !waitFor(30*time.Second, func() bool {
		h, err := c.Health(context.Background(), &vrxv1.HealthRequest{})
		return err == nil && h.GetVppConnected() && !h.GetReconcileInProgress()
	}) {
		t.Fatal("agent not connected to VPP within 30 s")
	}
	empty := `{"interfaces": {}, "services": {}}`
	t.Cleanup(func() { // rollback also when the test failed half-way (the agent may have been restarted)
		if resp := applyDoc(t, c, s, "cleanup", empty); resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
			t.Errorf("cleanup apply: %v", resp)
		}
	})
	loop := func(k int) string { return fmt.Sprintf("loop%d%d", s.num, 70+k) }
	vppName := func(n string) string { return s.prefix + ":" + s.prefix + "-" + n }

	// 1. commit → Retrieve, vppctl, attachments exactly once
	if resp := applyDoc(t, c, s, "apply", qosDoc(s)); resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("apply: %v", resp)
	}
	want := parse(t, retrievedQoS(s)).GetServices().GetQos()
	if got := retrieveQoS(t, c, s); !proto.Equal(got, want) {
		t.Fatalf("Retrieve\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}
	for _, args := range [][]string{{"show", "policer"}, {"show", "qos", "egress", "map"}, {"show", "qos", "mark"}, {"show", "qos", "record"}, {"show", "qos", "store"}} {
		out := vppctl(t, args...)
		t.Logf("$ vppctl %s\n%s", strings.Join(args, " "), out)
	}
	pol := vppctl(t, "show", "policer")
	for _, n := range []string{"gold", "pps", "up"} {
		name := vppName(n)
		if n == "up" {
			name = s.prefix + ":shaper:" + s.prefix + "-up"
		}
		if !strings.Contains(pol, name) {
			t.Errorf("show policer lacks %s", name)
		}
	}
	var once [4]int // the feature count of one apply per attachment, taken after the first commit
	checkOnce := func(stage string) {
		t.Helper()
		got := [4]int{features(t, loop(1), "policer-input"), features(t, loop(1), "policer-output"), features(t, loop(2), "policer-output"), features(t, loop(3), "policer-input")}
		t.Logf("%s: policer feature entries in/out %s, out %s, in %s = %v\n$ vppctl show interface features %s\n%s", stage, loop(1), loop(2), loop(3), got,
			loop(1), vppctl(t, "show", "interface", "features", loop(1)))
		if once == [4]int{} {
			for _, n := range got {
				if n == 0 || n != got[0] {
					t.Fatalf("%s: every attachment must be applied, once each: %v", stage, got)
				}
			}
			once = got
		}
		if got != once {
			t.Fatalf("%s: an attachment was applied again or lost (D-076): %v, after the first commit %v", stage, got, once)
		}
	}
	checkOnce("after commit")
	// a repeat is an empty plan (write-only attachments are not applied again)
	if resp := applyDoc(t, c, s, "repeat", qosDoc(s)); resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED || len(resp.GetResults()) != 0 {
		t.Fatalf("repeat: %v", resp)
	}
	checkOnce("after a repeated commit")

	// 2. RPCs
	ctx := context.Background()
	st, err := c.QosPolicerState(ctx, &vrxv1.QosPolicerStateRequest{Owner: s.prefix})
	if err != nil || len(st.GetPolicers()) != 3 {
		t.Fatalf("QosPolicerState %v %v", st, err)
	}
	t.Logf("QosPolicerState: %s", protojson.Format(st))
	if r, err := c.QosPolicerReset(ctx, &vrxv1.QosPolicerResetRequest{Owner: s.prefix, Name: s.prefix + "-gold"}); err != nil {
		t.Fatalf("QosPolicerReset: %v", err)
	} else {
		t.Logf("QosPolicerReset: %s", protojson.Format(r))
	}

	// 3. restart safety without restarting VPP
	agent.stop(t)
	conn, err := govpp.Connect(apiSocket)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Disconnect()
	idx, ok := policerIndex(t, conn, vppName("gold"))
	if !ok {
		t.Fatal("gold not found in VPP")
	}
	lossCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := policer.NewServiceClient(conn).PolicerDel(lossCtx, &policer.PolicerDel{PolicerIndex: idx}); err != nil {
		t.Fatalf("simulated loss: policer_del: %v", err)
	}
	// the gold policer is attached to loop<N>71's input: VPP keeps the feature (its index is stale); the resync
	// re-creates the policer under the same name, and the attachment by name stays applied once
	if _, err := qos.NewServiceClient(conn).QosEgressMapDelete(lossCtx, &qos.QosEgressMapDelete{ID: uint32(s.base)}); err != nil { //nolint:gosec // G115: slot base
		t.Fatalf("simulated loss: qos_egress_map_delete: %v", err)
	}
	t.Logf("simulated loss: policer %s (index %d) and egress map %d deleted with the agent stopped", vppName("gold"), idx, s.base)
	agent = startAgent(t, s, work)
	start := time.Now()
	if !waitFor(30*time.Second, func() bool {
		h, err := c.Health(ctx, &vrxv1.HealthRequest{})
		if err != nil || !h.GetVppConnected() || h.GetReconcileInProgress() {
			return false
		}
		return proto.Equal(retrieveQoS(t, c, s), want)
	}) {
		t.Fatalf("not rebuilt within 30 s: %s", protojson.Format(retrieveQoS(t, c, s)))
	}
	t.Logf("rebuilt after the agent restart in %s", time.Since(start).Round(time.Millisecond))
	checkOnce("after the agent restart")

	// 4. rollback
	if resp := applyDoc(t, c, s, "rollback", empty); resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("rollback: %v", resp)
	}
	if got := retrieveQoS(t, c, s); !proto.Equal(got, &vrxv1.QosService{}) {
		t.Fatalf("after rollback Retrieve: %s", protojson.Format(got))
	}
	pol = vppctl(t, "show", "policer")
	if strings.Contains(pol, s.prefix+":") {
		t.Fatalf("policers of %s left:\n%s", s.prefix, pol)
	}
	maps := vppctl(t, "show", "qos", "egress", "map")
	for _, id := range []int{s.base, s.base + 1} {
		if regexp.MustCompile(`Map-ID:` + strconv.Itoa(id) + `\b`).MatchString(maps) { // qos_egress_map.c: " Map-ID:%d"
			t.Fatalf("egress map %d left:\n%s", id, maps)
		}
	}
	t.Logf("after rollback:\n$ vppctl show policer\n%s\n$ vppctl show qos egress map\n%s\n$ vppctl show qos mark\n%s", pol, maps, vppctl(t, "show", "qos", "mark"))
}
