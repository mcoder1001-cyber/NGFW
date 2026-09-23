package natcommon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"strings"

	"ngfw/agent/binapi/interface_types"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/internal/vpp"
)

// Iface is one VPP interface as the NAT descriptors need it.
type Iface struct {
	SwIfIndex uint32
	Name      string
	Tag       string
}

// IfaceTable is a snapshot of sw_interface_dump indexed both ways.
type IfaceTable struct {
	byIndex map[uint32]Iface
	byName  map[string]Iface
}

// ErrNoSuchInterface is returned when an interface name is not present on the VPP.
var ErrNoSuchInterface = errors.New("natcommon: no such interface")

// DumpInterfaces takes a full sw_interface_dump snapshot.
func DumpInterfaces(ctx context.Context, c vpp.Client) (*IfaceTable, error) {
	stream, err := interfaces.NewServiceClient(c).SwInterfaceDump(ctx, &interfaces.SwInterfaceDump{SwIfIndex: ^interface_types.InterfaceIndex(0)})
	if err != nil {
		return nil, fmt.Errorf("sw_interface_dump: %w", err)
	}
	t := &IfaceTable{byIndex: map[uint32]Iface{}, byName: map[string]Iface{}}
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return t, nil
		}
		if err != nil {
			return nil, fmt.Errorf("sw_interface_dump: %w", err)
		}
		i := Iface{SwIfIndex: uint32(d.SwIfIndex), Name: strings.TrimRight(d.InterfaceName, "\x00"), Tag: strings.TrimRight(d.Tag, "\x00")}
		t.byIndex[i.SwIfIndex] = i
		t.byName[i.Name] = i
	}
}

// ByIndex looks an interface up by sw_if_index.
func (t *IfaceTable) ByIndex(idx uint32) (Iface, bool) {
	i, ok := t.byIndex[idx]
	return i, ok
}

// ByName looks an interface up by its VPP name.
func (t *IfaceTable) ByName(name string) (Iface, bool) {
	i, ok := t.byName[name]
	return i, ok
}

// Name returns the interface name for idx, or "sw_if_index:<n>" when unknown (an interface
// deleted between two dumps); the caller decides whether that is an error.
func (t *IfaceTable) Name(idx uint32) string {
	if i, ok := t.byIndex[idx]; ok {
		return i.Name
	}
	return fmt.Sprintf("sw_if_index:%d", idx)
}

// ResolveInterface returns the sw_if_index of the interface called name. VPP's name filter
// is a substring match, so the result is compared exactly.
func ResolveInterface(ctx context.Context, c vpp.Client, name string) (interface_types.InterfaceIndex, error) {
	if name == "" {
		return 0, fmt.Errorf("%w: empty name", ErrNoSuchInterface)
	}
	stream, err := interfaces.NewServiceClient(c).SwInterfaceDump(ctx, &interfaces.SwInterfaceDump{
		SwIfIndex: ^interface_types.InterfaceIndex(0), NameFilterValid: true, NameFilter: name,
	})
	if err != nil {
		return 0, fmt.Errorf("sw_interface_dump: %w", err)
	}
	found, idx := false, interface_types.InterfaceIndex(0)
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("sw_interface_dump: %w", err)
		}
		if strings.TrimRight(d.InterfaceName, "\x00") == name {
			found, idx = true, d.SwIfIndex
		}
	}
	if !found {
		return 0, fmt.Errorf("%w: %q", ErrNoSuchInterface, name)
	}
	return idx, nil
}

// InterfaceAddresses returns the IPv4 addresses configured on the given interfaces
// (ip_address_dump). The NAT44 plugins add an interface's addresses to the pool when the
// interface is registered with *_add_del_interface_addr; pool Retrieve uses this set to tell
// those apart from explicit pool ranges.
func InterfaceAddresses(ctx context.Context, c vpp.Client, idxs []uint32) (map[netip.Addr]bool, error) {
	svc := ip.NewServiceClient(c)
	out := map[netip.Addr]bool{}
	for _, idx := range idxs {
		stream, err := svc.IPAddressDump(ctx, &ip.IPAddressDump{SwIfIndex: interface_types.InterfaceIndex(idx), IsIPv6: false})
		if err != nil {
			return nil, fmt.Errorf("ip_address_dump: %w", err)
		}
		for {
			d, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("ip_address_dump: %w", err)
			}
			if a, err := netip.ParseAddr(AddrString(ip_types.Prefix(d.Prefix).Address)); err == nil {
				out[a] = true
			}
		}
	}
	return out, nil
}
