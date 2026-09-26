package agent

import (
	"context"
	"crypto/ecdh"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/wireguard"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
)

// Test vectors: SHA-256 of "VRX_TEST_PSK_F-wireguard_<label>" (00-CONTEXT fixture rule), never real keys.
func wgVector(label string) []byte {
	s := sha256.Sum256([]byte("VRX_TEST_PSK_F-wireguard_" + label))
	return s[:]
}

func wgPub(t *testing.T, label string) string {
	t.Helper()
	k, err := ecdh.X25519().NewPrivateKey(wgVector(label))
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(k.PublicKey().Bytes())
}

var allDomains = []string{"interfaces", "vrfs", "routing", "vpn"}

func wgDoc(t *testing.T) string {
	return fmt.Sprintf(`{
	  "vrfs": {"red": {"id": 7001}},
	  "routing": {"static": []},
	  "vpn": {"wireguard": {"interfaces": {"site-a": {
	    "enabled": true, "description": "HQ", "instance": 7001, "vrf": "red", "underlayVrf": "default",
	    "listenAddress": "10.7.8.1", "listenPort": 20710, "privateKeyRef": "key/w7-site-a",
	    "address": ["10.7.9.1/24", "fd00:7::1/64"], "mtu": 1420, "routeAllowedIps": true,
	    "peers": {
	      "b1": {"description": "branch 1", "publicKey": %q, "presharedKeyRef": "psk/w7-b1",
	             "endpoint": {"address": "10.7.8.2", "port": 20711}, "allowedIps": ["10.7.11.0/24", "10.7.10.0/24"], "persistentKeepaliveSec": 25},
	      "b2": {"publicKey": %q, "allowedIps": ["10.7.12.0/24"]}
	    }}}}}
	}`, wgPub(t, "peer1"), wgPub(t, "peer2"))
}

// wgCanonical is what Retrieve reports for wgDoc (sorted lists, explicit scalars).
func wgCanonical(t *testing.T) *vrxv1.VpnConfig {
	t.Helper()
	ds := doc(t, fmt.Sprintf(`{"vpn": {"wireguard": {"interfaces": {"site-a": {
	    "enabled": true, "description": "HQ", "instance": 7001, "vrf": "red", "underlayVrf": "default",
	    "listenAddress": "10.7.8.1", "listenPort": 20710, "privateKeyRef": "key/w7-site-a",
	    "address": ["10.7.9.1/24", "fd00:7::1/64"], "mtu": 1420, "routeAllowedIps": true,
	    "peers": {
	      "b1": {"description": "branch 1", "publicKey": %q, "presharedKeyRef": "psk/w7-b1",
	             "endpoint": {"address": "10.7.8.2", "port": 20711}, "allowedIps": ["10.7.10.0/24", "10.7.11.0/24"], "persistentKeepaliveSec": 25},
	      "b2": {"publicKey": %q, "allowedIps": ["10.7.12.0/24"], "persistentKeepaliveSec": 0}
	    }}}}}}`, wgPub(t, "peer1"), wgPub(t, "peer2")))
	return ds.GetVpn()
}

func putWgSecrets(t *testing.T) {
	t.Helper()
	sec := subsystems.WireguardSecretsFor(testOwner)
	if err := sec.Put("key/w7-site-a", wgVector("itf")); err != nil {
		t.Fatal(err)
	}
	if err := sec.Put("psk/w7-b1", wgVector("psk1")); err != nil {
		t.Fatal(err)
	}
}

func TestWireguardApplyRetrieveStateRollback(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	putWgSecrets(t)
	ctx := context.Background()

	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "wg1", DesiredState: doc(t, wgDoc(t)), Subsystems: allDomains})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	byKey := map[string]*vrxv1.ObjectResult{}
	for _, r := range resp.GetResults() {
		byKey[r.GetKey()] = r
	}
	for key, ptr := range map[string]string{
		"wireguard.interface/wg7001":                 "/vpn/wireguard/interfaces/site-a",
		"wireguard.meta/wg7001":                      "/vpn/wireguard/interfaces/site-a",
		"wireguard.peer/wg7001/" + wgPub(t, "peer1"): "/vpn/wireguard/interfaces/site-a/peers/b1",
		"interface-ip/wg7001/10.7.9.1/24":            "/vpn/wireguard/interfaces/site-a/address/0",
		"interface.mtu/wg7001":                       "/vpn/wireguard/interfaces/site-a/mtu",
		"interface.admin-state/wg7001":               "/vpn/wireguard/interfaces/site-a/enabled",
		"interface-ip.table/wg7001":                  "/vpn/wireguard/interfaces/site-a/vrf",
		"ip.route/7001/10.7.10.0/24":                 "/vpn/wireguard/interfaces/site-a/peers/b1/allowedIps/1",
		"ip.route/7001/10.7.12.0/24":                 "/vpn/wireguard/interfaces/site-a/peers/b2/allowedIps/0",
	} {
		r := byKey[key]
		if r == nil || r.GetPointer() != ptr || r.GetCode() != vrxv1.ObjectResultCode_OBJECT_RESULT_CODE_OK {
			t.Fatalf("%s: result %v (want pointer %s)", key, r, ptr)
		}
	}
	if !v.HasRoute(7001, "10.7.10.0/24") {
		t.Fatal("allowed-IP route missing")
	}

	// Retrieve == canonical desired; the wg interface and the automatic routes are not reported elsewhere
	got, err := s.Retrieve(ctx, &vrxv1.RetrieveRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if want := wgCanonical(t); !proto.Equal(got.GetDesiredState().GetVpn(), want) {
		t.Fatalf("retrieve vpn:\n got %s\nwant %s", protojson.Format(got.GetDesiredState().GetVpn()), protojson.Format(want))
	}
	if _, ok := got.GetDesiredState().GetInterfaces()["wg7001"]; ok || len(got.GetDesiredState().GetRouting().GetStatic()) != 0 {
		t.Fatalf("wg leaves leaked into interfaces/routing: %s", protojson.Format(got.GetDesiredState()))
	}
	if !strings.Contains(strings.Join(got.GetSubsystems(), ","), "vpn") {
		t.Fatalf("subsystems %v", got.GetSubsystems())
	}
	// no secret material and no DF-5 reference in what the agent returns
	if js := protojson.Format(got.GetDesiredState()); strings.Contains(js, "x25519:") || strings.Contains(js, "hmac:") {
		t.Fatalf("retrieve carries a DF-5 reference: %s", js)
	}

	// idempotent
	v.Reset()
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "wg2", DesiredState: doc(t, wgDoc(t)), Subsystems: allDomains})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if len(resp.GetResults()) != 0 {
		t.Fatalf("second apply: %v", resp.GetResults())
	}

	// live state
	v.SetWireguardPeerFlags(1, wireguard.WIREGUARD_PEER_ESTABLISHED)
	st, err := s.WireguardState(ctx, &vrxv1.WireguardStateRequest{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.GetInterfaces()) != 1 {
		t.Fatalf("state %v", st)
	}
	itf := st.GetInterfaces()[0]
	if itf.GetName() != "wg7001" || itf.GetListenPort() != 20710 || itf.GetListenAddress() != "10.7.8.1" || !itf.GetAdminUp() || len(itf.GetPeers()) != 2 {
		t.Fatalf("interface state %v", itf)
	}
	pk, _ := ecdh.X25519().NewPrivateKey(wgVector("itf"))
	if itf.GetPublicKey() != base64.StdEncoding.EncodeToString(pk.PublicKey().Bytes()) {
		t.Fatal("interface public key")
	}
	var established int
	for _, p := range itf.GetPeers() {
		if p.GetEstablished() {
			established++
		}
	}
	if established != 1 {
		t.Fatalf("peers %v", itf.GetPeers())
	}
	if _, err := s.WireguardState(ctx, &vrxv1.WireguardStateRequest{Owner: "someone-else"}, nil); err == nil {
		t.Fatal("a foreign owner must be refused")
	}

	// rollback: the configuration without WireGuard deletes every object (and the metadata)
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "wg3", DesiredState: doc(t, `{"vrfs": {"red": {"id": 7001}}, "routing": {"static": []}, "vpn": {}}`), Subsystems: allDomains})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	got, err = s.Retrieve(ctx, &vrxv1.RetrieveRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetDesiredState().GetVpn() != nil || v.WireguardPeerCount() != 0 || v.HasRoute(7001, "10.7.10.0/24") {
		t.Fatalf("after rollback: %s peers=%d", protojson.Format(got.GetDesiredState()), v.WireguardPeerCount())
	}
	if _, ok := v.InterfaceByName("wg7001"); ok {
		t.Fatal("wg7001 left in VPP")
	}
	kvs, err := s.sched.Retrieve(ctx, scheduler.Only("wireguard.meta"))
	if err != nil || len(kvs) != 0 {
		t.Fatalf("metadata left: %v %v", kvs, err)
	}
}

func TestWireguardWithoutSecretMaterial(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir()) // no secrets: the product agent today (PENDING-secret-channel)
	rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{TxnId: "d", DesiredState: doc(t, wgDoc(t)), Subsystems: allDomains})
	if err != nil {
		t.Fatal(err)
	}
	var warned []string
	for _, e := range rep.GetErrors() {
		if e.GetRule() == "agent.secret-unavailable" {
			if e.GetSeverity() != vrxv1.IssueSeverity_ISSUE_SEVERITY_WARNING {
				t.Fatalf("issue %v", e)
			}
			warned = append(warned, e.GetPointer())
		}
	}
	if strings.Join(warned, ",") != "/vpn/wireguard/interfaces/site-a/peers/b1/presharedKeyRef,/vpn/wireguard/interfaces/site-a/privateKeyRef" {
		t.Fatalf("warnings %v", rep.GetErrors())
	}
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "n1", DesiredState: doc(t, wgDoc(t)), Subsystems: allDomains})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_ROLLED_BACK)
	if !strings.Contains(applyText(resp), "PENDING-secret-channel") {
		t.Fatalf("response %v", resp)
	}
	if _, ok := v.InterfaceByName("wg7001"); ok || v.WireguardPeerCount() != 0 {
		t.Fatal("objects left after the failed apply")
	}

	// only the PSK missing: the peer is never created without it
	sec := subsystems.WireguardSecretsFor(testOwner)
	if err := sec.Put("key/w7-site-a", wgVector("itf")); err != nil {
		t.Fatal(err)
	}
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "n2", DesiredState: doc(t, wgDoc(t)), Subsystems: allDomains})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_ROLLED_BACK)
	if v.WireguardPeerCount() != 0 {
		t.Fatal("a peer was created without its preshared key")
	}
	if !strings.Contains(applyText(resp), "psk/w7-b1") || strings.Contains(applyText(resp), base64.StdEncoding.EncodeToString(wgVector("itf"))) {
		t.Fatal("material in an error")
	}
}

func TestWireguardPeerEventsPublished(t *testing.T) {
	v := coretest.New()
	dir := t.TempDir()
	owned, err := ownertable.Open(dir, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan *vrxv1.Event, 16)
	reg := scheduler.NewRegistry()
	w, err := subsystems.Register(reg, subsystems.Env{Client: v, Owner: testOwner, StateDir: dir, Owned: owned, NetdevKind: fakeNetdevs,
		Publish: func(ev *vrxv1.Event) { events <- ev }})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Connected(ctx) // starts the watcher (an event sink exists)
	sched := scheduler.New(reg, nil)
	s, err := NewService(ServiceConfig{Owner: testOwner, Version: "test", VPP: v, Scheduler: sched, StateDir: dir, BeforeTxn: w.BeforeTxn, NetdevKind: w.NetdevKind()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	putWgSecrets(t)
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "e1", DesiredState: doc(t, wgDoc(t)), Subsystems: allDomains})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)

	deadline := time.Now().Add(3 * time.Second)
	for !subsystems.WireguardObserverFor(testOwner).Active() {
		if time.Now().After(deadline) {
			t.Fatal("watcher not active")
		}
		time.Sleep(10 * time.Millisecond)
	}
	v.Emit(&wireguard.WireguardPeerEvent{PeerIndex: 1, Flags: wireguard.WIREGUARD_PEER_ESTABLISHED})
	select {
	case ev := <-events:
		if ev.GetKind() != vrxv1.EventKind_EVENT_KIND_WIREGUARD_PEER_CHANGED || ev.GetInterface() != "wg7001" ||
			ev.GetAttributes()["established"] != "true" || ev.GetAttributes()["dead"] != "false" || ev.GetAttributes()["public_key"] == "" {
			t.Fatalf("event %v", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no event published")
	}
	st, err := s.WireguardState(context.Background(), &vrxv1.WireguardStateRequest{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var seen bool
	for _, p := range st.GetInterfaces()[0].GetPeers() {
		if p.GetPeerIndex() == 1 && p.GetLastHandshake() != nil {
			seen = true
		}
	}
	if !seen || !st.GetEventsActive() {
		t.Fatalf("last handshake not reported: %v", st)
	}
}

// applyText is the response's message and per-object messages (where a failure is reported).
func applyText(r *vrxv1.ApplyResponse) string {
	parts := []string{r.GetMessage()}
	for _, o := range r.GetResults() {
		parts = append(parts, o.GetKey()+": "+o.GetMessage())
	}
	return strings.Join(parts, "\n")
}
