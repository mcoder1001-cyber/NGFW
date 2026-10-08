package subsystems

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"net/netip"
	"ngfw/agent/binapi/interface_types"
	ipapi "ngfw/agent/binapi/ip"
	l2api "ngfw/agent/binapi/l2"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/df6"
	desc "ngfw/agent/internal/descriptors/pppoe"
	"ngfw/agent/internal/multiwan"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/pppoe"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type carrierForwarding struct {
	lease     desc.CarrierLease
	mirror    desc.Mirror
	admission string
	epoch     string
	until     time.Time
}

// ForwardingGateway never accepts raw PPP hook state as forwarding evidence.
func (rt *PppoeRuntime) ForwardingGateway(logical string) (string, string, string, bool) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	r, ok := rt.carrierReady[logical]
	s := rt.applied[logical]
	if !ok || s.Carrier == nil || !time.Now().Before(r.until) || r.mirror.LocalIPv4 == "" {
		return "", "", "", false
	}
	return r.mirror.LocalIPv4, r.mirror.PeerIPv4, r.epoch, true
}
func (rt *PppoeRuntime) CarrierDelegationReady(logical, admission string) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	r, ok := rt.carrierReady[logical]
	return ok && rt.applied[logical].Carrier != nil && admission != "" && admission == r.admission && time.Now().Before(r.until)
}

// prepareCarrierForwarding verifies actual kernel state and the owned VPP TAP.
// Caller holds the transaction fence and rt.mu. Readiness is published only
// after the normal VPP mirror has successfully converged.
func (rt *PppoeRuntime) prepareCarrierForwarding(ctx context.Context, s pppoe.Session, m desc.Mirror) (carrierForwarding, error) {
	delete(rt.carrierReady, s.Iface)
	invocation, err := rt.carrierInvocation(ctx, s)
	if err != nil {
		return carrierForwarding{}, err
	}
	lease, err := rt.carrierLease(ctx, s)
	if err != nil {
		return carrierForwarding{}, err
	}
	host := &pppoeCarrierHost{runner: rt.runner}
	if err = host.Configure(ctx, lease, s.DefaultRoute && s.IPv6Enabled()); err != nil {
		return carrierForwarding{}, err
	}
	if _, err = host.Verify(ctx, lease); err != nil {
		return carrierForwarding{}, err
	}
	ifs, err := df6.DumpInterfaces(ctx, rt.vpp, rt.owner)
	if err != nil {
		return carrierForwarding{}, err
	}
	idx, ok := ifs.IndexByTag(s.Iface)
	if !ok {
		return carrierForwarding{}, errors.New("owned PPP transit disappeared")
	}
	details, ok := ifs.Table().Details(idx)
	if !ok || len(details.Mtu) == 0 || details.Mtu[0] != s.MTU {
		return carrierForwarding{}, errors.New("PPP transit MTU differs")
	}
	if err = rt.verifyCarrierVPP(ctx, s, ifs, idx); err != nil {
		return carrierForwarding{}, err
	}
	admission, err := os.ReadFile(filepath.Join(rt.sessionStateDir(s), s.HostIf+".ipv6.admission"))
	if err != nil {
		return carrierForwarding{}, err
	}
	after, err := rt.carrierInvocation(ctx, s)
	if err != nil || after != invocation {
		return carrierForwarding{}, errors.New("PPP process changed during forwarding verification")
	}
	sessionGeneration, err := rt.carrierSessionGeneration(s)
	if err != nil {
		return carrierForwarding{}, err
	}
	epoch := lease.Generation + ":" + strings.TrimSpace(string(admission)) + ":" + sessionGeneration + ":" + invocation
	return carrierForwarding{lease: lease, mirror: m, admission: strings.TrimSpace(string(admission)), epoch: epoch, until: time.Now().Add(3 * time.Second)}, nil
}

func (rt *PppoeRuntime) ProbeForwarding(ctx context.Context, logical string, monitor *ngfwv1.WanMonitor) multiwan.CheckResult {
	fail := multiwan.CheckResult{Unavailable: true}
	if monitor == nil {
		return fail
	}
	target, err := netip.ParseAddr(monitor.GetTarget())
	if err != nil || !target.Is4() || target.IsUnspecified() || target.IsMulticast() {
		return fail
	}
	kind := monitor.GetType()
	if kind != "icmp" && kind != "http" && kind != "dns" {
		return fail
	}
	rt.mu.Lock()
	r, ok := rt.carrierReady[logical]
	s := rt.applied[logical]
	rt.mu.Unlock()
	if !ok || s.Carrier == nil || !time.Now().Before(r.until) {
		return fail
	}
	invocation, err := rt.carrierInvocation(ctx, s)
	if err != nil || !strings.HasSuffix(r.epoch, ":"+invocation) {
		return fail
	}
	var result struct {
		Sent        int     `json:"sent"`
		Received    int     `json:"received"`
		LatencyMs   float64 `json:"latencyMs"`
		Unavailable bool    `json:"unavailable"`
	}
	request := map[string]any{"op": "probe", "token": r.lease.Token, "generation": r.lease.Generation, "kind": kind, "target": target.String()}
	if err = (&pppoeCarrierHost{runner: rt.runner}).broker(ctx, &result, r.lease.Token, request); err != nil {
		return fail
	}
	sessionGeneration, err := rt.carrierSessionGeneration(s)
	if err != nil || !strings.Contains(r.epoch, ":"+sessionGeneration+":") {
		return fail
	}
	invocationAfter, err := rt.carrierInvocation(ctx, s)
	if err != nil || invocationAfter != invocation {
		return fail
	}
	rt.mu.Lock()
	current, still := rt.carrierReady[logical]
	rt.mu.Unlock()
	if !still || current.epoch != r.epoch || current.admission != r.admission || !time.Now().Before(current.until) {
		return fail
	}
	return multiwan.CheckResult{Sent: result.Sent, Received: result.Received, AvgLatencyMs: int(result.LatencyMs), Unavailable: result.Unavailable}
}

func (rt *PppoeRuntime) carrierInvocation(ctx context.Context, s pppoe.Session) (string, error) {
	out, err := rt.runner.Run(ctx, renderers.Command{Path: pppoe.SystemctlBin, Args: []string{"show", carrierUnit(s), "-p", "InvocationID", "-p", "ActiveState"}, Timeout: 3 * time.Second})
	if err != nil {
		return "", errors.New("PPP process identity unavailable")
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(out.Stdout), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if ok {
			values[k] = v
		}
	}
	id := values["InvocationID"]
	raw, e := hex.DecodeString(id)
	if values["ActiveState"] != "active" || e != nil || len(raw) != 16 {
		return "", errors.New("PPP process is not active with a valid invocation identity")
	}
	return id, nil
}

func (rt *PppoeRuntime) verifyCarrierVPP(ctx context.Context, s pppoe.Session, ifs *df6.Interfaces, transit uint32) error {
	parent, err := ifs.Index(s.Carrier.Parent)
	if err != nil {
		return err
	}
	raw, ok := ifs.IndexByTag(s.Carrier.RawLogical())
	if !ok {
		return errors.New("owned raw PPP TAP disappeared")
	}
	stream, err := l2api.NewServiceClient(rt.vpp).L2XconnectDump(ctx, &l2api.L2XconnectDump{})
	if err != nil {
		return err
	}
	forward, reverse := false, false
	for {
		row, e := stream.Recv()
		if errors.Is(e, io.EOF) {
			break
		}
		if e != nil {
			return e
		}
		if row.RxSwIfIndex == parent {
			if uint32(row.TxSwIfIndex) != raw {
				return errors.New("raw PPP cross-connect changed")
			}
			forward = true
		}
		if uint32(row.RxSwIfIndex) == raw {
			if row.TxSwIfIndex != parent {
				return errors.New("raw PPP return cross-connect changed")
			}
			reverse = true
		}
	}
	if !forward || !reverse {
		return errors.New("raw PPP cross-connect is incomplete")
	}
	for _, is6 := range []bool{false, true} {
		if is6 && s.MTU < 1280 {
			continue
		}
		expected := s.Carrier.VPP4()
		if is6 {
			expected = s.Carrier.VPP6()
		}
		wanted := netip.MustParsePrefix(expected)
		addrs, e := ipapi.NewServiceClient(rt.vpp).IPAddressDump(ctx, &ipapi.IPAddressDump{SwIfIndex: interface_types.InterfaceIndex(transit), IsIPv6: is6})
		if e != nil {
			return e
		}
		found := false
		for {
			row, e := addrs.Recv()
			if errors.Is(e, io.EOF) {
				break
			}
			if e != nil {
				return e
			}
			got, e := netip.ParsePrefix(row.Prefix.String())
			if e == nil && got == wanted {
				found = true
			}
		}
		if !found {
			return errors.New("PPP transit connected address is absent")
		}
	}
	return nil
}

// carrierSessionGeneration changes for every NCP-up event, including redial
// inside one persistent pppd process (where systemd InvocationID stays fixed).
func (rt *PppoeRuntime) carrierSessionGeneration(s pppoe.Session) (string, error) {
	parts := []string{}
	suffixes := []string{".state"}
	if s.IPv6Enabled() {
		suffixes = append(suffixes, ".state6")
	}
	for _, suffix := range suffixes {
		body, err := os.ReadFile(filepath.Join(rt.sessionStateDir(s), s.HostIf+suffix))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if len(body) > 65536 {
			return "", errors.New("PPP session evidence exceeds its bound")
		}
		values := map[string]string{}
		for _, line := range strings.Split(string(body), "\n") {
			k, v, ok := strings.Cut(line, "=")
			if ok {
				values[k] = v
			}
		}
		if values["phase"] != "up" {
			continue
		}
		generation := strings.ReplaceAll(values["session_generation"], "-", "")
		decoded, err := hex.DecodeString(generation)
		if err != nil || len(decoded) != 16 {
			return "", errors.New("PPP NCP generation is absent")
		}
		parts = append(parts, generation)
	}
	if len(parts) == 0 {
		return "", errors.New("PPP NCP is not up")
	}
	return strings.Join(parts, "."), nil
}
