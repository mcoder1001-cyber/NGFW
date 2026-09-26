package desired

// The `services` sub-keys this build projects, and the agent.unsupported-field notes for the others (so that
// /state/drift does not compare what the agent never applies). F-qos-flat carries the same two declarations as
// F-kea-dhcp-relay's desired/kea.go and F-unbound-chrony-syslog's desired/dns_services.go (the first `services`
// feature to merge keeps them; the others keep only their `ServicesImplemented["<key>"] = true` init line and call
// ServicesUnsupported once per projection) — F-qos-flat-questions.md Q4.

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// ServicesImplemented lists the `services` sub-keys (JSON names) that this build projects. A feature that projects
// another sub-key adds it from an init() in its own file (e.g. `ServicesImplemented["qos"] = true`).
var ServicesImplemented = map[string]bool{}

// ServicesUnsupported warns agent.unsupported-field for every non-empty `services` sub-key that no builder projects.
func ServicesUnsupported(s Sink, svc *vrxv1.ServicesConfig) {
	if svc == nil {
		return
	}
	m := svc.ProtoReflect()
	fields := m.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if ServicesImplemented[fd.JSONName()] || !m.Has(fd) {
			continue
		}
		if fd.Kind() == protoreflect.MessageKind && proto.Size(m.Get(fd).Message().Interface()) == 0 {
			continue
		}
		s.Warnf(Ptr("services", fd.JSONName()), "agent.unsupported-field", "services.%s is not applied by this agent build", fd.JSONName())
	}
}
