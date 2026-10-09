package multiwan

import (
	"google.golang.org/protobuf/proto"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/pppoe"
	"ngfw/agent/internal/scheduler"
)

// RouteDescriptor orders WAN route writes after a pending PPP membership change.
// The client withdraws its standalone default before WAN owns that FIB prefix.
// Optional permits installations with no PPP client; inverse ordering removes
// WAN paths before rollback or removal can restore standalone PPP policy.
type RouteDescriptor struct{ *core.RouteDescriptor }

// Dependencies declares prerequisite objects for safe reconciliation.
func (d *RouteDescriptor) Dependencies(value proto.Message) []scheduler.Dependency {
	return append(d.RouteDescriptor.Dependencies(value), scheduler.Dependency{Key: pppoe.ClientConfigKey, Optional: true})
}
