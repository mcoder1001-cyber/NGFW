package acl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"go.fd.io/govpp/api"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/vpp"
)

// ErrNoInterface is returned (wrapped) when a desired binding names an interface VPP does not have.
var ErrNoInterface = errors.New("acl: no such interface")

// allInterfaces is the sw_if_index wildcard of the dump requests (~0).
const allInterfaces = ^uint32(0)

// ifaceInfo is what the acl descriptors need from sw_interface_details.
type ifaceInfo struct {
	Index uint32
	Name  string
	Tag   string
}

// ifaceTable maps interface names to indexes and back for one Retrieve/Create call.
type ifaceTable struct {
	byIndex map[uint32]ifaceInfo
	byName  map[string]ifaceInfo
}

// dumpInterfaces runs sw_interface_dump (all interfaces) once. Bindings are keyed by interface
// name (stable across restarts) while the plugin speaks sw_if_index, so every binding
// descriptor needs this table.
func dumpInterfaces(ctx context.Context, c vpp.Client) (*ifaceTable, error) {
	stream, err := interfaces.NewServiceClient(c).SwInterfaceDump(ctx, &interfaces.SwInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(allInterfaces)})
	if err != nil {
		return nil, fmt.Errorf("sw_interface_dump: %w", err)
	}
	details, err := drain(stream, stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("sw_interface_dump: %w", err)
	}
	t := &ifaceTable{byIndex: make(map[uint32]ifaceInfo, len(details)), byName: make(map[string]ifaceInfo, len(details))}
	for _, d := range details {
		info := ifaceInfo{
			Index: uint32(d.SwIfIndex),
			Name:  strings.TrimRight(d.InterfaceName, "\x00"),
			Tag:   strings.TrimRight(d.Tag, "\x00"),
		}
		t.byIndex[info.Index] = info
		t.byName[info.Name] = info
	}
	return t, nil
}

// index resolves an interface name; the error wraps ErrNoInterface.
func (t *ifaceTable) index(name string) (uint32, error) {
	info, ok := t.byName[name]
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrNoInterface, name)
	}
	return info.Index, nil
}

// drain reads a generated dump stream until io.EOF (the control_ping_reply). On any other error
// the stream is closed and the error returned.
func drain[T any](stream api.Stream, recv func() (T, error)) ([]T, error) {
	var out []T
	for {
		d, err := recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			_ = stream.Close()
			return nil, err
		}
		out = append(out, d)
	}
}
