package ripng

import (
	"encoding/json"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/renderers/frr/rip"
)

// StatusReader identifies the on-demand RIPng peer status reader.
const StatusReader = "ripngStatus"

// RoutesReader identifies the on-demand RIPng route reader.
const RoutesReader = "ripngRoutes"

// ShowStatus reads public RIPng peer status.
const ShowStatus frr.ShowCommand = "show ipv6 ripng status"

// ShowRoutes reads RIPng routes across VRFs as JSON.
const ShowRoutes frr.ShowCommand = "show ipv6 route vrf all ripng json"

func init() {
	frr.RegisterStateReader(frr.StateReader{Key: StatusReader, Command: ShowStatus, OnDemand: true, Parse: func(raw []byte) (json.RawMessage, error) { return rip.ParseStatus(raw, true) }})
	frr.RegisterStateReader(frr.StateReader{Key: RoutesReader, Command: ShowRoutes, OnDemand: true})
}
