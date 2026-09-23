package df2

import (
	"fmt"
	"net/netip"
	"sort"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/fib_types"
	"ngfw/agent/binapi/ip_types"
)

// NormalizePaths returns the canonical form of a path list, the form Retrieve produces:
// addresses in netip text, weight 0 → 1, proto derived from the next hop, table_id cleared on
// attached paths, sorted. Callers normalise desired paths with it before diffing.
func NormalizePaths(paths []*FibPath) ([]*FibPath, error) {
	out := make([]*FibPath, 0, len(paths))
	for _, p := range paths {
		n, err := normalizePath(p)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	sortPaths(out)
	return out, nil
}

func normalizePath(p *FibPath) (*FibPath, error) {
	n := proto.Clone(p).(*FibPath)
	if n.NextHop != "" {
		a, err := ParseAddr(n.NextHop)
		if err != nil {
			return nil, err
		}
		n.NextHop = a.String()
		if a.Is4() {
			n.Proto = FibPath_IP4
		} else {
			n.Proto = FibPath_IP6
		}
	}
	if n.Weight == 0 {
		n.Weight = 1
	}
	if n.Interface != "" {
		n.TableId = 0
	}
	return n, nil
}

func pathLess(a, b *FibPath) bool {
	switch {
	case a.Type != b.Type:
		return a.Type < b.Type
	case a.Proto != b.Proto:
		return a.Proto < b.Proto
	case a.NextHop != b.NextHop:
		return a.NextHop < b.NextHop
	case a.Interface != b.Interface:
		return a.Interface < b.Interface
	case a.TableId != b.TableId:
		return a.TableId < b.TableId
	case a.Weight != b.Weight:
		return a.Weight < b.Weight
	default:
		return a.Preference < b.Preference
	}
}

func sortPaths(paths []*FibPath) {
	sort.SliceStable(paths, func(i, j int) bool { return pathLess(paths[i], paths[j]) })
}

// EncodePaths converts normalised desired paths to fib_types paths, resolving interface names.
func EncodePaths(paths []*FibPath, ifs *Interfaces) ([]fib_types.FibPath, error) {
	out := make([]fib_types.FibPath, 0, len(paths))
	for _, p := range paths {
		e, err := EncodePath(p, ifs)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// EncodePath converts one desired path to fib_types.FibPath.
func EncodePath(p *FibPath, ifs *Interfaces) (fib_types.FibPath, error) {
	if p.Weight > 255 || p.Preference > 255 {
		return fib_types.FibPath{}, fmt.Errorf("fib path: weight/preference %d/%d exceed 255", p.Weight, p.Preference)
	}
	out := fib_types.FibPath{
		SwIfIndex:  NoInterface,
		TableID:    p.TableId,
		Weight:     uint8(p.Weight),     //nolint:gosec // checked above
		Preference: uint8(p.Preference), //nolint:gosec // checked above
		Type:       fib_types.FibPathType(p.Type),
		Proto:      fib_types.FibPathNhProto(p.Proto),
	}
	if p.ResolveViaAttached {
		out.Flags |= fib_types.FIB_API_PATH_FLAG_RESOLVE_VIA_ATTACHED
	}
	if p.ResolveViaHost {
		out.Flags |= fib_types.FIB_API_PATH_FLAG_RESOLVE_VIA_HOST
	}
	if p.Interface != "" {
		idx, err := ifs.Index(p.Interface)
		if err != nil {
			return fib_types.FibPath{}, fmt.Errorf("fib path: %w", err)
		}
		out.SwIfIndex = uint32(idx)
	}
	if p.NextHop != "" {
		a, err := ParseAddr(p.NextHop)
		if err != nil {
			return fib_types.FibPath{}, fmt.Errorf("fib path: %w", err)
		}
		if a.Is4() {
			out.Proto = fib_types.FIB_API_PATH_NH_PROTO_IP4
			out.Nh.Address = ip_types.AddressUnionIP4(a.As4())
		} else {
			out.Proto = fib_types.FIB_API_PATH_NH_PROTO_IP6
			out.Nh.Address = ip_types.AddressUnionIP6(a.As16())
		}
	}
	return out, nil
}

// DecodePaths converts dumped paths to the canonical desired form (see NormalizePaths).
func DecodePaths(paths []fib_types.FibPath, ifs *Interfaces) []*FibPath {
	out := make([]*FibPath, 0, len(paths))
	for _, p := range paths {
		out = append(out, DecodePath(p, ifs))
	}
	sortPaths(out)
	return out
}

// DecodePath converts one dumped path. An unknown sw_if_index decodes to "#<index>" so the
// diff shows the problem instead of silently matching.
func DecodePath(p fib_types.FibPath, ifs *Interfaces) *FibPath {
	out := &FibPath{
		Weight:             uint32(p.Weight),
		Preference:         uint32(p.Preference),
		Type:               FibPath_Type(p.Type),   //nolint:gosec // enum values 0..10
		Proto:              FibPath_Proto(p.Proto), //nolint:gosec // enum values 0..4
		ResolveViaAttached: p.Flags&fib_types.FIB_API_PATH_FLAG_RESOLVE_VIA_ATTACHED != 0,
		ResolveViaHost:     p.Flags&fib_types.FIB_API_PATH_FLAG_RESOLVE_VIA_HOST != 0,
	}
	if out.Weight == 0 {
		out.Weight = 1
	}
	if p.SwIfIndex != NoInterface {
		if name, ok := ifs.Name(p.SwIfIndex); ok {
			out.Interface = name
		} else {
			out.Interface = fmt.Sprintf("#%d", p.SwIfIndex)
		}
	} else {
		out.TableId = p.TableID
	}
	var nh netip.Addr
	switch p.Proto {
	case fib_types.FIB_API_PATH_NH_PROTO_IP4:
		nh = netip.AddrFrom4(p.Nh.Address.GetIP4())
	case fib_types.FIB_API_PATH_NH_PROTO_IP6:
		nh = netip.AddrFrom16(p.Nh.Address.GetIP6())
	}
	if nh.IsValid() && !nh.IsUnspecified() {
		out.NextHop = nh.String()
	}
	return out
}
