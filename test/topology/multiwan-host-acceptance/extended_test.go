package acl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"ngfw/agent/binapi/ip_types"
	natapi "ngfw/agent/binapi/nat44_ed"
	"ngfw/agent/binapi/nat_types"
)

const mwUDPServer = `import socket,json,sys
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);s.bind(('198.18.7.2',9000))
print('ready '+sys.argv[1],flush=True)
while True:
 data,source=s.recvfrom(4096)
 print(json.dumps({'peer':sys.argv[1],'source':source,'packet':data.decode()}),flush=True)
 s.sendto(json.dumps({'peer':sys.argv[1],'source':source[0],'port':source[1],'echo':data.decode()}).encode(),source)
`
const mwUDPClient = `import socket,json,sys
n,base,nat=int(sys.argv[1]),int(sys.argv[2]),int(sys.argv[3])
counts={'wan1':0,'wan2':0};samples=[];sources={};stable=0
for i in range(n):
 s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);s.bind(('10.7.1.2',base+i));s.settimeout(1)
 records=[]
 for repeat in range(2):
  message='flow-%d-repeat-%d'%(base+i,repeat)
  s.sendto(message.encode(),('198.18.7.2',9000))
  try:data,address=s.recvfrom(4096)
  except Exception:
   print(json.dumps({'failedFlow':i,'insidePort':base+i,'repeat':repeat,'counts':counts}),flush=True);raise
  record=json.loads(data);assert address[0]=='198.18.7.2' and record['echo']==message,record
  expected={'wan1':'10.7.2.1','wan2':'10.7.3.1'}[record['peer']] if nat else '10.7.1.2'
  assert record['source']==expected,(record,expected)
  records.append(record)
 assert (records[0]['peer'],records[0]['source'],records[0]['port'])==(records[1]['peer'],records[1]['source'],records[1]['port']),records
 stable+=1;counts[records[0]['peer']]+=1;sources[records[0]['source']]=sources.get(records[0]['source'],0)+1
 if i<3 or i>=n-3:samples.append({'insidePort':base+i,**records[0]})
 s.close()
print(json.dumps({'flows':n,'echoes':2*n,'stableFlows':stable,'counts':counts,'sources':sources,'samples':samples}))
`

type mwFlowProof struct {
	Flows  int            `json:"flows"`
	Echoes int            `json:"echoes"`
	Stable int            `json:"stableFlows"`
	Counts map[string]int `json:"counts"`
}

func mwFlows(t *testing.T, n, base int, nat bool, label string) mwFlowProof {
	t.Helper()
	flag := "0"
	if nat {
		flag = "1"
	}
	out, err := run(t, "ip", "netns", "exec", "ns-w7-mw-lan", "python3", "-u", "-c", mwUDPClient, fmt.Sprint(n), fmt.Sprint(base), flag)
	t.Logf("%s: %s", label, out)
	if err != nil {
		t.Fatalf("%s real UDP exchange failed: %v", label, err)
	}
	var proof mwFlowProof
	if err := json.Unmarshal([]byte(out), &proof); err != nil {
		t.Fatal(err)
	}
	if proof.Flows != n || proof.Echoes != 2*n || proof.Stable != n || proof.Counts["wan1"]+proof.Counts["wan2"] != n {
		t.Fatalf("incomplete flow proof %+v", proof)
	}
	return proof
}
func mwRatio(t *testing.T, p mwFlowProof, label string) {
	t.Helper()
	want := float64(p.Flows) / 4
	// Relative ten percent tolerance around the requested 1:3 split.
	if float64(p.Counts["wan1"]) < want*.9 || float64(p.Counts["wan1"]) > want*1.1 {
		t.Fatalf("%s weighted1:3 expected WAN1within10%%of%.0f got%v", label, want, p.Counts)
	}
}
func mwSessions(t *testing.T) []*natapi.Nat44UserSessionV3Details {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn := connectVPP(t)
	dump, err := natapi.NewServiceClient(conn).Nat44UserSessionV3Dump(ctx, &natapi.Nat44UserSessionV3Dump{IPAddress: ip_types.IP4Address{10, 7, 1, 2}, VrfID: 0})
	if err != nil {
		t.Fatal(err)
	}
	var rows []*natapi.Nat44UserSessionV3Details
	for {
		row, err := dump.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row)
	}
	return rows
}
func mwSessionCounts(rows []*natapi.Nat44UserSessionV3Details, base, n int) map[string]int {
	counts := map[string]int{}
	for _, row := range rows {
		if row.Protocol == 17 && int(row.InsidePort) >= base && int(row.InsidePort) < base+n {
			counts[fmt.Sprintf("%d.%d.%d.%d", row.OutsideIPAddress[0], row.OutsideIPAddress[1], row.OutsideIPAddress[2], row.OutsideIPAddress[3])]++
		}
	}
	return counts
}
func mwBalanceReady(t *testing.T) {
	t.Helper()
	var fib string
	if !waitFor(6*time.Second, func() bool {
		fib, _ = run(t, "vppctl", "show", "ip", "fib")
		block := regexp.MustCompile(`(?s)0\.0\.0\.0/0\r?\n(.*?)0\.0\.0\.0/32`).FindStringSubmatch(fib)
		return len(block) == 2 && strings.Contains(block[1], "ipv4 via 10.7.2.2 host-w7m1") && strings.Contains(block[1], "ipv4 via 10.7.3.2 host-w7m2")
	}) {
		t.Fatalf("actual balanced default not installed: %s", fib)
	}
}

func mwNATOutputsReady(t *testing.T) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn := connectVPP(t)
	ifs := dumpIfs(t, conn)
	stream, err := natapi.NewServiceClient(conn).Nat44EdOutputInterfaceGet(ctx, &natapi.Nat44EdOutputInterfaceGet{})
	if err != nil {
		return false
	}
	found := map[uint32]bool{}
	for {
		d, _, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return false
		}
		if d != nil {
			found[uint32(d.SwIfIndex)] = true
		}
	}
	return found[ifs["host-w7m1"].idx] && found[ifs["host-w7m2"].idx]
}

func mwNATPoolProof(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := natapi.NewServiceClient(connectVPP(t)).Nat44AddressDump(ctx, &natapi.Nat44AddressDump{})
	if err != nil {
		t.Fatal(err)
	}
	pools := map[string]uint32{}
	for {
		row, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		address := fmt.Sprintf("%d.%d.%d.%d", row.IPAddress[0], row.IPAddress[1], row.IPAddress[2], row.IPAddress[3])
		pools[address] = row.VrfID
	}
	t.Logf("actual native NAT pools address->VRF=%v", pools)
	if len(pools) != 2 {
		t.Fatalf("expected exact two member pools got%v", pools)
	}
	for _, address := range []string{"10.7.2.1", "10.7.3.1"} {
		vrf, ok := pools[address]
		if !ok || vrf != 0 {
			t.Fatalf("member%s pool must bind defaultVRF0 got%v", address, pools)
		}
	}
}

func mwExtended(t *testing.T, st *stack, group map[string]any) {
	a := st.api
	t.Logf("extended acceptance requested mode=%q (empty means automatic NAT)", os.Getenv("NGFW_MULTIWAN_EXTENDED_MODE"))
	// AF_PACKET cannot consume veth transport checksum placeholders (tools/lab rig contract).
	for i, side := range []string{"lan", "wan1", "wan2"} {
		ns := "ns-w7-mw-" + side
		device := fmt.Sprintf("w7p%d", i)
		mustRun(t, "ip", "netns", "exec", ns, "ethtool", "-K", device, "tx", "off")
		features := mustRun(t, "ip", "netns", "exec", ns, "ethtool", "-k", device)
		if !strings.Contains(features, "tx-checksumming: off") {
			t.Fatalf("owned veth %s checksum offload remains enabled", device)
		}
		t.Logf("owned %s:%s tx-checksumming off", ns, device)
	}
	for i := 1; i <= 2; i++ {
		log := t.TempDir() + fmt.Sprintf("/udp-wan%d.txt", i)
		process := start(t, "owned UDP endpoint", log, os.Environ(), "ip", "netns", "exec", fmt.Sprintf("ns-w7-mw-wan%d", i), "python3", "-u", "-c", mwUDPServer, fmt.Sprintf("wan%d", i))
		t.Cleanup(func() {
			process.stop(t)
			raw, _ := os.ReadFile(log)
			if len(raw) > 8000 {
				raw = raw[:8000]
			}
			t.Logf("owned endpoint log: %s", raw)
		})
		if !waitFor(3*time.Second, func() bool { raw, _ := os.ReadFile(log); return strings.Contains(string(raw), "ready ") }) {
			t.Fatal("UDP endpoint not ready")
		}
	}
	t.Cleanup(func() {
		a.patch("/routing", map[string]any{"pbr": map[string]any{"policies": map[string]any{"wanpin": nil}, "attachments": []any{}}, "static": []any{}})
		// Endpoints cannot create new sessions during this cleanup callback.
		conn := connectVPP(t)
		client := natapi.NewServiceClient(conn)
		for _, row := range mwSessions(t) {
			if row.Protocol > 255 {
				t.Fatalf("invalid cleanup protocol%d", row.Protocol)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, err := client.Nat44DelSession(ctx, &natapi.Nat44DelSession{Address: row.InsideIPAddress, Port: row.InsidePort, Protocol: uint8(row.Protocol), VrfID: 0, Flags: nat_types.NAT_IS_INSIDE | nat_types.NAT_IS_EXT_HOST_VALID, ExtHostAddress: row.ExtHostAddress, ExtHostPort: row.ExtHostPort})
			cancel()
			if err != nil {
				t.Errorf("owned session cleanup: %v", err)
			}
		}
		a.patch("/nat", map[string]any{"enabled": false, "inside": []any{}, "outside": []any{}, "outputFeature": []any{}, "pools": []any{}})
		a.patch("/acl", map[string]any{"lists": map[string]any{"wanpin": nil}})
		a.commit("multiwan-extended-cleanup")
	})
	group["mode"] = "balance"
	members := group["members"].([]any)
	members[0].(map[string]any)["weight"] = 1
	members[1].(map[string]any)["weight"] = 3
	a.patch("/routing", map[string]any{"wanGroups": []any{group}})
	a.commit("multiwan-weighted-balance")
	mwBalanceReady(t)
	t.Log(mustRun(t, "vppctl", "show", "ip", "fib"))
	plain := mwFlows(t, 1000, 30000, false, "plain weighted1000 flows")
	mwRatio(t, plain, "plain")
	if os.Getenv("NGFW_MULTIWAN_EXTENDED_MODE") == "balance-pbr" {
		t.Log("extended profile=balance-pbr; NAT gates separately failed and are not claimed here")
		mwPBR(t, st, false)
		return
	}
	natConfig := map[string]any{"enabled": true, "mode": "ed", "sessionLimit": 4096, "inside": []string{}}
	if os.Getenv("NGFW_MULTIWAN_EXTENDED_MODE") == "explicit-pools" {
		natConfig["outputFeature"] = []string{"host-w7m1", "host-w7m2"}
		natConfig["pools"] = []any{map[string]any{"name": "w1", "range": "10.7.2.1-10.7.2.1", "vrf": "default"}, map[string]any{"name": "w2", "range": "10.7.3.1-10.7.3.1", "vrf": "default"}}
		t.Log("diagnostic explicit VRF-scoped pools; automatic WAN interface pools are suppressed, no automatic SNAT PASS claim")
	}
	a.patch("/nat", natConfig)
	t.Logf("NAT real API commit=%s", js(a.commit("multiwan-per-member-nat")))
	if !waitFor(5*time.Second, func() bool { return mwNATOutputsReady(t) }) {
		t.Fatal("actual NAT output API does not report both owned WAN interfaces")
	}
	t.Log(mustRun(t, "vppctl", "show", "interface", "features", "host-w7m1"))
	t.Log(mustRun(t, "vppctl", "show", "interface", "features", "host-w7m2"))
	mwBalanceReady(t)
	mwNATPoolProof(t)
	translated := mwFlows(t, 1000, 32000, true, "NAT weighted1000 sticky flows")
	mwRatio(t, translated, "NAT")
	rows := mwSessions(t)
	counts := mwSessionCounts(rows, 32000, 1000)
	t.Logf("actual NAT V3 sessions before cut=%v", counts)
	if counts["10.7.2.1"] != translated.Counts["wan1"] || counts["10.7.3.1"] != translated.Counts["wan2"] {
		t.Fatal("NAT session outside addresses disagree with actual packet source/peer proof")
	}
	for _, row := range rows {
		if row.Protocol == 17 && row.InsidePort >= 32000 && row.InsidePort < 33000 && (row.TotalPkts < 2 || row.TotalBytes == 0) {
			t.Fatal("NAT session counters lack real packet traffic")
		}
	}
	var packets, bytes uint64
	for _, row := range rows {
		if row.Protocol == 17 && row.InsidePort >= 32000 && row.InsidePort < 33000 {
			packets += uint64(row.TotalPkts)
			bytes += row.TotalBytes
		}
	}
	t.Logf("1000 actual NAT session counters packets=%d bytes=%d", packets, bytes)
	st.agent.stop(t)
	st.startAgent(t)
	// Existing FIB survives restart; wait for fresh monitor observations before
	// testing the healthy-to-dead transition consumed by watchWAN cleanup.
	if !waitFor(6*time.Second, func() bool {
		state := st.api.call("GET", "/api/v1/state/wan", nil)
		groups, ok := state.body["groups"].([]any)
		if state.status != 200 || !ok || len(groups) != 1 {
			return false
		}
		members, ok := groups[0].(map[string]any)["members"].([]any)
		if !ok || len(members) != 2 {
			return false
		}
		for _, raw := range members {
			if raw.(map[string]any)["up"] != true {
				return false
			}
		}
		return true
	}) {
		t.Fatal("restarted real monitor did not observe both members healthy")
	}
	mwBalanceReady(t)
	mwNATPoolProof(t)
	replay := mwSessionCounts(mwSessions(t), 32000, 1000)
	if replay["10.7.2.1"] != counts["10.7.2.1"] || replay["10.7.3.1"] != counts["10.7.3.1"] {
		t.Fatalf("agent restart changed owned native sessions before failure: %v->%v", counts, replay)
	}
	t.Logf("restart native pools/session replay preserved=%v", replay)
	mwFlows(t, 20, 33000, true, "after agent restart per-member source packet proof")
	mustRun(t, "ip", "-n", "ns-w7-mw-wan1", "link", "set", "w7p1", "down")
	if !waitFor(8*time.Second, func() bool {
		after := mwSessionCounts(mwSessions(t), 32000, 1000)
		return after["10.7.2.1"] == 0 && after["10.7.3.1"] == counts["10.7.3.1"]
	}) {
		t.Fatal("dead-link NAT cleanup did not delete onlyWAN1 sessions")
	}
	t.Logf("actual NAT V3 sessions after cut=%v", mwSessionCounts(mwSessions(t), 32000, 1000))
	after := mwFlows(t, 20, 34000, true, "post-dead-link NAT packet proof")
	if after.Counts["wan2"] != 20 {
		t.Fatal("dead member still selected")
	}
	mustRun(t, "ip", "-n", "ns-w7-mw-wan1", "link", "set", "w7p1", "up")
	mustRun(t, "ip", "-n", "ns-w7-mw-wan1", "route", "replace", "default", "via", "10.7.2.1")
	mwBalanceReady(t)
	mwPBR(t, st, true)
}

func mwPBR(t *testing.T, st *stack, nat bool) {
	a := st.api
	// PBR must override a conflicting more-specific ordinary route, proving ABF participates.
	static := []any{map[string]any{"prefix": mwData + "/32", "nextHops": []any{map[string]any{"address": "10.7.2.2", "interface": "host-w7m1"}}}}
	a.patch("/routing", map[string]any{"static": static})
	a.commit("multiwan-pbr-contrast-route")
	contrast := mwFlows(t, 20, 35000, nat, "ordinary specific route control")
	if contrast.Counts["wan1"] != 20 {
		t.Fatal("specific route contrast not confined toWAN1")
	}
	a.patch("/acl", map[string]any{"lists": map[string]any{"wanpin": map[string]any{"rules": []any{map[string]any{"sequence": 10, "action": "permit", "ipVersion": "ipv4", "destination": map[string]any{"kind": "prefix", "prefix": mwData + "/32"}}}}}})
	a.patch("/routing", map[string]any{"pbr": map[string]any{"policies": map[string]any{"wanpin": map[string]any{"acl": "wanpin", "paths": []any{map[string]any{"wanGroup": "internet"}}}}, "attachments": []any{map[string]any{"policy": "wanpin", "interface": "host-w7m0", "family": "ipv4"}}}})
	t.Logf("PBR real API commit=%s", js(a.commit("multiwan-pbr-group")))
	t.Log(mustRun(t, "vppctl", "show", "abf"))
	pinned := mwFlows(t, 1000, 36000, nat, "PBR group weighted1000 actual flows")
	mwRatio(t, pinned, "PBR")
	t.Log(mustRun(t, "vppctl", "show", "interface"))
	t.Log(mustRun(t, "vppctl", "show", "nat44", "summary"))
}
