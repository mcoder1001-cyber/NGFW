// Package redistribute reads a scoped summary, never a full RIB, and projects configured redistribution edges.
package redistribute

import (
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers/frr"
	"sort"
)

// Reader identifies the scoped redistribution summary observation.
const Reader = "redistributionSummary"

func init() {
	frr.RegisterStateReader(frr.StateReader{Key: Reader, Command: frr.ShowIPSummaryAll})
}

// Edges returns deterministically ordered configured protocol redistribution edges.
func Edges(r *ngfwv1.RoutingConfig) []*ngfwv1.RedistributionEdge {
	var out []*ngfwv1.RedistributionEdge
	add := func(target, vrf string, d *ngfwv1.Redistribute, readonly bool) {
		if vrf == "" {
			vrf = "default"
		}
		for source, opt := range map[string]*ngfwv1.RedistributeOptions{"connected": d.GetConnected(), "static": d.GetStatic(), "bgp": d.GetBgp(), "ospf": d.GetOspf(), "isis": d.GetIsis(), "rip": d.GetRip()} {
			if opt == nil {
				continue
			}
			e := &ngfwv1.RedistributionEdge{Source: source, Target: target, Vrf: vrf, RouteMap: opt.GetRouteMap(), ReadOnly: readonly}
			if opt.Metric != nil {
				e.Metric = proto.Uint32(opt.GetMetric())
			}
			out = append(out, e)
		}
	}
	add("bgp", r.GetBgp().GetVrf(), r.GetBgp().GetRedistribute(), false)
	add("ospf", r.GetOspf().GetVrf(), r.GetOspf().GetRedistribute(), false)
	add("isis", r.GetIsis().GetVrf(), r.GetIsis().GetRedistribute(), false)
	add("rip", r.GetRip().GetVrf(), r.GetRip().GetRedistribute(), false)
	add("ospf6", r.GetOspf6().GetVrf(), r.GetOspf6().GetRedistribute(), true)
	add("ripng", r.GetRipng().GetVrf(), r.GetRipng().GetRedistribute(), true)
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		return a.Source < b.Source
	})
	return out
}
