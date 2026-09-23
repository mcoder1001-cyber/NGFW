package vpn

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/vpp"
)

// NoInterface is the "no interface" sw_if_index (~0).
const NoInterface = ^uint32(0)

// Interface is one row of sw_interface_dump: what the descriptors need to translate names to
// indices and to read the owner tag.
type Interface struct {
	Name      string
	Tag       string
	SwIfIndex uint32
}

// Interfaces is the interface table of one sw_interface_dump.
type Interfaces struct {
	byName  map[string]*Interface
	byIndex map[uint32]*Interface
}

// DumpInterfaces reads every interface (sw_interface_dump with sw_if_index ~0).
func DumpInterfaces(ctx context.Context, c vpp.Client) (*Interfaces, error) {
	stream, err := interfaces.NewServiceClient(c).SwInterfaceDump(ctx, &interfaces.SwInterfaceDump{
		SwIfIndex: interface_types.InterfaceIndex(NoInterface),
	})
	if err != nil {
		return nil, fmt.Errorf("sw_interface_dump: %w", err)
	}
	t := &Interfaces{byName: map[string]*Interface{}, byIndex: map[uint32]*Interface{}}
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return t, nil
		}
		if err != nil {
			return nil, fmt.Errorf("sw_interface_dump: %w", err)
		}
		i := &Interface{
			Name:      strings.TrimRight(d.InterfaceName, "\x00"),
			Tag:       strings.TrimRight(d.Tag, "\x00"),
			SwIfIndex: uint32(d.SwIfIndex),
		}
		t.byName[i.Name] = i
		t.byIndex[i.SwIfIndex] = i
	}
}

// ByName returns the interface called name.
func (t *Interfaces) ByName(name string) (*Interface, bool) {
	i, ok := t.byName[name]
	return i, ok
}

// ByIndex returns the interface with sw_if_index idx.
func (t *Interfaces) ByIndex(idx uint32) (*Interface, bool) {
	i, ok := t.byIndex[idx]
	return i, ok
}

// Index returns the sw_if_index of name or an error naming the missing interface.
func (t *Interfaces) Index(name string) (interface_types.InterfaceIndex, error) {
	i, ok := t.byName[name]
	if !ok {
		return 0, fmt.Errorf("vpn: interface %q does not exist in VPP", name)
	}
	return interface_types.InterfaceIndex(i.SwIfIndex), nil
}

// Name returns the name of sw_if_index idx ("" when unknown or NoInterface).
func (t *Interfaces) Name(idx uint32) string {
	if i, ok := t.byIndex[idx]; ok {
		return i.Name
	}
	return ""
}

// Owned reports whether the interface idx carries owner's tag and returns the id part of the tag.
func (t *Interfaces) Owned(idx uint32, owner string) (string, bool) {
	i, ok := t.byIndex[idx]
	if !ok {
		return "", false
	}
	return vpp.ParseOwnerTag(i.Tag, owner)
}

// TagInterface stamps the interface with the owner tag "<owner>:<id>" (sw_interface_tag_add_del).
func TagInterface(ctx context.Context, c vpp.Client, idx interface_types.InterfaceIndex, owner, id string) error {
	tag, err := vpp.OwnerTag(owner, id)
	if err != nil {
		return err
	}
	if _, err := interfaces.NewServiceClient(c).SwInterfaceTagAddDel(ctx, &interfaces.SwInterfaceTagAddDel{
		IsAdd: true, SwIfIndex: idx, Tag: tag,
	}); err != nil {
		return fmt.Errorf("sw_interface_tag_add_del: %w", err)
	}
	return nil
}
