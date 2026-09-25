package agent

// F-wireguard host check (VRX_INTEGRATION=1, shared lab lock, one package at a time — D-087). The agent's
// pieces are built in process over the host VPP exactly as Start builds them (subsystems.Register,
// Service), with the slot-local secret fixture put into the family's store before the first
// transaction — the stand-in for the sealed cache of PENDING-secret-channel option 1. Owner
// "<prefix>wg" (F-bonding shares slot 6), instances/tables base+51…53, 10.<slot>.5x.0/24, ports
// 20000+100·slot+10/+11 (never 51820). VPP is never restarted; everything is removed in t.Cleanup.
//
//	TestWireguardOnHost            apply → Retrieve == desired → idempotent → WireguardState → pause
//	                               for vppctl (VRX_WG_PAUSE) → simulated loss + agent restart →
//	                               recreated → rollback (empty vpn) → nothing left (binapi)
//	TestWireguardHandshakeOnHost   (VRX_WG_HANDSHAKE=1) a kernel WireGuard peer in the netns
//	                               ns-<owner>, reached through a tap of this owner (no af_packet: V24):
//	                               handshake → EVENT_KIND_WIREGUARD_PEER_CHANGED established published,
//	                               WireguardState established with last_handshake, ping through it.
//	                               (Two VPP wg interfaces peering over local addresses do not work:
//	                               ip4-local drops the handshake as a spoofed local-address packet.)

import (
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/wireguard"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/ifsanitize"
	"ngfw/agent/internal/vpp/vpptest"
)

// wgHost is one in-process agent (wiring + service) over the host VPP.
type wgHost struct {
	svc    *Service
	cancel context.CancelFunc
	events chan *vrxv1.Event
}

func startWgHost(t *testing.T, c *vpp.Conn, owner, dir string, secrets map[string][]byte) *wgHost {
	t.Helper()
	owned, err := ownertable.Open(dir, owner)
	if err != nil {
		t.Fatal(err)
	}
	h := &wgHost{events: make(chan *vrxv1.Event, 64)}
	reg := scheduler.NewRegistry()
	w, err := subsystems.Register(reg, subsystems.Env{Client: c, Owner: owner, StateDir: dir, Owned: owned,
		Publish: func(ev *vrxv1.Event) {
			select {
			case h.events <- ev:
			default:
			}
		}})
	if err != nil {
		t.Fatal(err)
	}
	for ref, m := range secrets {
		if err := subsystems.WireguardSecretsFor(owner).Put(ref, m); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	w.Connected(ctx) // boot identity, peer-event watcher
	h.svc, err = NewService(ServiceConfig{Owner: owner, Version: "it", VPP: c, Scheduler: scheduler.New(reg, nil), StateDir: dir, BeforeTxn: w.BeforeTxn, NetdevKind: w.NetdevKind()})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *wgHost) stop() {
	h.svc.Close()
	h.cancel()
}

func nrestarts(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("systemctl", "show", "vpp", "-p", "NRestarts").Output()
	if err != nil {
		return "unknown (" + err.Error() + ")"
	}
	return strings.TrimSpace(string(out))
}

// wgHostKeys returns the slot's fixture material: SHA-256 of "VRX_TEST_PSK_F-wireguard_<owner>_<label>".
func wgHostKeys(owner string) map[string][]byte {
	return map[string][]byte{
		"key/" + owner + "-a":  wgVector(owner + "_itf_a"),
		"key/" + owner + "-b":  wgVector(owner + "_itf_b"),
		"psk/" + owner + "-b1": wgVector(owner + "_psk_b1"),
	}
}

func pubOf(t *testing.T, material []byte) string {
	t.Helper()
	k, err := ecdh.X25519().NewPrivateKey(material)
	if err != nil {
		t.Fatal(err)
	}
	return b64(k.PublicKey().Bytes())
}

// wgHostDoc returns the applied document and its canonical Retrieve form.
func wgHostDoc(t *testing.T, owner string) (*vrxv1.DesiredState, *vrxv1.DesiredState) {
	t.Helper()
	slot, base := vpptest.Slot(t), vpptest.TableBase(t)
	p1, p2 := pubOf(t, wgVector(owner+"_peer1")), pubOf(t, wgVector(owner+"_peer2"))
	body := func(canon bool) string {
		keep := `"persistentKeepaliveSec": 0, `
		if !canon {
			keep = ""
		}
		return fmt.Sprintf(`{
	  "vrfs": {%[1]q: {"id": %[2]d}},
	  "vpn": {"wireguard": {"interfaces": {"site-a": {
	    "enabled": true, "description": "F-wireguard host check", "instance": %[2]d, "vrf": %[1]q, "underlayVrf": "default",
	    "listenAddress": "10.%[3]d.51.1", "listenPort": %[4]d, "privateKeyRef": "key/%[5]s-a",
	    "address": ["10.%[3]d.52.1/24"], "mtu": 1420, "routeAllowedIps": true,
	    "peers": {
	      "b1": {"publicKey": %[6]q, "presharedKeyRef": "psk/%[5]s-b1", "endpoint": {"address": "10.%[3]d.51.2", "port": %[7]d},
	             "allowedIps": ["10.%[3]d.53.0/24"], "persistentKeepaliveSec": 25},
	      "b2": {%[9]s"publicKey": %[8]q, "allowedIps": ["10.%[3]d.54.0/24"]}
	    }}}}}
	}`, owner+"-red", base+51, slot, 20000+100*slot+10, owner, p1, 20000+100*slot+11, p2, keep)
	}
	return doc(t, body(false)), doc(t, body(true))
}

// ownedWireguard lists this owner's wg interfaces (name → sw_if_index) and their peers (index) via binapi.
func ownedWireguard(t *testing.T, c vpp.Client, owner string) (map[string]uint32, []uint32) {
	t.Helper()
	ctx := context.Background()
	ifs, _ := ownedOnHost(t, c, owner)
	wgs := map[string]uint32{}
	onOurs := map[uint32]bool{}
	for name, idx := range ifs {
		if strings.HasPrefix(name, "wg") {
			wgs[name] = idx
			onOurs[idx] = true
		}
	}
	st, err := wireguard.NewServiceClient(c).WireguardPeersDump(ctx, &wireguard.WireguardPeersDump{PeerIndex: ^uint32(0)})
	if err != nil {
		t.Fatal(err)
	}
	var peers []uint32
	for {
		d, err := st.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if onOurs[uint32(d.Peer.SwIfIndex)] {
			peers = append(peers, d.Peer.PeerIndex)
		}
	}
	return wgs, peers
}

// deleteWireguard simulates loss (or cleans up): this owner's peers and wg interfaces via binapi.
func deleteWireguard(t *testing.T, c vpp.Client, owner string) int {
	t.Helper()
	ctx := context.Background()
	wgs, peers := ownedWireguard(t, c, owner)
	svc := wireguard.NewServiceClient(c)
	for _, p := range peers {
		if _, err := svc.WireguardPeerRemove(ctx, &wireguard.WireguardPeerRemove{PeerIndex: p}); err != nil {
			t.Errorf("remove peer %d: %v", p, err)
		}
	}
	for name, idx := range wgs {
		if err := ifsanitize.BeforeDelete(ctx, c, idx, name); err != nil {
			t.Errorf("clear %s before delete: %v", name, err)
		}
		if _, err := svc.WireguardInterfaceDelete(ctx, &wireguard.WireguardInterfaceDelete{SwIfIndex: interface_types.InterfaceIndex(idx)}); err != nil {
			t.Errorf("delete %s: %v", name, err)
		}
	}
	return len(wgs) + len(peers)
}

func wgRetrieve(t *testing.T, s *Service) *vrxv1.DesiredState {
	t.Helper()
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{})
	if err != nil {
		t.Fatal(err)
	}
	return got.GetDesiredState()
}

// wgRelevant is the part of a Retrieve the WireGuard check compares: vrfs and vpn, and it fails
// when WireGuard leaves leaked into interfaces or routing.static.
func wgRelevant(t *testing.T, ds *vrxv1.DesiredState) *vrxv1.DesiredState {
	t.Helper()
	for name := range ds.GetInterfaces() {
		if strings.HasPrefix(name, "wg") {
			t.Fatalf("%s reported under interfaces", name)
		}
	}
	if n := len(ds.GetRouting().GetStatic()); n != 0 {
		t.Fatalf("%d static routes reported (the allowed-IP routes belong to vpn.wireguard)", n)
	}
	return &vrxv1.DesiredState{Vrfs: ds.GetVrfs(), Vpn: ds.GetVpn()}
}

func TestWireguardOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	owner := vpptest.Prefix(t) + "wg" // F-bonding shares the slot: a sub-owner of its own
	t.Logf("VPP %s before", nrestarts(t))
	t.Cleanup(func() { t.Logf("VPP %s after", nrestarts(t)) })
	raw := vpp.Dial(vppSocket(), vpp.ConnOptions{})
	t.Cleanup(raw.Close)
	wctx, wcancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer wcancel()
	if err := raw.WaitConnected(wctx); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "state")
	keys := wgHostKeys(owner)
	t.Cleanup(func() {
		h := startWgHost(t, raw, owner, dir, keys)
		defer h.stop()
		resp := apply(t, h.svc, &vrxv1.ApplyRequest{TxnId: owner + "-cleanup", DesiredState: &vrxv1.DesiredState{}, Subsystems: allDomains})
		if resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
			t.Errorf("cleanup apply: %v", resp)
		}
		if n := deleteWireguard(t, raw, owner); n != 0 {
			t.Errorf("cleanup left %d WireGuard objects (deleted via binapi)", n)
		}
		deleteOwned(t, raw, owner)
	})
	deleteWireguard(t, raw, owner) // leftovers of an aborted run
	deleteOwned(t, raw, owner)

	desiredDoc, canonical := wgHostDoc(t, owner)
	want := wgRelevant(t, canonical)
	h := startWgHost(t, raw, owner, dir, keys)
	ctx := context.Background()

	// 1. apply → Retrieve == desired
	resp := apply(t, h.svc, &vrxv1.ApplyRequest{TxnId: owner + "-1", DesiredState: desiredDoc, Subsystems: allDomains})
	if resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("apply: %s", protojson.Format(resp))
	}
	t.Logf("apply: %s", protojson.Format(resp.GetSummary()))
	for _, r := range resp.GetResults() {
		t.Logf("  %-60s %s", r.GetKey(), r.GetPointer())
	}
	if got := wgRelevant(t, wgRetrieve(t, h.svc)); !proto.Equal(got, want) {
		t.Fatalf("retrieve:\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}
	t.Log("Retrieve == desired (vrfs + vpn.wireguard; no wg leaves under interfaces/routing)")

	// 2. idempotent
	resp = apply(t, h.svc, &vrxv1.ApplyRequest{TxnId: owner + "-2", DesiredState: desiredDoc, Subsystems: allDomains})
	if resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED || len(resp.GetResults()) != 0 {
		t.Fatalf("idempotent apply: %s", protojson.Format(resp))
	}
	t.Logf("idempotent apply: %s", protojson.Format(resp.GetSummary()))

	// 3. live state
	st, err := h.svc.WireguardState(ctx, &vrxv1.WireguardStateRequest{}, newStatsReader("/run/vpp/stats.sock", h.svc.log))
	if err != nil {
		t.Fatal(err)
	}
	stJSON := protojson.Format(st)
	t.Logf("WireguardState: %s", stJSON)
	if len(st.GetInterfaces()) != 1 || len(st.GetInterfaces()[0].GetPeers()) != 2 || !st.GetInterfaces()[0].GetAdminUp() {
		t.Fatalf("state %s", stJSON)
	}
	for _, m := range keys {
		if strings.Contains(stJSON, b64(m)) {
			t.Fatal("key material in WireguardState")
		}
	}
	if s, _ := strconv.Atoi(os.Getenv("VRX_WG_PAUSE")); s > 0 {
		t.Logf("pausing %d s for vppctl evidence (redact the private key and mac-key lines)", s)
		time.Sleep(time.Duration(s) * time.Second)
	}

	// 4. restart simulation: stop the agent, delete our WireGuard objects via binapi, start → recreated
	h.stop()
	n := deleteWireguard(t, raw, owner)
	if n != 3 {
		t.Fatalf("deleted %d objects, want 1 interface + 2 peers", n)
	}
	start := time.Now()
	h = startWgHost(t, raw, owner, dir, keys)
	rs := h.svc.Resync(ctx)
	took := time.Since(start)
	if rs.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("resync: %s", protojson.Format(rs))
	}
	if got := wgRelevant(t, wgRetrieve(t, h.svc)); !proto.Equal(got, want) {
		t.Fatalf("after restart:\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}
	t.Logf("restart after loss: resync %s, converged in %v", protojson.Format(rs.GetSummary()), took)
	if took > 30*time.Second {
		t.Fatalf("restart took %v (> 30 s)", took)
	}

	// 5. rollback: vpn without WireGuard → nothing left (Retrieve and binapi)
	resp = apply(t, h.svc, &vrxv1.ApplyRequest{TxnId: owner + "-3", DesiredState: doc(t, `{"vpn": {}}`), Subsystems: allDomains})
	if resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("rollback apply: %s", protojson.Format(resp))
	}
	t.Logf("rollback: %s", protojson.Format(resp.GetSummary()))
	got := wgRetrieve(t, h.svc)
	if got.GetVpn() != nil || len(got.GetVrfs()) != 0 {
		t.Fatalf("after rollback Retrieve: %s", protojson.Format(got))
	}
	if wgs, peers := ownedWireguard(t, raw, owner); len(wgs)+len(peers) != 0 {
		t.Fatalf("after rollback VPP still has %v %v", wgs, peers)
	}
	t.Logf("after rollback Retrieve: %s (vpn unset, no wg interface or peer of %s in VPP)", protojson.Format(got), owner)
	h.stop()
}

func TestWireguardHandshakeOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	if os.Getenv("VRX_WG_HANDSHAKE") != "1" {
		t.Skip("opt-in: VRX_WG_HANDSHAKE=1 (a kernel WireGuard peer in the slot netns, reached through a tap)")
	}
	for _, bin := range []string{"ip", "wg"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not installed", bin)
		}
	}
	vpptest.LockLab(t)
	owner := vpptest.Prefix(t) + "wh" // a sub-owner of the slot (F-bonding shares slot 6)
	slot, base := vpptest.Slot(t), vpptest.TableBase(t)
	ns, hostIf, kernIf := "ns-"+owner, owner+"-t0", owner+"-k0"
	vppPort, kernPort := 20000+100*slot+10, 20000+100*slot+11
	t.Logf("VPP %s before", nrestarts(t))
	t.Cleanup(func() { t.Logf("VPP %s after", nrestarts(t)) })
	raw := vpp.Dial(vppSocket(), vpp.ConnOptions{})
	t.Cleanup(raw.Close)
	wctx, wcancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer wcancel()
	if err := raw.WaitConnected(wctx); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput() //nolint:gosec // fixed test argv
		if err != nil {
			t.Fatalf("%s: %v: %s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}
	dir := filepath.Join(t.TempDir(), "state")
	keys := map[string][]byte{"key/" + owner + "-a": wgVector(owner + "_vpp")}
	kernPriv := wgVector(owner + "_kernel")
	kernPub, vppPub := pubOf(t, kernPriv), pubOf(t, keys["key/"+owner+"-a"])

	// the tap (TD-3: its sw_if_index is sanitized on creation) into the slot netns
	_ = exec.Command("ip", "netns", "del", ns).Run()
	run("ip", "netns", "add", ns)
	t.Cleanup(func() { _ = exec.Command("ip", "netns", "del", ns).Run() })
	tapd := tapv2.New(raw, owner+"t") // not the agent's owner: its authoritative `interfaces` would take the tap down
	tap := &tapv2.Tap{Name: "tap" + strconv.Itoa(int(vpptest.LoopbackInstance(t, 60))), Id: vpptest.LoopbackInstance(t, 60), HostIfName: hostIf, HostNamespace: ns,
		HostIp4Prefix: fmt.Sprintf("10.%d.60.2/24", slot), RxRingSize: 256, TxRingSize: 256}
	tmeta, err := tapd.Create(context.Background(), tap)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tapd.Delete(context.Background(), tap, tmeta); err != nil {
			t.Errorf("delete tap: %v", err)
		}
	})
	t.Cleanup(func() {
		h := startWgHost(t, raw, owner, dir, keys)
		defer h.stop()
		if resp := apply(t, h.svc, &vrxv1.ApplyRequest{TxnId: owner + "-cleanup", DesiredState: &vrxv1.DesiredState{}, Subsystems: allDomains}); resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
			t.Errorf("cleanup apply: %v", resp)
		}
		deleteWireguard(t, raw, owner)
	})

	// the kernel peer
	kf := filepath.Join(t.TempDir(), "k")
	if err := os.WriteFile(kf, []byte(b64(kernPriv)), 0o600); err != nil {
		t.Fatal(err)
	}
	run("ip", "-n", ns, "link", "add", kernIf, "type", "wireguard")
	run("ip", "netns", "exec", ns, "wg", "set", kernIf, "private-key", kf, "listen-port", strconv.Itoa(kernPort),
		"peer", vppPub, "allowed-ips", fmt.Sprintf("10.%d.61.1/32", slot), "endpoint", fmt.Sprintf("10.%d.60.1:%d", slot, vppPort), "persistent-keepalive", "1")
	run("ip", "-n", ns, "addr", "add", fmt.Sprintf("10.%d.61.2/24", slot), "dev", kernIf)
	run("ip", "-n", ns, "link", "set", kernIf, "up")

	// the tap's VPP side (a test fixture, not agent configuration: P08 has no tap kind)
	tapIdx := interface_types.InterfaceIndex(tmeta.(iface.Meta).SwIfIndex)
	ifc := interfaces.NewServiceClient(raw)
	tapAddr, err := ip_types.ParseAddressWithPrefix(fmt.Sprintf("10.%d.60.1/24", slot))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ifc.SwInterfaceAddDelAddress(context.Background(), &interfaces.SwInterfaceAddDelAddress{SwIfIndex: tapIdx, IsAdd: true, Prefix: tapAddr}); err != nil {
		t.Fatal(err)
	}
	if _, err := ifc.SwInterfaceSetFlags(context.Background(), &interfaces.SwInterfaceSetFlags{SwIfIndex: tapIdx, Flags: interface_types.IF_STATUS_API_FLAG_ADMIN_UP}); err != nil {
		t.Fatal(err)
	}
	// the agent: the wg interface and its peer
	js := fmt.Sprintf(`{
	  "vpn": {"wireguard": {"interfaces": {"road": {"instance": %[2]d, "listenAddress": "10.%[1]d.60.1", "listenPort": %[3]d,
	    "privateKeyRef": "key/%[4]s-a", "address": ["10.%[1]d.61.1/24"], "routeAllowedIps": true,
	    "peers": {"kernel": {"publicKey": %[5]q, "endpoint": {"address": "10.%[1]d.60.2", "port": %[6]d}, "allowedIps": ["10.%[1]d.61.2/32"]}}}}}}
	}`, slot, base+60, vppPort, owner, kernPub, kernPort)
	h := startWgHost(t, raw, owner, dir, keys)
	defer h.stop()
	if resp := apply(t, h.svc, &vrxv1.ApplyRequest{TxnId: owner + "-hs", DesiredState: doc(t, js), Subsystems: allDomains}); resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("apply: %s", protojson.Format(resp))
	}
	deadline := time.After(20 * time.Second)
	for established := false; !established; {
		select {
		case ev := <-h.events:
			t.Logf("event: kind=%s interface=%s message=%q attributes=%v", ev.GetKind(), ev.GetInterface(), ev.GetMessage(), ev.GetAttributes())
			established = ev.GetKind() == vrxv1.EventKind_EVENT_KIND_WIREGUARD_PEER_CHANGED && ev.GetAttributes()["established"] == "true"
		case <-deadline:
			if s, _ := strconv.Atoi(os.Getenv("VRX_WG_PAUSE")); s > 0 {
				time.Sleep(time.Duration(s) * time.Second) // debugging: objects stay for vppctl
			}
			t.Fatalf("no established event within 20 s; kernel side:\n%s", run("ip", "netns", "exec", ns, "wg", "show", kernIf))
		}
	}
	// traffic through the tunnel, from the kernel peer to VPP's tunnel address
	t.Logf("ping through the tunnel:\n%s", run("ip", "netns", "exec", ns, "ping", "-c", "3", "-W", "2", fmt.Sprintf("10.%d.61.1", slot)))
	t.Logf("kernel peer (wg show, keys are public):\n%s", run("ip", "netns", "exec", ns, "wg", "show", kernIf, "latest-handshakes"))
	st, err := h.svc.WireguardState(context.Background(), &vrxv1.WireguardStateRequest{}, newStatsReader("/run/vpp/stats.sock", h.svc.log))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("WireguardState: %s", protojson.Format(st))
	if len(st.GetInterfaces()) != 1 || len(st.GetInterfaces()[0].GetPeers()) != 1 {
		t.Fatalf("state %v", st)
	}
	p := st.GetInterfaces()[0].GetPeers()[0]
	if !p.GetEstablished() || p.GetLastHandshake() == nil || p.GetEndpoint() != fmt.Sprintf("10.%d.60.2", slot) || st.GetInterfaces()[0].GetRxPackets() == 0 {
		t.Fatalf("peer not established / no traffic: %v", st)
	}
	if s, _ := strconv.Atoi(os.Getenv("VRX_WG_PAUSE")); s > 0 {
		t.Logf("pausing %d s for vppctl evidence", s)
		time.Sleep(time.Duration(s) * time.Second)
	}
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
