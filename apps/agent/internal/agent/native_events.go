package agent

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/ikev2"
)

// Native SA events contain public identities and transitions only. Packet counters,
// uptime and authentication material never affect the event identity.
func nativeSASnapshot(states []ikev2.SAState) (map[string]string, error) {
	if len(states) > ipsecMaxSAs {
		return nil, fmt.Errorf("native SA event observation limit")
	}
	out := make(map[string]string, len(states))
	for _, sa := range states {
		if sa.Profile == "" {
			return nil, fmt.Errorf("native SA event identity unavailable")
		}
		key := sa.Profile + "/" + strconv.FormatUint(sa.ISPI, 16)
		if _, duplicate := out[key]; duplicate {
			return nil, fmt.Errorf("duplicate native SA observation")
		}
		children := make([]string, 0, len(sa.Children))
		for _, child := range sa.Children {
			children = append(children, fmt.Sprintf("%08x:%08x", child.ISPI, child.RSPI))
		}
		sort.Strings(children)
		out[key] = sa.State + "|" + strings.Join(children, ",")
	}
	return out, nil
}

func nativeSAChanges(before, after map[string]string) []*ngfwv1.Event {
	keys := make(map[string]bool, len(before)+len(after))
	for key := range before {
		keys[key] = true
	}
	for key := range after {
		keys[key] = true
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	var out []*ngfwv1.Event
	for _, key := range ordered {
		old, existed := before[key]
		next, present := after[key]
		if existed && present && old == next {
			continue
		}
		change := "changed"
		if !existed {
			change = "up"
		} else if !present {
			change = "down"
		} else if strings.SplitN(old, "|", 2)[0] == strings.SplitN(next, "|", 2)[0] {
			change = "rekey"
		}
		tunnel, _, _ := strings.Cut(key, "/")
		out = append(out, &ngfwv1.Event{Kind: ngfwv1.EventKind_EVENT_KIND_IPSEC_SA_CHANGED,
			Message:    "native IPsec SA " + change,
			Attributes: map[string]string{"engine": "vpp-ikev2", "tunnel": tunnel, "sa": key, "change": change}})
	}
	return out
}

// Failed/unavailable readback retains the last valid baseline, so transient API
// loss cannot be reported as every tunnel going down. The first snapshot is a
// baseline; a restarted agent does not publish a synthetic negotiation event.
func watchNativeSAEvents(ctx context.Context, interval time.Duration, read func(context.Context) (map[string]string, error), publish func(*ngfwv1.Event)) {
	timer := time.NewTicker(interval)
	defer timer.Stop()
	var previous map[string]string
	for {
		callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		snapshot, err := read(callCtx)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err == nil && snapshot != nil {
			if previous != nil {
				for _, event := range nativeSAChanges(previous, snapshot) {
					publish(event)
				}
			}
			previous = snapshot
		}
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}

func (a *Agent) watchNativeIPsec(ctx context.Context) {
	watchNativeSAEvents(ctx, time.Second, func(ctx context.Context) (map[string]string, error) {
		if err := a.svc.lock(ctx); err != nil {
			return nil, err
		}
		degraded := a.svc.isDegraded()
		configured := len(a.svc.st.desired.GetVpn().GetIpsec().GetTunnels()) != 0
		a.svc.unlock()
		if degraded {
			return nil, fmt.Errorf("native state is reconciling")
		}
		if !configured {
			return map[string]string{}, nil
		}
		if !a.svc.vpp.Connected() {
			return nil, fmt.Errorf("VPP unavailable")
		}
		if err := ikev2.RequireSafeState(ctx, a.svc.vpp); err != nil {
			return nil, err
		}
		states, err := ikev2.SAs(ctx, a.svc.vpp, a.svc.owner)
		if err != nil {
			return nil, err
		}
		return nativeSASnapshot(states)
	}, a.svc.events().publishFeature)
}
