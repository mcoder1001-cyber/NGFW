package ravpn

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"ngfw/agent/binapi/interface_types"
	tapapi "ngfw/agent/binapi/tapv2"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
)

// RetiredVPPBoot positively proves the old process cannot still own its TAP.
// Unknown /proc identity is not proof of death; no process is signaled.
func RetiredVPPBoot(old bootid.Identity) bool {
	if !old.Complete() || old.PID <= 0 {
		return false
	}
	reader := bootid.Reader{}
	host := reader.BootID()
	if host == "" {
		return false
	}
	if host != old.BootID {
		return true
	}
	_, e := os.Stat(filepath.Join("/proc", strconv.Itoa(old.PID)))
	if errors.Is(e, os.ErrNotExist) {
		return true
	}
	if e != nil {
		return false
	}
	current := reader.ForPID(old.PID)
	return current.Complete() && current.PID > 0 && !current.Equal(old)
}

// VerifyTAPAbsent enumerates every owner's TAP. A numeric-ID collision, reused
// old index or same namespace/link refuses recovery without deleting anything.
func VerifyTAPAbsent(ctx context.Context, client vpp.Client, endpoint *tapv2.Tap, oldIndex uint32) (result error) {
	if client == nil || endpoint == nil {
		return ErrBoundary
	}
	stream, e := tapapi.NewServiceClient(client).SwInterfaceTapV2Dump(ctx, &tapapi.SwInterfaceTapV2Dump{SwIfIndex: interface_types.InterfaceIndex(^uint32(0))})
	if e != nil {
		return ErrBoundary
	}
	defer func() {
		if err := stream.Close(); err != nil && result == nil {
			result = ErrBoundary
		}
	}()
	for count := 0; count <= 8193; count++ {
		row, e := stream.Recv()
		if errors.Is(e, io.EOF) {
			return nil
		}
		if e != nil || row == nil {
			return ErrBoundary
		}
		if row.ID == endpoint.Id || row.SwIfIndex == oldIndex || row.HostNamespace == endpoint.HostNamespace && row.HostIfName == endpoint.HostIfName {
			return ErrBoundary
		}
	}
	return ErrBoundary
}
