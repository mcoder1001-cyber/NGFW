package coretest

import (
	"go.fd.io/govpp/api"
	tapapi "ngfw/agent/binapi/tapv2"
	"sort"
)

// AddTAP seeds a real model interface plus its exact raw TAP dump metadata.
// The raw dump includes every owner; ownership filtering belongs to descriptors.
func (v *VPP) AddTAP(name, tag string, detail tapapi.SwInterfaceTapV2Details) uint32 {
	index := v.AddInterface(name, "tap", tag)
	detail.SwIfIndex = index
	detail.DevName = name
	v.mu.Lock()
	v.raTAPs[index] = detail
	v.mu.Unlock()
	return index
}
func init() {
	RegisterExtension("remote-access-raw-tap-dump", func(v *VPP) {
		v.raTAPs = map[uint32]tapapi.SwInterfaceTapV2Details{}
		v.On("sw_interface_tap_v2_dump", func(message api.Message) ([]api.Message, error) {
			request := message.(*tapapi.SwInterfaceTapV2Dump)
			v.mu.Lock()
			defer v.mu.Unlock()
			indices := []uint32{}
			for index := range v.raTAPs {
				if _, exists := v.Ifaces[index]; exists && (uint32(request.SwIfIndex) == ^uint32(0) || uint32(request.SwIfIndex) == index) {
					indices = append(indices, index)
				}
			}
			sort.Slice(indices, func(i, j int) bool { return indices[i] < indices[j] })
			rows := make([]api.Message, 0, len(indices))
			for _, index := range indices {
				detail := v.raTAPs[index]
				rows = append(rows, &detail)
			}
			return rows, nil
		})
	})
}
