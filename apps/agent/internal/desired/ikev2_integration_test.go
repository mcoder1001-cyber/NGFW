package desired

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/strongswan/govici/vici"
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/ikev2"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/testpeer/strongswan"
	"ngfw/agent/internal/testpeer/strongswan/swantest"
	"ngfw/agent/internal/trafficbtest"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestIKEv2NativePackets runs only in the disposable VPP namespace. It uses a
// stock strongSwan as an interoperability peer, never as the NGFW implementation.
func TestIKEv2NativePackets(t *testing.T) {
	if os.Getenv("NGFW_INTEGRATION") != "1" || os.Getenv("NGFW_DISPOSABLE_VPP") != "1" {
		t.Skip("requires disposable VPP")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 210*time.Second)
	defer cancel()
	run := func(args ...string) string {
		t.Helper()
		//nolint:gosec // Disposable lab fixture uses an explicitly authorized local executable or evidence directory.
		out, e := exec.CommandContext(ctx, args[0], args[1:]...).CombinedOutput()
		if e != nil {
			t.Fatalf("%v: %v: %s", args, e, out)
		}
		return string(out)
	}
	productBinary := os.Getenv("NGFW_NATIVE_AGENT_BIN")
	production := productBinary != ""
	cli := func(cmd string) string { return run("vppctl", "-s", vpptest.CLISocket(), cmd) }
	if os.Getenv("NGFW_NATIVE_FAST_DPD") == "1" {
		cli("ikev2 set liveness 1 3")
	}
	h, e := swantest.New("w8", swantest.FindRoot("w10"))
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if err := h.Close(); err != nil {
			t.Error(err)
		}
	}()
	peer, e := h.AddNetNS(ctx, "native-peer")
	if e != nil {
		t.Fatal(e)
	}
	lan, e := h.AddNetNS(ctx, "native-lan")
	if e != nil {
		t.Fatal(e)
	}
	for _, pair := range []struct{ host, peer, ns, addr string }{{"w8nwan", "w8npeer", peer, "198.18.8.2/24"}, {"w8nlan", "w8nhost", lan, "198.18.81.2/24"}} {
		run("ip", "link", "add", pair.host, "type", "veth", "peer", "name", pair.peer)
		host := pair.host
		//nolint:gosec // Disposable fixture removes only its locally created named veth.
		t.Cleanup(func() { _ = exec.Command("ip", "link", "del", host).Run() })
		run("ip", "link", "set", pair.peer, "netns", pair.ns)
		run("ip", "link", "set", pair.host, "up")
		run("ip", "-n", pair.ns, "link", "set", pair.peer, "up")
		run("ip", "-n", pair.ns, "addr", "add", pair.addr, "dev", pair.peer)
		run("ethtool", "-K", pair.host, "tx", "off", "rx", "off", "tso", "off", "gso", "off", "gro", "off")
		run("ip", "netns", "exec", pair.ns, "ethtool", "-K", pair.peer, "tx", "off", "rx", "off", "tso", "off", "gso", "off", "gro", "off")
		if !production {
			cli("create host-interface name " + pair.host)
			cli("set interface state host-" + pair.host + " up")
		}
	}
	if !production {
		cli("set interface ip address host-w8nwan 198.18.8.1/24")
		cli("set interface ip address host-w8nlan 198.18.81.1/24")
		cli("create ipip tunnel src 198.18.8.1 dst 198.18.8.2 instance 8001")
		cli("set interface state ipip8001 down")
		cli("set interface tag ipip8001 w8:ipip8001")
		cli("set interface ip address ipip8001 198.18.83.1/30")
		cli("ip route add 198.18.82.0/24 via ipip8001")
	}
	run("ip", "-n", peer, "addr", "add", "198.18.82.2/24", "dev", "lo")
	run("ip", "-n", peer, "route", "add", "198.18.81.0/24", "via", "198.18.8.1")
	run("ip", "-n", lan, "route", "add", "198.18.82.0/24", "via", "198.18.81.1")
	pcap := filepath.Join(t.TempDir(), "native-underlay.pcap")
	//nolint:gosec // Disposable lab fixture uses an explicitly authorized local executable or evidence directory.
	capture := exec.Command("tcpdump", "-U", "-n", "-i", "w8nwan", "-w", pcap)
	if err := capture.Start(); err != nil {
		t.Fatal(err)
	}
	stopped := false
	stopCapture := func() {
		if !stopped {
			_ = capture.Process.Signal(syscall.SIGINT)
			_ = capture.Wait()
			stopped = true
		}
	}
	defer stopCapture()
	time.Sleep(200 * time.Millisecond)
	//nolint:gosec // Disposable lab fixture uses an explicitly authorized local executable or evidence directory.
	//nolint:gosec // Disposable fixture uses its own generated namespace with fixed executable and arguments.
	if out, e := exec.CommandContext(ctx, "ip", "netns", "exec", lan, "ping", "-c", "1", "-W", "1", "198.18.82.2").CombinedOutput(); e == nil {
		t.Fatalf("traffic escaped before SA installation: %s", out)
	}
	conn := vpp.Dial(vpptest.APISocket(), vpp.ConnOptions{})
	defer conn.Close()
	if e := conn.WaitConnected(ctx); e != nil {
		t.Fatal(e)
	}
	key, e := vpn.LoadOrCreateKeyFile(filepath.Join(t.TempDir(), "key"))
	if e != nil {
		t.Fatal(e)
	}
	material := make([]byte, 32)
	rand.Read(material)
	if trafficbtest.Enabled() {
		text := []byte(fmt.Sprintf("%x", material))
		vpn.Zero(material)
		material = text
	}
	defer vpn.Zero(material)
	resolver := vpn.NewMapResolver(key, material)
	env := IKEv2Env{SecretRef: func(context.Context, string) (string, error) { return key.Ref(material), nil }}
	ds := nativeDoc(t)
	tun := ds.Vpn.Ipsec.Tunnels["site"]
	tun.LocalTs = []string{"198.18.81.0/24"}
	tun.RemoteTs = []string{"198.18.82.0/24"}
	sink := &sink{}
	IKEv2(sink, ds, inVPN, env)
	if len(sink.errs) > 0 {
		t.Fatal(sink.errs)
	}
	profile := sink.value(scheduler.Join(ikev2.ProfileName, "site")).(*vpnpb.Ikev2Profile)
	initiator := os.Getenv("NGFW_NATIVE_INITIATOR") == "1"
	if initiator {
		profile.Responder = &vpnpb.Ikev2Responder{Interface: "host-w8nwan", Address: "198.18.8.2"}
	}
	desc := ikev2.NewProfile(ikev2.Config{Client: conn, Owner: "w8", Secrets: resolver, Keys: key})
	var restControl *trafficbtest.Client
	var product ngfwv1.DataplaneClient
	var productApply func(string)
	var productPartial func(string)
	var productRestart func()
	if production {
		extra := &ngfwv1.DesiredState{}
		if e := protojson.Unmarshal([]byte(`{"interfaces":{"host-w8nwan":{"enabled":true,"ipv4":["198.18.8.1/24"]},"host-w8nlan":{"enabled":true,"ipv4":["198.18.81.1/24"]}},"routing":{"static":[{"vrf":"default","prefix":"198.18.82.0/24","nextHops":[{"address":"198.18.83.2","interface":"site"}]}]}}`), extra); e != nil {
			t.Fatal(e)
		}
		ds.Interfaces = extra.Interfaces
		ds.Routing = extra.Routing
		work := t.TempDir()
		socket := filepath.Join(work, "agent.sock")
		var process *exec.Cmd
		processEnv := append(os.Environ(), "NGFW_AGENT_SOCKET="+socket, "NGFW_OWNER=w8", "NGFW_GLOBALS_OWNER=0", "NGFW_VPP_TABLE_BASE=8000", "NGFW_AGENT_STATE_DIR="+filepath.Join(work, "state"), "NGFW_METRICS_ADDR=127.0.0.1:0", "NGFW_SOCKET_GROUP=root")
		//nolint:gosec // Disposable fixture uses its own private directory and generated namespace with fixed command arguments.
		logfile, e := os.Create(filepath.Join(work, "agent.log"))
		if e != nil {
			t.Fatal(e)
		}
		defer func() { _ = logfile.Close() }()
		t.Cleanup(func() {
			if t.Failed() {
				//nolint:gosec // Disposable fixture uses its own private directory and generated namespace with fixed command arguments.
				data, _ := os.ReadFile(filepath.Join(work, "agent.log"))
				t.Log(string(data))
			}
		})
		startProduct := func() {
			//nolint:gosec // Disposable lab fixture uses an explicitly authorized local executable or evidence directory.
			process = exec.CommandContext(ctx, productBinary)
			process.Env = processEnv
			process.Stdout = logfile
			process.Stderr = logfile
			if e := process.Start(); e != nil {
				t.Fatal(e)
			}
		}
		startProduct()
		defer func() { _ = process.Process.Signal(syscall.SIGTERM); _ = process.Wait() }()
		cc, e := grpc.NewClient("unix://"+socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if e != nil {
			t.Fatal(e)
		}
		defer func() { _ = cc.Close() }()
		product = ngfwv1.NewDataplaneClient(cc)
		ready := time.Now().Add(20 * time.Second)
		for {
			_, e = product.Health(ctx, &ngfwv1.HealthRequest{})
			if e == nil {
				break
			}
			if time.Now().After(ready) {
				t.Fatal("production agent readiness failed", e)
			}
			time.Sleep(200 * time.Millisecond)
		}
		productRestart = func() {
			t.Helper()
			info, e := logfile.Stat()
			if e != nil {
				t.Fatal(e)
			}
			offset := info.Size()
			_ = process.Process.Signal(syscall.SIGTERM)
			if e := process.Wait(); e != nil {
				t.Fatal(e)
			}
			startProduct()
			deadline := time.Now().Add(20 * time.Second)
			for {
				_, e := product.Retrieve(ctx, &ngfwv1.RetrieveRequest{Owner: "w8"})
				//nolint:gosec // Disposable fixture uses its own private directory and generated namespace with fixed command arguments.
				data, _ := os.ReadFile(filepath.Join(work, "agent.log"))
				resynced := int64(len(data)) > offset && bytes.Contains(data[offset:], []byte("\"msg\":\"resync finished\""))
				if e == nil && resynced {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("production restart/resync readiness failed", e)
				}
				time.Sleep(200 * time.Millisecond)
			}
		}
		productSecrets := map[string][]byte{"psk/site": append([]byte(nil), material...)}
		if trafficbtest.Enabled() {
			restControl = trafficbtest.New(t, socket, "w8", os.Getenv("NGFW_TRAFFIC_B_PHASE"), process.Process.Pid)
			defer restControl.Close(t)
		}
		productApply = func(txn string) {
			t.Helper()
			if restControl != nil {
				restControl.Apply(t, txn, ds, productSecrets)
				return
			}
			result, e := product.Apply(ctx, &ngfwv1.ApplyRequest{TxnId: txn, DesiredState: ds, SecretBundle: &ngfwv1.SecretBundle{Values: productSecrets}})
			if e != nil {
				t.Fatal(e)
			}
			if result.Status != ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED {
				t.Fatalf("production Apply failed: %v", result)
			}
		}
		productPartial = func(txn string) {
			t.Helper()
			partial := proto.Clone(ds).(*ngfwv1.DesiredState)
			partial.Vpn = nil
			result, e := product.Apply(ctx, &ngfwv1.ApplyRequest{TxnId: txn, DesiredState: partial, Subsystems: []string{"tunnels"}})
			if e == nil && result.Status == ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED {
				t.Log("partial Apply safely retained stored VPN context")
			}
			if state := cli("show interface ipip8001"); !strings.Contains(state, " down ") {
				t.Fatal("partial Apply escaped admin guard", state)
			}
			t.Log("partial tunnels-only Apply refused protected admin-up: " + txn)
		}
		productApply("native-initial")
		productPartial("native-partial-before-sa")
		//nolint:gosec // Disposable fixture uses its own generated namespace with fixed executable and arguments.
		if out, e := exec.CommandContext(ctx, "ip", "netns", "exec", lan, "ping", "-c", "1", "-W", "1", "198.18.82.2").CombinedOutput(); e == nil {
			t.Fatalf("traffic escaped production Apply before SA: %s", out)
		}
	} else {
		meta, e := desc.Create(ctx, profile)
		if e != nil {
			t.Fatal(e)
		}
		defer func() {
			if err := desc.Delete(context.Background(), profile, meta); err != nil {
				t.Error(err)
			}
		}()
	}
	action := func(operation string, ikeSpi uint64, childSpi uint32) error {
		if !production {
			switch operation {
			case "initiate":
				return ikev2.InitiateSAInit(ctx, conn, "w8", "site")
			case "rekey":
				return ikev2.RekeyChildSA(ctx, conn, childSpi)
			case "delete-sa":
				return ikev2.DeleteIKESA(ctx, conn, ikeSpi)
			}
		}
		stream, e := product.Action(ctx, &ngfwv1.ActionRequest{Action: &ngfwv1.ActionRequest_Ikev2{Ikev2: &ngfwv1.Ikev2Action{Tunnel: "site", Operation: operation, IkeSpi: ikeSpi, ChildSpi: childSpi}}})
		if e != nil {
			return e
		}
		for {
			_, e = stream.Recv()
			if e == io.EOF {
				return nil
			}
			if e != nil {
				return e
			}
		}
	}
	peerDoc := proto.Clone(ds).(*ngfwv1.DesiredState)
	pt := peerDoc.Vpn.Ipsec.Tunnels["site"]
	peerDoc.Vpn.Ipsec.Tunnels = map[string]*ngfwv1.IpsecTunnel{"w8-site": pt}
	pt.Engine = proto.String("strongswan")
	pt.RouteBased = nil
	pt.LocalAddr = proto.String("198.18.8.2")
	pt.RemoteAddr = proto.String("198.18.8.1")
	pt.LocalId = proto.String("@remote.test")
	pt.RemoteId = proto.String("@local.test")
	pt.LocalTs = tun.RemoteTs
	pt.RemoteTs = tun.LocalTs
	pt.StartAction = proto.String("start")
	if initiator {
		pt.StartAction = proto.String("none")
	}
	pt.Mobike = proto.Bool(false)
	newPeerRenderer := func() *strongswan.Renderer {
		return strongswan.New(strongswan.WithPaths(h.Paths("native-peer")), strongswan.WithOwnerPrefix("w8"), strongswan.WithSecretResolver(strongswan.SecretResolverFunc(func(context.Context, string) ([]byte, error) { return append([]byte(nil), material...), nil })), strongswan.WithDaemonConfig(strongswan.DaemonConfig{Plugins: strongswan.DefaultPlugins(), LogLevel: 1, QuietJournal: true}))
	}
	r := newPeerRenderer()
	files, e := r.Render(ctx, peerDoc)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = h.Start(ctx, "native-peer", peer, files[h.Paths("native-peer").StrongswanConf].Content); e != nil {
		t.Fatal(e)
	}
	if e = r.Apply(ctx, files); e != nil {
		t.Fatal(e)
	}
	if initiator {
		if e := action("initiate", 0, 0); e != nil {
			t.Fatal(e)
		}
	}
	deadline := time.Now().Add(30 * time.Second)
	var sas []ikev2.SAState
	for time.Now().Before(deadline) {
		sas, e = ikev2.SAs(ctx, conn, "w8")
		if e == nil && len(sas) > 0 && len(sas[0].Children) > 0 {
			break
		}
		time.Sleep(time.Second)
	}
	if len(sas) == 0 || len(sas[0].Children) == 0 {
		t.Log(cli("show errors"))
		t.Log(cli("show interface"))
		log, _ := os.ReadFile(h.Paths("native-peer").LogFile)
		t.Fatalf("native negotiation failed: %v; %s; peer log %s", e, cli("show ikev2 sa"), log)
	}
	t.Logf("native IKE negotiated %s, child SPI %#x/%#x", sas[0].State, sas[0].Children[0].ISPI, sas[0].Children[0].RSPI)
	if state := cli("show interface ipip8001"); !strings.Contains(state, " up ") {
		t.Fatal("IKE did not enable protected IPIP", state)
	}
	//nolint:gosec // Disposable fixture uses its own generated namespace with fixed executable and arguments.
	out, pe := exec.CommandContext(ctx, "ip", "netns", "exec", lan, "ping", "-c", "5", "-W", "2", "198.18.82.2").CombinedOutput()
	if pe != nil {
		t.Log(cli("show errors"))
		t.Log(cli("show interface"))
		t.Log(cli("show ipsec sa"))
		t.Log(cli("show ipsec protect"))
		log, _ := os.ReadFile(h.Paths("native-peer").LogFile)
		t.Log(string(log))
		t.Fatal(string(out))
	}
	t.Log(string(out))
	t.Log(run("ip", "netns", "exec", peer, "ping", "-I", "198.18.82.2", "-c", "5", "-W", "2", "198.18.81.2"))
	//nolint:gosec // Disposable fixture uses its own private directory and generated namespace with fixed command arguments.
	server := exec.CommandContext(ctx, "ip", "netns", "exec", peer, "python3", "-c", `import socket
s=socket.socket();s.bind(('198.18.82.2',28081));s.listen(1)
c,_=s.accept();data=b'x'*1048576;c.sendall(data);c.shutdown(socket.SHUT_WR);c.close();s.close()`)
	if e := server.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() {
		if server.ProcessState == nil {
			_ = server.Process.Kill()
			_ = server.Wait()
		}
	}()
	time.Sleep(200 * time.Millisecond)
	t.Log(run("ip", "netns", "exec", lan, "python3", "-c", `import socket
s=socket.create_connection(('198.18.82.2',28081),10);data=bytearray()
while True:
 chunk=s.recv(65536)
 if not chunk:break
 data.extend(chunk)
assert data==b'x'*1048576
print('TCP exact 1048576 bytes passed')`))
	if e := server.Wait(); e != nil {
		t.Fatal(e)
	}
	counters, e := ikev2.SACounters(ctx, conn, vpptest.StatsSocket(), sas)
	if e != nil {
		t.Fatal(e)
	}
	for _, spi := range []uint32{sas[0].Children[0].ISPI, sas[0].Children[0].RSPI} {
		v, ok := counters[spi]
		if !ok || v.Packets == 0 || v.Bytes == 0 {
			t.Fatalf("missing actual forwarding counter for SPI %#x", spi)
		}
		t.Logf("SPI %#x: packets=%d bytes=%d", spi, v.Packets, v.Bytes)
	}
	if production {
		state, e := product.IpsecState(ctx, &ngfwv1.IpsecStateRequest{Owner: "w8", Tunnels: []string{"site"}})
		if e != nil {
			t.Fatal(e)
		}
		if len(state.Sas) != 1 || len(state.Sas[0].Children) != 1 {
			t.Fatal("production native state omitted owned SA")
		}
		if state.Sas[0].Initiator != initiator {
			t.Fatal("production state reported incorrect native IKE role")
		}
		ch := state.Sas[0].Children[0]
		if ch.BytesIn == 0 || ch.BytesOut == 0 || ch.PacketsIn == 0 || ch.PacketsOut == 0 {
			t.Fatal("production state omitted measured counters")
		}
		raw, e := proto.Marshal(state)
		if e != nil {
			t.Fatal(e)
		}
		if bytes.Contains(raw, material) {
			t.Fatal("production state echoed key material")
		}
		if e := action("rekey", 0, 0); e == nil {
			t.Fatal("accepted foreign CHILD SPI")
		}
		if e := action("delete-sa", 1, 0); e == nil {
			t.Fatal("accepted foreign IKE SPI")
		}
		if !initiator {
			if e := action("rekey", 0, sas[0].Children[0].ISPI); status.Code(e) != codes.FailedPrecondition {
				t.Fatal("native responder local rekey must explicitly refuse", e)
			}
		}
		t.Log("production native state measured inbound/outbound counters without keys; foreign SPI actions refused")
	}
	if got := cli("show ipsec protect"); !strings.Contains(got, "ipip8001") {
		t.Fatal("no tunnel protection", got)
	}
	if production {
		productApply("native-active-reconcile")
		if state := cli("show interface ipip8001"); !strings.Contains(state, " up ") {
			t.Fatal("production reconciliation lowered active protected tunnel", state)
		}
		t.Log(run("ip", "netns", "exec", lan, "ping", "-c", "3", "-W", "2", "198.18.82.2"))
		t.Log("production Apply reconciliation retained active SA and traffic")
		productRestart()
		next, e := ikev2.SAs(ctx, conn, "w8")
		if e != nil || len(next) != 1 || next[0].ISPI != sas[0].ISPI || len(next[0].Children) != 1 || next[0].Children[0].ISPI != sas[0].Children[0].ISPI {
			t.Fatal("agent restart changed active SA", e)
		}
		if state := cli("show interface ipip8001"); !strings.Contains(state, " up ") {
			t.Fatal("initial Service.Resync lowered active tunnel", state)
		}
		t.Log(run("ip", "netns", "exec", lan, "ping", "-c", "3", "-W", "2", "198.18.82.2"))
		t.Log("production agent restart reopened sealed secrets, initial Service.Resync retained identical active IKE/CHILD SPIs and protected traffic")
	} else {
		kvs, e := desc.Retrieve(ctx)
		if e != nil || len(kvs) != 1 || !proto.Equal(kvs[0].Value, profile) {
			t.Fatalf("native profile read-back differs: %v, %v", e, kvs)
		}
	}
	if initiator {
		if e := action("rekey", 0, sas[0].Children[0].ISPI); e != nil {
			t.Fatal(e)
		}
	} else {
		vc, e := strongswan.DialVICI(ctx, h.Paths("native-peer").ViciSocket)
		if e != nil {
			t.Fatal(e)
		}
		msg := vici.NewMessage()
		_ = msg.Set("child", "w8-site")
		if _, e = vc.Call(ctx, "rekey", msg); e != nil {
			t.Fatal(e)
		}
		_ = vc.Close()
	}
	old := sas[0].Children[0].ISPI
	deadline = time.Now().Add(15 * time.Second)
	changed := false
	for time.Now().Before(deadline) {
		next, e := ikev2.SAs(ctx, conn, "w8")
		if e == nil && len(next) > 0 && len(next[0].Children) > 0 && next[0].Children[0].ISPI != old {
			sas = next
			changed = true
			break
		}
		time.Sleep(time.Second)
	}
	if !changed {
		t.Fatal("rekey did not replace CHILD SPI")
	}
	t.Log("rekey changed CHILD SPI")
	t.Log(run("ip", "netns", "exec", lan, "ping", "-c", "3", "-W", "2", "198.18.82.2"))
	var savedRoutes []*ngfwv1.StaticRoute
	if production {
		savedRoutes = ds.Routing.Static
		ds.Routing.Static = nil
		productApply("native-route-withdraw")
	} else {
		cli("ip route del 198.18.82.0/24 via ipip8001")
	}
	//nolint:gosec // Disposable fixture uses its own generated namespace with fixed executable and arguments.
	if out, e := exec.CommandContext(ctx, "ip", "netns", "exec", lan, "ping", "-c", "1", "-W", "1", "198.18.82.2").CombinedOutput(); e == nil {
		t.Fatalf("traffic continued after route withdrawal: %s", out)
	}
	if production {
		ds.Routing.Static = savedRoutes
		productApply("native-route-restore")
	} else {
		cli("ip route add 198.18.82.0/24 via ipip8001")
	}
	t.Log(run("ip", "netns", "exec", lan, "ping", "-c", "2", "-W", "2", "198.18.82.2"))
	if os.Getenv("NGFW_NATIVE_PEER_LOSS") == "1" {
		oldIke := sas[0].ISPI
		if e := h.Crash("native-peer"); e != nil {
			t.Fatal(e)
		}
		if e := h.FlushXfrm(ctx, peer); e != nil {
			t.Fatal(e)
		}
		t.Log("peer crashed; awaiting native VPP default liveness expiry (30 seconds, 3 retries)")
		deadline := time.Now().Add(110 * time.Second)
		expired := false
		for time.Now().Before(deadline) {
			next, e := ikev2.SAs(ctx, conn, "w8")
			if e == nil && len(next) == 0 {
				expired = true
				break
			}
			time.Sleep(time.Second)
		}
		if !expired {
			t.Fatal("native liveness did not remove lost-peer SA")
		}
		t.Log("default VPP DPD removed crashed peer SA")
		if state := cli("show interface ipip8001"); !strings.Contains(state, " down ") {
			t.Fatal("peer-loss expiry did not lower protected IPIP", state)
		}
		//nolint:gosec // Disposable fixture uses its own generated namespace with fixed executable and arguments.
		if out, e := exec.CommandContext(ctx, "ip", "netns", "exec", lan, "ping", "-c", "1", "-W", "1", "198.18.82.2").CombinedOutput(); e == nil {
			t.Fatalf("traffic escaped after peer loss: %s", out)
		}
		if _, e := h.Start(ctx, "native-peer", peer, files[h.Paths("native-peer").StrongswanConf].Content); e != nil {
			t.Fatal(e)
		}
		r = newPeerRenderer()
		if e := r.Apply(ctx, files); e != nil {
			t.Fatal(e)
		}
		deadline = time.Now().Add(30 * time.Second)
		recovered := false
		for time.Now().Before(deadline) {
			next, e := ikev2.SAs(ctx, conn, "w8")
			if e == nil && len(next) == 1 && len(next[0].Children) > 0 && next[0].ISPI != oldIke {
				sas = next
				recovered = true
				break
			}
			time.Sleep(time.Second)
		}
		if !recovered {
			next, e := ikev2.SAs(ctx, conn, "w8")
			t.Logf("recovery SA snapshot: %v error %v", next, e)
			t.Log(cli("show errors"))
			t.Log(cli("show interface"))
			log, _ := os.ReadFile(h.Paths("native-peer").LogFile)
			t.Log(string(log))
			t.Fatal("native tunnel did not recover after peer restart")
		}
		t.Log(run("ip", "netns", "exec", lan, "ping", "-c", "3", "-W", "2", "198.18.82.2"))
		t.Log(run("ip", "netns", "exec", peer, "ping", "-I", "198.18.82.2", "-c", "3", "-W", "2", "198.18.81.2"))
		t.Log("default VPP DPD cleared crashed peer and lowered IPIP; peer restart and automatic native retry restored bidirectional encrypted traffic")
	}
	if e := action("delete-sa", sas[0].ISPI, 0); e != nil {
		t.Fatal(e)
	}
	time.Sleep(time.Second)
	if state := cli("show interface ipip8001"); !strings.Contains(state, " down ") {
		t.Fatal("IPIP not fail-closed after SA deletion", state)
	}
	//nolint:gosec // Disposable fixture uses its own private directory and generated namespace with fixed command arguments.
	if out, e := exec.CommandContext(ctx, "ip", "netns", "exec", lan, "ping", "-c", "2", "-W", "1", "198.18.82.2").CombinedOutput(); e == nil {
		t.Fatalf("traffic escaped after SA deletion: %s", out)
	}
	if production {
		productPartial("native-partial-after-delete")
	}
	stopCapture()
	if evidence := os.Getenv("NGFW_NATIVE_EVIDENCE_DIR"); evidence != "" {
		//nolint:gosec // Disposable lab fixture uses an explicitly authorized local executable or evidence directory.
		if e := os.MkdirAll(evidence, 0700); e != nil {
			t.Fatal(e)
		}
		//nolint:gosec // Read only the capture file created by this fixture inside t.TempDir.
		data, e := os.ReadFile(pcap)
		if e != nil {
			t.Fatal(e)
		}
		name := "responder-underlay.pcap"
		if initiator {
			name = "initiator-underlay.pcap"
		}
		//nolint:gosec // Disposable lab fixture uses an explicitly authorized local executable or evidence directory.
		if e := os.WriteFile(filepath.Join(evidence, name), data, 0600); e != nil {
			t.Fatal(e)
		}
	}
	plain := run("tcpdump", "-n", "-r", pcap, "ip proto 4")
	// tcpdump's stderr contains the file description; packet records have an IP
	// source/destination line. No unencrypted IPIP may exist in any lifecycle phase.
	if strings.Contains(plain, " > ") {
		t.Fatal("plaintext IPIP observed on underlay")
	}
	esp := run("tcpdump", "-n", "-r", pcap, "ip proto 50")
	if !strings.Contains(esp, "ESP") {
		t.Fatal("no ESP in underlay capture")
	}
	if production {
		if restControl != nil {
			restControl.Close(t)
		}
		if restControl == nil {
			result, err := product.Apply(ctx, &ngfwv1.ApplyRequest{TxnId: "native-owned-rollback", DesiredState: &ngfwv1.DesiredState{}, Subsystems: []string{"vpn", "tunnels", "routing", "interfaces", "vrfs"}})
			if err != nil || result.GetStatus() != ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED {
				t.Fatal("production owned rollback failed", err, result.GetStatus())
			}
		}

		actual, err := product.Retrieve(ctx, &ngfwv1.RetrieveRequest{Owner: "w8"})
		if err != nil {
			t.Fatal("post-rollback Retrieve failed", err)
		}
		state := actual.GetDesiredState()
		if len(state.GetInterfaces()) != 0 || len(state.GetTunnels().GetIpip()) != 0 || len(state.GetRouting().GetStatic()) != 0 || len(state.GetVpn().GetIpsec().GetTunnels()) != 0 {
			t.Fatal("owned configuration remains after production rollback")
		}
		profiles, err := desc.Retrieve(ctx)
		if err != nil || len(profiles) != 0 {
			t.Fatal("owned IKE profile remains after rollback", err)
		}
		if got := cli("show ipsec protect"); strings.Contains(got, "ipip8001") {
			t.Fatal("owned tunnel protection remains after rollback")
		}
		if got := cli("show interface"); strings.Contains(got, "ipip8001") || strings.Contains(got, "host-w8n") {
			t.Fatal("owned VPP tunnel/interface remains after rollback")
		}
		t.Log("production owned rollback removed profiles, protection, routes, tunnels and interfaces; Retrieve empty before disposable VPP shutdown")
	}
	t.Log("underlay capture: ESP present; plaintext IPIP absent before negotiation, during ICMP/TCP/rekey, and after SA deletion")
	t.Log("route withdrawal and recovery passed; SA deletion lowered IPIP and stopped traffic while route remained")
}
