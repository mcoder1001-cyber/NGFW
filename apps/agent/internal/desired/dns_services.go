package desired

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// The `services` domain is shared by several features (F-kea-dhcp-relay, F-unbound-chrony-syslog, F-rpf-adl-pbr,
// later F-snmp …). Each projects its own sub-keys and registers them here from an init() in its own file; every
// other non-empty sub-key is reported as agent.unsupported-field, so /state/drift never compares what this agent
// does not apply. (F-kea-dhcp-relay carries the same two declarations in desired/kea.go: whichever branch merges
// second drops its copy — a duplicate declaration fails the build, never silently.)

// ServicesImplemented lists the `services` sub-keys (JSON names) this build projects.
var ServicesImplemented = map[string]bool{}

// ServicesUnsupported warns agent.unsupported-field for every non-empty `services` sub-key that no builder projects.
func ServicesUnsupported(s Sink, svc *vrxv1.ServicesConfig) {
	unsupported(s, "services", svc, ServicesImplemented)
}

// unsupported warns for every set, non-empty field of msg whose JSON name is not in implemented.
func unsupported(s Sink, root string, msg proto.Message, implemented map[string]bool) {
	if msg == nil || !msg.ProtoReflect().IsValid() {
		return
	}
	m := msg.ProtoReflect()
	fields := m.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if implemented[fd.JSONName()] || !m.Has(fd) {
			continue
		}
		if fd.Kind() == protoreflect.MessageKind && !fd.IsList() && !fd.IsMap() && proto.Size(m.Get(fd).Message().Interface()) == 0 {
			continue
		}
		s.Warnf(Ptr(root, fd.JSONName()), "agent.unsupported-field", "%s.%s is not applied by this agent build", root, fd.JSONName())
	}
}
