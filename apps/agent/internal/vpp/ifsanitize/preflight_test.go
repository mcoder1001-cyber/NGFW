package ifsanitize_test

import (
	"context"
	"strings"
	"testing"

	"go.fd.io/govpp/api"

	classifyapi "ngfw/agent/binapi/classify"
	interfaces "ngfw/agent/binapi/interface"
	ipapi "ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	ipsecapi "ngfw/agent/binapi/ipsec"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/binapi/vlib"
	"ngfw/agent/internal/vpp/fake"
	"ngfw/agent/internal/vpp/ifsanitize"
)

// Real VPP 26.06 output captured on vrx-a during TestV19InheritanceClearedOnHost (loop292 with an
// inherited ip classify binding to table 0).
const fibWithClassify = `ipv4-VRF:0, fib_index:0, flow hash:[src dst sport dport proto flowlabel ] epoch:0 flags:none locks:[default-route:1, ]
0.0.0.0/0
  unicast-ip4-chain
  [@0]: dpo-load-balance: [proto:ip4 index:1 buckets:1 uRPF:0 to:[0:0]]
    [0] [@0]: dpo-drop ip4
10.2.91.1/32
  unicast-ip4-chain
  [@0]: dpo-load-balance: [proto:ip4 index:14 buckets:1 uRPF:23 to:[0:0]]
    [0] [@13]: ip4-classify:[0]:table:0
`

const inaclDeleted = ` Intfc idx      Classify table		Interface name
         2                   0		DELETED (2)
`

func preflightFake(tables []uint32, cli map[string]string) *fake.Client {
	f := fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{}))
	f.Reply("sw_interface_dump",
		&interfaces.SwInterfaceDetails{SwIfIndex: 0, InterfaceName: "local0"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 5, InterfaceName: "host-w9l0", Tag: "w9:host-w9l0"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 6, InterfaceName: "loop292", Tag: "w2:loop292"},
	)
	f.Reply("classify_table_ids", &classifyapi.ClassifyTableIdsReply{Ids: tables, Count: uint32(len(tables))}) //nolint:gosec // test sizes
	f.On("classify_table_by_interface", func(req api.Message) ([]api.Message, error) {
		r := req.(*classifyapi.ClassifyTableByInterface)
		rep := &classifyapi.ClassifyTableByInterfaceReply{SwIfIndex: r.SwIfIndex, IP4TableID: ifsanitize.NoIndex, IP6TableID: ifsanitize.NoIndex, L2TableID: ifsanitize.NoIndex}
		if r.SwIfIndex == 5 {
			rep.IP4TableID = 7
		}
		return []api.Message{rep}, nil
	})
	f.On("cli_inband", func(req api.Message) ([]api.Message, error) {
		return []api.Message{&vlib.CliInbandReply{Reply: cli[req.(*vlib.CliInband).Cmd]}}, nil
	})
	f.On("ip_address_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*ipapi.IPAddressDump)
		if r.SwIfIndex == 6 && !r.IsIPv6 {
			p, _ := ip_types.ParseAddressWithPrefix("10.2.91.1/32")
			return []api.Message{&ipapi.IPAddressDetails{SwIfIndex: 6, Prefix: p}}, nil
		}
		return nil, nil
	})
	f.Reply("ipsec_spd_interface_dump",
		&ipsecapi.IpsecSpdInterfaceDetails{SpdIndex: 1, SwIfIndex: 9}, // deleted interface
		&ipsecapi.IpsecSpdInterfaceDetails{SpdIndex: 2, SwIfIndex: 6}, // tagged: fine
	)
	return f
}

func TestPreflightNamesTheOffendingInterface(t *testing.T) {
	f := preflightFake([]uint32{1}, map[string]string{"show ip fib": fibWithClassify, "show inacl type ip4": inaclDeleted})
	got, err := ifsanitize.Preflight(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	fatal := 0
	for _, g := range got {
		lines = append(lines, g.String())
		if g.Fatal {
			fatal++
		}
	}
	all := strings.Join(lines, "\n")
	t.Logf("findings:\n%s", all)
	for _, want := range []string{
		"FAIL  interface host-w9l0 (tag w9:host-w9l0): input ACL ip4 bound to classify table 7, which does not exist",
		"FAIL  interface loop292 (tag w2:loop292): FIB ipv4-VRF:0 10.2.91.1/32 has a classify DPO to classify table 0, which does not exist",
		"WARN  interface DELETED (2): input ACL ip4 bound to classify table 0, which does not exist",
		"WARN  interface DELETED (9): IPsec SPD (index 1) still bound",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q", want)
		}
	}
	if fatal != 2 || !got[0].Fatal || !got[1].Fatal {
		t.Fatalf("fatal findings first, want 2: %v", lines)
	}
}

func TestPreflightClean(t *testing.T) {
	// the same bindings with their tables alive are fine
	f := preflightFake([]uint32{0, 7}, map[string]string{"show ip fib": fibWithClassify, "show inacl type ip4": inaclDeleted})
	got, err := ifsanitize.Preflight(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range got {
		if g.Fatal {
			t.Fatalf("unexpected fatal finding %s", g)
		}
	}
}
