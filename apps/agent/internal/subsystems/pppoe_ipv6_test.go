package subsystems

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"go.fd.io/govpp/api"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	mss "ngfw/agent/binapi/mss_clamp"
	"ngfw/agent/internal/renderers/pppoe"
)

// fakeFIB is a tiny VPP model: interface addresses and default-route paths keyed "<prefix> via <nh> t<table>".
type fakeFIB struct {
	addrs  map[string]bool
	routes map[string]bool
}

func (f *fakeFIB) sorted() (a, r []string) {
	for k := range f.addrs {
		a = append(a, k)
	}
	for k := range f.routes {
		r = append(r, k)
	}
	sort.Strings(a)
	sort.Strings(r)
	return a, r
}

// The IPv6 side of a session follows its ipv6-up/ipv6-down state: addresses (/128) and the ::/0 path via the RA
// router are mirrored next to IPv4, an IPv6 down withdraws only IPv6, a renumbering replaces the address, and
// removing the session withdraws everything and its IPv6 state.
func TestPppoeIPv6MirrorFollowsHookState(t *testing.T) {
	rt, _, v := newTestRuntime(t)
	rt.globalsOwner = false
	root := t.TempDir()
	t.Setenv(EnvHostServicesDir, root)
	paths := pppoe.PathsUnder(filepath.Join(root, "pppoe"))
	rt.renderer = pppoe.New(pppoe.WithPaths(paths))
	rt.stateDir = paths.StateDir
	v.AddInterface("wan0", "")
	fib := &fakeFIB{addrs: map[string]bool{}, routes: map[string]bool{}}
	v.On("sw_interface_get_table", func(req api.Message) ([]api.Message, error) {
		if req.(*interfaces.SwInterfaceGetTable).IsIPv6 {
			return []api.Message{&interfaces.SwInterfaceGetTableReply{VrfID: 9402}}, nil
		}
		return []api.Message{&interfaces.SwInterfaceGetTableReply{VrfID: 9401}}, nil
	})
	v.Reply("mss_clamp_enable_disable", &mss.MssClampEnableDisableReply{})
	v.On("ip_address_dump", func(req api.Message) ([]api.Message, error) {
		var out []api.Message
		for a := range fib.addrs {
			p, _ := ip_types.ParseAddressWithPrefix(a)
			if (p.Address.Af == ip_types.ADDRESS_IP6) == req.(*ip.IPAddressDump).IsIPv6 {
				out = append(out, &ip.IPAddressDetails{Prefix: p})
			}
		}
		return out, nil
	})
	v.On("sw_interface_add_del_address", func(req api.Message) ([]api.Message, error) {
		r := req.(*interfaces.SwInterfaceAddDelAddress)
		if r.IsAdd {
			fib.addrs[r.Prefix.String()] = true
		} else {
			delete(fib.addrs, r.Prefix.String())
		}
		return []api.Message{&interfaces.SwInterfaceAddDelAddressReply{}}, nil
	})
	v.On("ip_route_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*ip.IPRouteAddDel)
		if !r.IsMultipath || r.Route.NPaths != 1 {
			t.Fatal("whole default route mutation")
		}
		p := r.Route.Paths[0]
		nh := p.Nh.Address.GetIP4().String()
		if r.Route.Prefix.Address.Af == ip_types.ADDRESS_IP6 {
			nh = p.Nh.Address.GetIP6().String()
		}
		k := r.Route.Prefix.String() + " via " + nh + " t" + strconv.Itoa(int(r.Route.TableID))
		if r.IsAdd {
			fib.routes[k] = true
		} else {
			delete(fib.routes, k)
		}
		return []api.Message{&ip.IPRouteAddDelReply{}}, nil
	})
	session := pppoe.Session{Iface: "wan0", HostIf: "tap0", Username: "u", Password: "NGFW_TEST_PSK_F-pppoe-client-wiring", MTU: 1492, DefaultRoute: true, MSSClamp: true, IPv6: "dhcpv6"} //nolint:gosec // required non-production fixture marker
	if err := rt.Apply(context.Background(), []pppoe.Session{session}); err != nil {
		t.Fatal(err)
	}
	ipup := func() {
		t.Helper()
		cmd := exec.Command(filepath.Join(paths.IPUpDir, "ngfw-tap0")) //nolint:gosec // rendered hook in this private test directory
		cmd.Env = append(os.Environ(), "PPP_IPPARAM=ngfw-tap0", "IPLOCAL=198.51.100.5", "IPREMOTE=198.51.100.1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatal(err, string(out))
		}
	}
	state6 := func(body string) {
		t.Helper()
		// what the ipv6-up hook's refresher writes (the hook itself needs a PPP link: lab-host acceptance)
		if err := os.WriteFile(filepath.Join(paths.StateDir, "tap0.state6"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	check := func(label string, wantAddrs, wantRoutes []string) {
		t.Helper()
		if err := rt.poll(context.Background()); err != nil {
			t.Fatal(label, err)
		}
		a, r := fib.sorted()
		if strings.Join(a, ",") != strings.Join(wantAddrs, ",") || strings.Join(r, ",") != strings.Join(wantRoutes, ",") {
			t.Fatalf("%s:\naddrs  %v want %v\nroutes %v want %v", label, a, wantAddrs, r, wantRoutes)
		}
	}
	ipup()
	state6("phase=up\nppp_iface=ppp0\nlllocal=fe80::1:2\nllremote=fe80::9\naddr=2001:db8:9::100/128\naddr=2001:db8:9:0:1:2:3:4/64\ngw=fe80::9\npd=2001:db8:9100::/56\n")
	dualAddrs := []string{"198.51.100.5/32", "2001:db8:9:0:1:2:3:4/128", "2001:db8:9::100/128"}
	dualRoutes := []string{"0.0.0.0/0 via 198.51.100.1 t9401", "::/0 via fe80::9 t9402"}
	check("dual stack up", dualAddrs, dualRoutes)
	st, err := rt.State("wan0", 0, "")
	if err != nil || st.GetIpv6() != "2001:db8:9::100/128, 2001:db8:9:0:1:2:3:4/64, delegated 2001:db8:9100::/56" {
		t.Fatalf("state %+v %v", st, err)
	}
	// runtime-owned addresses (both families) are reported so the static address reconciler leaves them alone
	virt, err := rt.runtimeAddresses(context.Background(), nil)
	if err != nil || len(virt) != 1 {
		t.Fatal(virt, err)
	}
	for _, a := range []string{"198.51.100.5", "2001:db8:9::100", "2001:db8:9:0:1:2:3:4"} {
		found := false
		for _, m := range virt {
			found = found || m[a]
		}
		if !found {
			t.Fatalf("runtime addresses lack %s: %v", a, virt)
		}
	}
	check("unchanged", dualAddrs, dualRoutes)
	// renumbering (new SLAAC prefix): the old address goes, the new one comes
	state6("phase=up\nppp_iface=ppp0\naddr=2001:db8:9::100/128\naddr=2001:db8:a:0:1:2:3:4/64\ngw=fe80::9\n")
	check("renumbered", []string{"198.51.100.5/32", "2001:db8:9::100/128", "2001:db8:a:0:1:2:3:4/128"}, dualRoutes)
	// IPv6 down (the real ipv6-down hook) withdraws IPv6 only
	cmd := exec.Command(filepath.Join(paths.IPv6DownDir, "ngfw-tap0")) //nolint:gosec // rendered hook in this private test directory
	cmd.Env = append(os.Environ(), "PPP_IPPARAM=ngfw-tap0", "PPP_IFACE=ppp0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	check("ipv6 down", []string{"198.51.100.5/32"}, []string{"0.0.0.0/0 via 198.51.100.1 t9401"})
	if st, _ := rt.State("wan0", 0, ""); st.GetIpv6() != "" || st.GetPhase() != "up" {
		t.Fatalf("after ipv6 down: %+v", st)
	}
	// up again, then remove the session: everything withdrawn, IPv6 state gone
	state6("phase=up\nppp_iface=ppp0\naddr=2001:db8:9::100/128\ngw=fe80::9\n")
	check("ipv6 up again", []string{"198.51.100.5/32", "2001:db8:9::100/128"}, dualRoutes)
	if err := rt.Apply(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if a, r := fib.sorted(); len(a)+len(r) != 0 {
		t.Fatalf("removal left %v %v", a, r)
	}
	for _, f := range []string{"tap0.state", "tap0.state6", "tap0.pd"} {
		if _, err := os.Stat(filepath.Join(paths.StateDir, f)); !os.IsNotExist(err) {
			t.Fatalf("%s kept after removal", f)
		}
	}
	for _, f := range []string{filepath.Join(paths.IPv6UpDir, "ngfw-tap0"), filepath.Join(paths.HelperDir, "ngfw-dhcpcd-tap0.conf")} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Fatalf("%s kept after removal", f)
		}
	}
}

// IPv6 hook state is ignored for a session whose IPv6 is off (a stale state6 never reaches VPP).
func TestPppoeIPv6IgnoredWhenOff(t *testing.T) {
	s := pppoe.Session{Iface: "wan0", HostIf: "tap0", IPv6: "off", DefaultRoute: true}
	v6 := pppoe.IPv6State{Up: true}
	m := mirrorFor(s, nil, v6)
	if len(m.LocalIPv6) != 0 || m.PeerIPv6 != "" {
		t.Fatal(m)
	}
}
