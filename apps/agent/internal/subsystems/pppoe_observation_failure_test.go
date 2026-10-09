package subsystems

import (
	"context"
	"encoding/json"
	"errors"
	"go.fd.io/govpp/api"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	desc "ngfw/agent/internal/descriptors/pppoe"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/pppoe"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPppoeObservationFailureWithdrawsAndRetries(t *testing.T) {
	for _, failure := range []string{"unreadable", "malformed"} {
		t.Run(failure, func(t *testing.T) {
			rt, _, v := newTestRuntime(t)
			rt.carrierMode, rt.carrierRoot = true, t.TempDir()
			spec, err := pppoe.NewCarrierSpec(rt.owner, "wan0", "wanraw", 1492)
			if err != nil {
				t.Fatal(err)
			}
			s := pppoe.Session{Iface: "wan0", HostIf: "tap0", MTU: 1492, DefaultRoute: true, Carrier: &spec}
			rt.applied = map[string]pppoe.Session{s.Iface: s}
			m := desc.Mirror{Interface: s.Iface, LocalIPv4: "192.0.2.7/32", PeerIPv4: "192.0.2.1", DefaultRoute: true}
			rt.mirrored = map[string]desc.Mirror{s.Iface: m}
			rt.carrierReady = map[string]carrierForwarding{s.Iface: {epoch: "prior", admission: "prior", until: time.Now().Add(time.Minute), mirror: m}}
			v.AddInterface(s.Iface, "")
			v.Reply("sw_interface_get_table", &interfaces.SwInterfaceGetTableReply{VrfID: 9000})
			route, address, failDelete := true, true, true
			v.On("ip_route_add_del", func(req api.Message) ([]api.Message, error) {
				r := req.(*ip.IPRouteAddDel)
				if r.IsAdd || !r.IsMultipath {
					t.Fatal("invalid withdrawal", r)
				}
				if len(rt.carrierReady) != 0 {
					t.Fatal("readiness survived until route removal")
				}
				if failDelete {
					return nil, errors.New("injected route cleanup failure")
				}
				route = false
				return []api.Message{&ip.IPRouteAddDelReply{}}, nil
			})
			v.On("ip_address_dump", func(api.Message) ([]api.Message, error) {
				if !address {
					return nil, nil
				}
				p, _ := ip_types.ParseAddressWithPrefix(m.LocalIPv4)
				return []api.Message{&ip.IPAddressDetails{Prefix: p}}, nil
			})
			v.On("sw_interface_add_del_address", func(req api.Message) ([]api.Message, error) {
				if req.(*interfaces.SwInterfaceAddDelAddress).IsAdd {
					t.Fatal("unexpected address add")
				}
				address = false
				return []api.Message{&interfaces.SwInterfaceAddDelAddressReply{}}, nil
			})
			run := &observationFailureRunner{carrierTestRunner: carrierTestRunner{lease: namespaceLeaseForTest(spec)}, failWithdraw: true}
			rt.runner = run
			state := filepath.Join(rt.sessionStateDir(s), s.HostIf+".state")
			if err = os.MkdirAll(filepath.Dir(state), 0700); err != nil {
				t.Fatal(err)
			}
			if failure == "unreadable" {
				if err = os.Mkdir(state, 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				if err = os.WriteFile(state, []byte("phase=up\nlocal=not-an-address\npeer=invalid\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err = rt.poll(t.Context()); err == nil {
				t.Fatal("observation/cleanup failure was hidden")
			}
			if rt.CarrierDelegationReady(s.Iface, "prior") {
				t.Fatal("failed observation still admits delegated prefixes")
			}
			if len(rt.carrierReady) != 0 || len(rt.mirrored) != 1 {
				t.Fatal("readiness or retry tracking incorrect")
			}
			if run.withdraws != 1 {
				t.Fatal("VPP failure prevented independent kernel withdrawal")
			}
			failDelete = false
			if err = rt.poll(t.Context()); err == nil {
				t.Fatal("invalid observation was hidden")
			}
			if route || address || len(rt.mirrored) != 0 || len(rt.carrierReady) != 0 {
				t.Fatal("stale forwarding retained after retry")
			}
			{
				run.failWithdraw = false
				if err = rt.poll(t.Context()); err == nil {
					t.Fatal("invalid hook must remain reported")
				}
				if run.withdraws != 3 {
					t.Fatal("kernel withdrawal not retried after mirror removal", run.withdraws)
				}
			}
		})
	}
}

type observationFailureRunner struct {
	carrierTestRunner
	withdraws    int
	failWithdraw bool
}

func (r *observationFailureRunner) Run(ctx context.Context, c renderers.Command) (renderers.Output, error) {
	if c.Path == pppoe.Python3Bin && len(c.Args) > 2 && c.Args[2] == "broker-result" {
		var op string
		_ = json.Unmarshal(r.request["op"], &op)
		if op == "withdraw" {
			r.withdraws++
			if r.failWithdraw {
				return renderers.Output{}, errors.New("injected kernel cleanup failure")
			}
		}
	}
	return r.carrierTestRunner.Run(ctx, c)
}
