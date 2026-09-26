package desired

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// unsupported is the generic agent.unsupported-field walk used for `management` (ManagementUnsupported in
// syslog.go). The `services` sub-keys use ServicesImplemented/ServicesUnsupported in qos_services.go (the first
// `services` feature to merge kept them; F-qos-flat-questions.md Q4). It warns for every set, non-empty field of
// msg whose JSON name is not in implemented.
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
