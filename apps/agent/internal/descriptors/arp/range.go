// Package arp implements the descriptors of VPP's arp API: proxy-ARP address ranges and the
// per-interface proxy-ARP switch. Messages come from apps/agent/binapi/arp only.
package arp

import (
	"context"
	"fmt"
	"net/netip"
	"strconv"

	"google.golang.org/protobuf/proto"

	arpapi "ngfw/agent/binapi/arp"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// RangeName is the descriptor name; keys are "arp.proxy-range/<table>/<low>-<high>".
const RangeName = "arp.proxy-range"

// RangeDescriptor manages proxy-ARP ranges (proxy_arp_add_del). Ranges carry no tag: they
// are attributed to this agent by their FIB table id (df2.IDRange; nil = every table).
type RangeDescriptor struct {
	client vpp.Client
	tables *df2.IDRange
}

// NewRange returns the descriptor; tables scopes ownership on a shared VPP.
func NewRange(c vpp.Client, tables *df2.IDRange) *RangeDescriptor {
	return &RangeDescriptor{client: c, tables: tables}
}

// Name implements scheduler.Descriptor.
func (*RangeDescriptor) Name() string { return RangeName }

func canonIP4(s string) string {
	if a, err := df2.ParseAddr(s); err == nil {
		return a.String()
	}
	return s
}

// KeyOf implements scheduler.Descriptor.
func (*RangeDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	r := obj.(*ProxyRange)
	return scheduler.Join(RangeName, strconv.FormatUint(uint64(r.GetTableId()), 10), canonIP4(r.GetLow())+"-"+canonIP4(r.GetHigh()))
}

// Dependencies implements scheduler.Descriptor: the table must exist (table 0 always does).
func (*RangeDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	return df2.VRFDeps(obj.(*ProxyRange).GetTableId())
}

func encodeRange(r *ProxyRange) (arpapi.ProxyArp, error) {
	lo, err := df2.ParseAddr(r.GetLow())
	if err != nil {
		return arpapi.ProxyArp{}, err
	}
	hi, err := df2.ParseAddr(r.GetHigh())
	if err != nil {
		return arpapi.ProxyArp{}, err
	}
	lo4, err := df2.ToIP4(lo)
	if err != nil {
		return arpapi.ProxyArp{}, err
	}
	hi4, err := df2.ToIP4(hi)
	if err != nil {
		return arpapi.ProxyArp{}, err
	}
	if hi.Less(lo) {
		return arpapi.ProxyArp{}, fmt.Errorf("proxy-arp range %s-%s: high below low", lo, hi)
	}
	return arpapi.ProxyArp{TableID: r.GetTableId(), Low: lo4, Hi: hi4}, nil
}

func (d *RangeDescriptor) addDel(ctx context.Context, r *ProxyRange, isAdd bool) error {
	p, err := encodeRange(r)
	if err != nil {
		return err
	}
	if _, err := arpapi.NewServiceClient(d.client).ProxyArpAddDel(ctx, &arpapi.ProxyArpAddDel{IsAdd: isAdd, Proxy: p}); err != nil {
		return fmt.Errorf("proxy_arp_add_del: %w", err)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *RangeDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	r := obj.(*ProxyRange)
	if !d.tables.Owns(r.GetTableId()) {
		return nil, fmt.Errorf("%s: table %d outside this agent's range", RangeName, r.GetTableId())
	}
	return nil, d.addDel(ctx, r, true)
}

// Update implements scheduler.Descriptor: every field is part of the key.
func (*RangeDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor.
func (d *RangeDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	return d.addDel(ctx, obj.(*ProxyRange), false)
}

// Retrieve dumps every range and keeps those in owned tables.
func (d *RangeDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	stream, err := arpapi.NewServiceClient(d.client).ProxyArpDump(ctx, &arpapi.ProxyArpDump{})
	if err != nil {
		return nil, fmt.Errorf("proxy_arp_dump: %w", err)
	}
	details, err := df2.Collect(stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("proxy_arp_dump: %w", err)
	}
	var out []scheduler.KV
	for _, det := range details {
		if !d.tables.Owns(det.Proxy.TableID) {
			continue
		}
		v := &ProxyRange{TableId: det.Proxy.TableID, Low: netip.AddrFrom4(det.Proxy.Low).String(), High: netip.AddrFrom4(det.Proxy.Hi).String()}
		out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v})
	}
	return out, nil
}
