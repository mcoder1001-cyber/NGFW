package ripng

import (
	"encoding/json"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/renderers/frr/rip"
)

const StatusReader = "ripngStatus"
const RoutesReader = "ripngRoutes"
const ShowStatus frr.ShowCommand = "show ipv6 ripng status"
const ShowRoutes frr.ShowCommand = "show ipv6 route vrf all ripng json"

func init() {
	frr.RegisterStateReader(frr.StateReader{Key: StatusReader, Command: ShowStatus, OnDemand: true, Parse: func(raw []byte) (json.RawMessage, error) { return rip.ParseStatus(raw, true) }})
	frr.RegisterStateReader(frr.StateReader{Key: RoutesReader, Command: ShowRoutes, OnDemand: true})
}
