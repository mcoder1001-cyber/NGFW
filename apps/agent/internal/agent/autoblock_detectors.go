package agent

import (
	"bytes"
	"context"
	"net/netip"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/autoblock"
	"ngfw/agent/internal/descriptors/ikev2"
	"ngfw/agent/internal/detectors"
	"ngfw/agent/internal/renderers"
	"strconv"
	"time"
)

const journalBin = "/usr/bin/journalctl"

// Only the product agent reads the host journal, through the fixed-binary runner.
func (a *Agent) watchHostDetectors(ctx context.Context) {
	if a.svc.owner != "ngfw" {
		return
	}
	runner := &renderers.SystemRunner{Allow: renderers.NewAllowlist(journalBin), MaxOutput: 4 << 20}
	host := detectors.NewHost()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	since := a.svc.now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			now := a.svc.now()
			var cfg *ngfwv1.AutoBlock
			var ds *ngfwv1.DesiredState
			func() {
				if a.svc.lock(ctx) != nil {
					return
				}
				defer a.svc.unlock()
				ds = a.svc.st.desired
				cfg = ds.GetSecurity().GetAutoBlock()
			}()
			if !cfg.GetEnabled() {
				since = now
				continue
			}
			out, err := runner.Run(ctx, renderers.Command{Path: journalBin, Args: []string{"--no-pager", "--quiet", "--output=json", "--output-fields=MESSAGE,_COMM,_UID,_TRANSPORT,__CURSOR,__REALTIME_TIMESTAMP", "--lines=5000", "--since=@" + strconv.FormatInt(since.Unix(), 10)}, Timeout: 5 * time.Second})
			if err != nil {
				a.svc.log.Warn("auto-block journal unavailable", "err", err)
				continue
			}
			window := time.Duration(0)
			threshold := 0
			enabled := map[string]bool{}
			for _, r := range cfg.GetRules() {
				if !r.GetEnabled() {
					continue
				}
				enabled[r.GetSource()] = true
				if r.GetSource() == "portScan" {
					window = time.Duration(r.GetWindowSec()) * time.Second
					threshold = int(r.GetThreshold())
				}
			}
			for _, line := range bytes.Split(out.Stdout, []byte{'\n'}) {
				if ob := host.Observe(line, now, window, threshold, int(cfg.GetMaxEntries())); ob != nil && journalObservationEnabled(ob.Kind, enabled) {
					addr, err := netip.ParseAddr(ob.Source)
					if err != nil || autoblock.Allowed(ds, addr) {
						continue
					}
					a.svc.bus.publish(autoBlockObserved(*ob))
				}
			}
			since = now.Add(-time.Second)
		}
	}
}

// Native VPN failures come from the existing secret-safe SA reader. An unpatched
// state API stays unavailable; raw secret-bearing dumps are never a fallback.
func (a *Agent) watchNativeAutoBlock(ctx context.Context) {
	detector := detectors.NewNative()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	reported := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			var ds *ngfwv1.DesiredState
			func() {
				if a.svc.lock(ctx) != nil {
					return
				}
				defer a.svc.unlock()
				ds = a.svc.st.desired
			}()
			cfg := ds.GetSecurity().GetAutoBlock()
			enabled := false
			for _, r := range cfg.GetRules() {
				if r.GetEnabled() && r.GetSource() == "vpnAuth" {
					enabled = true
				}
			}
			if !cfg.GetEnabled() || !enabled || len(ds.GetVpn().GetIpsec().GetTunnels()) == 0 {
				continue
			}
			c, cancel := context.WithTimeout(ctx, 5*time.Second)
			nativeStateMu.Lock()
			states, err := ikev2.SAs(c, a.svc.vpp, a.svc.owner)
			nativeStateMu.Unlock()
			cancel()
			if err != nil {
				if !reported {
					a.svc.log.Warn("native VPN auto-block detector unavailable", "err", err)
					reported = true
				}
				continue
			}
			reported = false
			for _, ob := range detector.Observe(states, ds, a.svc.now()) {
				addr, err := netip.ParseAddr(ob.Source)
				if err != nil || autoblock.Allowed(ds, addr) {
					continue
				}
				a.svc.bus.publish(&ngfwv1.Event{Kind: ngfwv1.EventKind_EVENT_KIND_AUTOBLOCK_OBSERVED, Attributes: map[string]string{"source_ip": ob.Source, "detector": ob.Kind}})
			}
		}
	}
}

// Product VPN is native VPP IKEv2. Root charon logs belong to test peers or
// foreign daemons and never authorize a product runtime block.
func journalObservationEnabled(kind string, enabled map[string]bool) bool {
	return (kind == "ssh" || kind == "portScan") && enabled[kind]
}
