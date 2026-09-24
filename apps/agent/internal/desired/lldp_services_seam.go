package desired

// MERGE SEAM (F-loopback-bvi-gso-lldp-span) — DELETE THIS FILE when task/F-rpf-adl-pbr is on the base.
//
// F-rpf-adl-pbr registers the `services` domain first and owns its seam (desired/rpf_adl_pbr.go:
// `ServicesMembers`, the append-only set of implemented services members, and projectServices, which
// reports every other non-empty member as agent.unsupported-field). This branch is based on
// task/F-bridge-l2, which does not contain it, so this file carries an equivalent copy of exactly those
// two pieces (manager instruction 23:30: reuse F-rpf-adl-pbr's seam, no second Services constant).
// After deleting it, nothing else changes: lldp.go's init() registers "lldp"/"nsim" in F-rpf-adl-pbr's
// map, reportUnsupportedServices stays nil, and F-rpf-adl-pbr's projectServices does the reporting.

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// ServicesMembers are the `services.*` members (JSON names) this agent build implements. Append-only: a feature
// that implements a member adds it from an init() in its own file (`func init() { ServicesMembers["nsim"] = true }`);
// every other non-empty member is reported as agent.unsupported-field.
var ServicesMembers = map[string]bool{}

func init() { reportUnsupportedServices = unsupportedServicesSeam }

// unsupportedServicesSeam reports the services members no feature implements (F-rpf-adl-pbr's loop).
func unsupportedServicesSeam(s Sink, svc *vrxv1.ServicesConfig) {
	svc.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if ServicesMembers[fd.JSONName()] || fd.Message() == nil || proto.Size(v.Message().Interface()) == 0 {
			return true
		}
		s.Warnf(Ptr("services", fd.JSONName()), lbgsUnsupported, "services.%s is not implemented by this agent build", fd.JSONName())
		return true
	})
}
