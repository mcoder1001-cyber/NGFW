package subsystems

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/l2"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	desc "ngfw/agent/internal/descriptors/pppoe"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/pppoe"
)

// These controls invoke the product runtime and broker adapter. Only the host
// process and VPP API boundaries are simulated; no forwarding helper is bypassed.
type forwardingProductRunner struct {
	carrierTestRunner
	verified   carrierVerification
	invocation string
	operations []string
	onResult   func(string)
}

func (r *forwardingProductRunner) Run(ctx context.Context, c renderers.Command) (renderers.Output, error) {
	if c.Path == pppoe.SystemctlBin && len(c.Args) > 0 && c.Args[0] == "show" {
		return renderers.Output{Stdout: []byte("ActiveState=active\nInvocationID=" + r.invocation + "\n")}, nil
	}
	if c.Path == pppoe.Python3Bin && len(c.Args) > 2 && c.Args[2] == "broker-result" {
		var op string
		_ = json.Unmarshal(r.request["op"], &op)
		r.operations = append(r.operations, op)
		if r.onResult != nil {
			r.onResult(op)
		}
		var value any
		switch op {
		case "verify":
			value = r.verified
		case "probe":
			value = map[string]any{"sent": 1, "received": 1, "latencyMs": 7}
		default:
			return r.carrierTestRunner.Run(ctx, c)
		}
		receipt := r.receipt
		receipt.OK = true
		receipt.Result, _ = json.Marshal(value)
		body, err := json.Marshal(receipt)
		return renderers.Output{Stdout: body}, err
	}
	return r.carrierTestRunner.Run(ctx, c)
}

type forwardingProductFixture struct {
	rt      *PppoeRuntime
	run     *forwardingProductRunner
	session pppoe.Session
	mirror  desc.Mirror
	rows    []*interfaces.SwInterfaceDetails
	reverse bool
}

func newForwardingProductFixture(t *testing.T) *forwardingProductFixture {
	t.Helper()
	rt, _, v := newTestRuntime(t)
	spec, err := pppoe.NewCarrierSpec(rt.owner, "pppwan", "wanraw", 1492)
	if err != nil {
		t.Fatal(err)
	}
	s := pppoe.Session{Iface: "pppwan", HostIf: "ppp0", MTU: 1492, IPv6: "off", Carrier: &spec}
	lease := desc.CarrierLease{Spec: spec, Token: spec.Token(), Generation: strings.Repeat("a", 32), Boot: "boot", Namespace: []uint64{1, 2}, Configured: true}
	run := &forwardingProductRunner{carrierTestRunner: carrierTestRunner{lease: lease}, invocation: strings.Repeat("b", 32), verified: carrierVerification{Verified: true, Token: lease.Token, Generation: lease.Generation, Boot: lease.Boot, Namespace: lease.Namespace, MTU: 1492, Links: map[string]uint32{spec.RawHost(): 1, spec.TransitHost(): 2, "ppp0": 3}, PPPAddresses: []string{"192.0.2.10/32"}}}
	rt.runner = run
	rt.carrierReady = map[string]carrierForwarding{}
	rt.applied[s.Iface] = s
	f := &forwardingProductFixture{rt: rt, run: run, session: s, mirror: desc.Mirror{LocalIPv4: "192.0.2.10/32", PeerIPv4: "192.0.2.1"}, reverse: true}
	for i, name := range []string{spec.Parent, spec.RawLogical(), s.Iface} {
		idx := uint32(i + 1)
		tag := ""
		if i > 0 {
			tag = rt.owner + ":" + name
		}
		f.rows = append(f.rows, &interfaces.SwInterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(idx), SupSwIfIndex: idx, InterfaceName: name, Tag: tag, Mtu: []uint32{1492}, Flags: interface_types.IF_STATUS_API_FLAG_ADMIN_UP | interface_types.IF_STATUS_API_FLAG_LINK_UP})
	}
	v.On("sw_interface_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, row := range f.rows {
			out = append(out, row)
		}
		return out, nil
	})
	v.On("l2_xconnect_dump", func(api.Message) ([]api.Message, error) {
		out := []api.Message{&l2.L2XconnectDetails{RxSwIfIndex: 1, TxSwIfIndex: 2}}
		if f.reverse {
			out = append(out, &l2.L2XconnectDetails{RxSwIfIndex: 2, TxSwIfIndex: 1})
		}
		return out, nil
	})
	v.On("ip_address_dump", func(message api.Message) ([]api.Message, error) {
		req := message.(*ip.IPAddressDump)
		address := spec.VPP4()
		if req.IsIPv6 {
			address = spec.VPP6()
		}
		prefix, err := ip_types.ParsePrefix(address)
		return []api.Message{&ip.IPAddressDetails{Prefix: ip_types.AddressWithPrefix(prefix)}}, err
	})
	dir := rt.sessionStateDir(s)
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, s.HostIf+".ipv6.admission"), []byte("admission"), 0600); err != nil {
		t.Fatal(err)
	}
	f.writeGeneration(t, "c")
	return f
}
func (f *forwardingProductFixture) writeGeneration(t *testing.T, digit string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.rt.sessionStateDir(f.session), f.session.HostIf+".state"), []byte("phase=up\nsession_generation="+strings.Repeat(digit, 32)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
}
func (f *forwardingProductFixture) prepare(t *testing.T) carrierForwarding {
	t.Helper()
	r, err := f.rt.prepareCarrierForwarding(t.Context(), f.session, f.mirror)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestCarrierForwardingProductPositive(t *testing.T) {
	f := newForwardingProductFixture(t)
	ready := f.prepare(t)
	if ready.epoch == "" || !ready.until.After(time.Now()) {
		t.Fatal("missing fresh forwarding receipt")
	}
	if len(f.rt.carrierReady) != 0 {
		t.Fatal("verification published readiness before mirror convergence")
	}
	f.rt.carrierReady[f.session.Iface] = ready // watcher publishes after mirror convergence
	local, peer, epoch, ok := f.rt.ForwardingGateway(f.session.Iface)
	if !ok || local != f.mirror.LocalIPv4 || peer != f.mirror.PeerIPv4 || epoch != ready.epoch {
		t.Fatal("gateway lost verified identity")
	}
	result := f.rt.ProbeForwarding(t.Context(), f.session.Iface, &ngfwv1.WanMonitor{Type: proto.String("icmp"), Target: proto.String("192.0.2.1")})
	if result.Unavailable || result.Sent != 1 || result.Received != 1 || result.AvgLatencyMs != 7 {
		t.Fatalf("probe failed: %+v", result)
	}
	if strings.Join(f.run.operations, ",") != "list,configure,verify,probe" {
		t.Fatalf("broker path skipped: %v", f.run.operations)
	}
}
func TestCarrierForwardingProductRejectsDrift(t *testing.T) {
	for _, name := range []string{"foreign-transit", "missing-transit", "missing-reverse", "address-mismatch", "missing-kernel-link", "foreign-namespace", "process-replaced"} {
		t.Run(name, func(t *testing.T) {
			f := newForwardingProductFixture(t)
			f.rt.carrierReady[f.session.Iface] = carrierForwarding{epoch: "old", until: time.Now().Add(time.Minute)}
			switch name {
			case "foreign-transit":
				f.rows[2].Tag = "foreign:pppwan"
			case "missing-transit":
				f.rows = f.rows[:2]
			case "missing-reverse":
				f.reverse = false
			case "address-mismatch":
				f.run.verified.PPPAddresses = []string{"192.0.2.99/32"}
			case "missing-kernel-link":
				delete(f.run.verified.Links, "ppp0")
			case "foreign-namespace":
				f.run.verified.Namespace = []uint64{3, 4}
			case "process-replaced":
				f.run.onResult = func(op string) {
					if op == "verify" {
						f.run.invocation = strings.Repeat("e", 32)
					}
				}
			}
			if _, err := f.rt.prepareCarrierForwarding(t.Context(), f.session, f.mirror); err == nil {
				t.Fatal("drift accepted")
			}
			if _, ok := f.rt.carrierReady[f.session.Iface]; ok {
				t.Fatal("old forwarding readiness survived failure")
			}
		})
	}
}
func TestCarrierForwardingProductRejectsInflightReplacement(t *testing.T) {
	for _, name := range []string{"ncp", "process", "readiness"} {
		t.Run(name, func(t *testing.T) {
			f := newForwardingProductFixture(t)
			f.rt.carrierReady[f.session.Iface] = f.prepare(t)
			f.run.onResult = func(op string) {
				if op != "probe" {
					return
				}
				switch name {
				case "ncp":
					f.writeGeneration(t, "d") // same address, lease and process
				case "process":
					f.run.invocation = strings.Repeat("e", 32)
				case "readiness":
					f.rt.mu.Lock()
					delete(f.rt.carrierReady, f.session.Iface)
					f.rt.mu.Unlock()
				}
			}
			result := f.rt.ProbeForwarding(t.Context(), f.session.Iface, &ngfwv1.WanMonitor{Type: proto.String("icmp"), Target: proto.String("192.0.2.1")})
			if !result.Unavailable {
				t.Fatalf("obsolete probe accepted: %+v", result)
			}
		})
	}
}

func TestCarrierForwardingProductRejectsNCPReplacementDuringVerification(t *testing.T) {
	f := newForwardingProductFixture(t)
	f.run.onResult = func(op string) {
		if op == "verify" {
			f.writeGeneration(t, "d")
		}
	}
	if _, err := f.rt.prepareCarrierForwarding(t.Context(), f.session, f.mirror); err == nil {
		t.Fatal("forwarding evidence spanning two NCP sessions accepted")
	}
}
