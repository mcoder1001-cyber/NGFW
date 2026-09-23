package df7

import (
	"fmt"
	"net/netip"
	"sort"

	"ngfw/agent/binapi/fib_types"
	"ngfw/agent/binapi/ip_types"
)

// Path types a desired FIB path may use (fib_types.FibPathType); "" = normal.
const (
	PathNormal  = ""
	PathDrop    = "drop"
	PathLocal   = "local"
	PathUnreach = "icmp-unreach"
	PathProhib  = "icmp-prohibit"
)

var pathTypes = map[string]fib_types.FibPathType{
	PathNormal:  fib_types.FIB_API_PATH_TYPE_NORMAL,
	PathDrop:    fib_types.FIB_API_PATH_TYPE_DROP,
	PathLocal:   fib_types.FIB_API_PATH_TYPE_LOCAL,
	PathUnreach: fib_types.FIB_API_PATH_TYPE_ICMP_UNREACH,
	PathProhib:  fib_types.FIB_API_PATH_TYPE_ICMP_PROHIBIT,
}

// Next-hop protocols (fib_types.FibPathNhProto); "" = derived from NextHop, else ip4.
const (
	ProtoIP4      = "ip4"
	ProtoIP6      = "ip6"
	ProtoMPLS     = "mpls"
	ProtoEthernet = "ethernet"
)

var pathProtos = map[string]fib_types.FibPathNhProto{
	ProtoIP4:      fib_types.FIB_API_PATH_NH_PROTO_IP4,
	ProtoIP6:      fib_types.FIB_API_PATH_NH_PROTO_IP6,
	ProtoMPLS:     fib_types.FIB_API_PATH_NH_PROTO_MPLS,
	ProtoEthernet: fib_types.FIB_API_PATH_NH_PROTO_ETHERNET,
}

// MaxLabels is the label stack depth of fib_types.fib_path (label_stack[16]).
const MaxLabels = 16

// MaxLabel is the largest 20-bit MPLS label.
const MaxLabel = 1<<20 - 1

// Label is one out-label of a path's label stack. The uniform/pipe mode (fib_mpls_label
// is_uniform) is not reported by VPP's path encoder, so only pipe mode is supported.
type Label struct {
	Label uint32 `json:"label,omitempty"`
	TTL   uint8  `json:"ttl,omitempty"`
	Exp   uint8  `json:"exp,omitempty"`
}

// Path is one FIB path in canonical form (NormalizePaths): Interface and NextHop for an
// attached next hop; NextHop + TableID for a recursive next hop; TableID alone for a lookup in
// that table (deag); Type for drop/local/icmp; Labels = the out-label stack pushed on the
// path. Weight 0 is stored as 1 (VPP's default).
type Path struct {
	Type               string  `json:"type,omitempty"`
	Proto              string  `json:"proto,omitempty"`
	Interface          string  `json:"interface,omitempty"`
	NextHop            string  `json:"next_hop,omitempty"`
	TableID            uint32  `json:"table_id,omitempty"`
	Weight             uint8   `json:"weight,omitempty"`
	Preference         uint8   `json:"preference,omitempty"`
	ResolveViaHost     bool    `json:"resolve_via_host,omitempty"`
	ResolveViaAttached bool    `json:"resolve_via_attached,omitempty"`
	Labels             []Label `json:"labels,omitempty"`
}

// NormalizePaths validates paths and returns them in the canonical form Retrieve produces:
// canonical next-hop text, proto derived from the next hop (ip4 when neither is set), weight
// 0 → 1, TableID cleared when an interface is given, sorted.
func NormalizePaths(paths []Path) ([]Path, error) {
	out := make([]Path, 0, len(paths))
	for i, p := range paths {
		n, err := normalizePath(p)
		if err != nil {
			return nil, fmt.Errorf("path %d: %w", i, err)
		}
		out = append(out, n)
	}
	SortPaths(out)
	return out, nil
}

func normalizePath(p Path) (Path, error) {
	if _, ok := pathTypes[p.Type]; !ok {
		return Path{}, Specf("unknown path type %q", p.Type)
	}
	if p.NextHop != "" {
		a, err := ParseAddr(p.NextHop)
		if err != nil {
			return Path{}, err
		}
		p.NextHop = a.String()
		want := ProtoIP4
		if a.Is6() {
			want = ProtoIP6
		}
		if p.Proto != "" && p.Proto != want {
			return Path{}, Specf("next hop %s is %s, path proto is %q", a, want, p.Proto)
		}
		p.Proto = want
	}
	if p.Proto == "" {
		p.Proto = ProtoIP4
	}
	if _, ok := pathProtos[p.Proto]; !ok {
		return Path{}, Specf("unknown path proto %q", p.Proto)
	}
	if p.Weight == 0 {
		p.Weight = 1
	}
	if p.Interface != "" || (p.Proto != ProtoIP4 && p.Proto != ProtoIP6) {
		p.TableID = 0
	}
	if len(p.Labels) > MaxLabels {
		return Path{}, Specf("%d labels, VPP takes at most %d", len(p.Labels), MaxLabels)
	}
	for _, l := range p.Labels {
		if l.Label > MaxLabel {
			return Path{}, Specf("label %d exceeds 20 bits", l.Label)
		}
		if l.Exp > 7 {
			return Path{}, Specf("label %d exp %d exceeds 3 bits", l.Label, l.Exp)
		}
	}
	if len(p.Labels) == 0 {
		p.Labels = nil
	}
	return p, nil
}

func pathKey(p Path) string {
	return fmt.Sprintf("%s|%s|%s|%s|%010d|%03d|%03d|%v", p.Type, p.Proto, p.NextHop, p.Interface, p.TableID, p.Weight, p.Preference, p.Labels)
}

// SortPaths orders paths deterministically.
func SortPaths(paths []Path) {
	sort.SliceStable(paths, func(i, j int) bool { return pathKey(paths[i]) < pathKey(paths[j]) })
}

// EncodePaths converts normalised paths to fib_types paths, resolving interface names.
func EncodePaths(paths []Path, ifs *Interfaces) ([]fib_types.FibPath, error) {
	out := make([]fib_types.FibPath, 0, len(paths))
	for _, p := range paths {
		e := fib_types.FibPath{
			SwIfIndex:  NoIndex,
			TableID:    p.TableID,
			Weight:     p.Weight,
			Preference: p.Preference,
			Type:       pathTypes[p.Type],
			Proto:      pathProtos[p.Proto],
		}
		if p.ResolveViaAttached {
			e.Flags |= fib_types.FIB_API_PATH_FLAG_RESOLVE_VIA_ATTACHED
		}
		if p.ResolveViaHost {
			e.Flags |= fib_types.FIB_API_PATH_FLAG_RESOLVE_VIA_HOST
		}
		if p.Interface != "" {
			idx, err := ifs.Resolve(p.Interface)
			if err != nil {
				return nil, fmt.Errorf("fib path: %w", err)
			}
			e.SwIfIndex = idx
		}
		if p.NextHop != "" {
			a, err := ParseAddr(p.NextHop)
			if err != nil {
				return nil, err
			}
			if a.Is4() {
				e.Nh.Address = ip_types.AddressUnionIP4(a.As4())
			} else {
				e.Nh.Address = ip_types.AddressUnionIP6(a.As16())
			}
		}
		e.NLabels = uint8(len(p.Labels)) //nolint:gosec // ≤ 16 (normalizePath)
		for i, l := range p.Labels {
			e.LabelStack[i] = fib_types.FibMplsLabel{Label: l.Label, TTL: l.TTL, Exp: l.Exp}
		}
		out = append(out, e)
	}
	return out, nil
}

// DecodePaths converts dumped paths to canonical form (see NormalizePaths).
func DecodePaths(paths []fib_types.FibPath, ifs *Interfaces) []Path {
	out := make([]Path, 0, len(paths))
	for _, p := range paths {
		d := Path{
			Weight:             p.Weight,
			Preference:         p.Preference,
			ResolveViaAttached: p.Flags&fib_types.FIB_API_PATH_FLAG_RESOLVE_VIA_ATTACHED != 0,
			ResolveViaHost:     p.Flags&fib_types.FIB_API_PATH_FLAG_RESOLVE_VIA_HOST != 0,
		}
		d.Type = fmt.Sprintf("#%d", p.Type)
		for name, t := range pathTypes {
			if t == p.Type {
				d.Type = name
			}
		}
		d.Proto = fmt.Sprintf("#%d", p.Proto)
		for name, pr := range pathProtos {
			if pr == p.Proto {
				d.Proto = name
			}
		}
		if d.Weight == 0 {
			d.Weight = 1
		}
		if p.SwIfIndex != NoIndex {
			d.Interface = ifs.Name(p.SwIfIndex)
		} else if d.Proto == ProtoIP4 || d.Proto == ProtoIP6 {
			d.TableID = p.TableID
		}
		var nh netip.Addr
		switch p.Proto {
		case fib_types.FIB_API_PATH_NH_PROTO_IP4:
			nh = netip.AddrFrom4(p.Nh.Address.GetIP4())
		case fib_types.FIB_API_PATH_NH_PROTO_IP6:
			nh = netip.AddrFrom16(p.Nh.Address.GetIP6())
		}
		if nh.IsValid() && !nh.IsUnspecified() {
			d.NextHop = nh.String()
		}
		n := int(p.NLabels)
		if n > MaxLabels {
			n = MaxLabels
		}
		for i := 0; i < n; i++ {
			l := p.LabelStack[i]
			d.Labels = append(d.Labels, Label{Label: l.Label, TTL: l.TTL, Exp: l.Exp})
		}
		out = append(out, d)
	}
	SortPaths(out)
	return out
}

// PathInterfaces returns the interface names the paths reference (for Dependencies).
func PathInterfaces(paths []Path) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range paths {
		if p.Interface != "" && !seen[p.Interface] {
			seen[p.Interface] = true
			out = append(out, p.Interface)
		}
	}
	return out
}
