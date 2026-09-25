package desired

// MERGE SEAM (F-lb) — DELETE THIS FILE when task/F-rpf-adl-pbr is on the base.
//
// F-rpf-adl-pbr registers the `services` domain first and owns its seam (desired/rpf_adl_pbr.go: `ServicesMembers`, the
// append-only set of implemented services members, and projectServices, which reports every other non-empty member
// as agent.unsupported-field). task/F-lb is based on main, which does not contain it, so this file carries an
// equivalent copy of exactly those two pieces (the F-loopback-bvi-gso-lldp-span precedent: reuse F-rpf-adl-pbr's
// seam, no second Services constant in desired). After deleting it nothing else changes: lb.go's init() registers
// "lb" in F-rpf-adl-pbr's map, lbReportUnsupportedServices stays nil and projectServices does the reporting.

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// ServicesMembers are the `services.*` members (JSON names) this agent build implements. Append-only: a feature that
// implements a member adds it from an init() in its own file (`func init() { ServicesMembers["lb"] = true }`); every
// other non-empty member is reported as agent.unsupported-field.
var ServicesMembers = map[string]bool{}

func init() { lbReportUnsupportedServices = lbUnsupportedServicesSeam }

// lbUnsupportedServicesSeam reports the services members no feature implements (F-rpf-adl-pbr's loop).
func lbUnsupportedServicesSeam(s Sink, svc *vrxv1.ServicesConfig) {
	svc.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if ServicesMembers[fd.JSONName()] || fd.Message() == nil || proto.Size(v.Message().Interface()) == 0 {
			return true
		}
		s.Warnf(Ptr("services", fd.JSONName()), lbRuleUnsupport, "services.%s is not implemented by this agent build", fd.JSONName())
		return true
	})
}
