package desired

import (
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/scheduler"
)

type pppoeWANSink struct {
	Sink
	groups []*ngfwv1.WanGroup
}

// PppoeWANContext attaches route ownership references to the daemon's private
// document. The operator's PPP defaultRoute setting is kept unchanged for
// retrieval/rollback; only session rendering suppresses the automatic default.
func PppoeWANContext(s Sink, groups []*ngfwv1.WanGroup) Sink {
	return &pppoeWANSink{Sink: s, groups: groups}
}

func (s *pppoeWANSink) Add(key scheduler.Key, value proto.Message, pointer string) {
	if key != PppoeClientKey {
		s.Sink.Add(key, value, pointer)
		return
	}
	doc, ok := value.(*ngfwv1.DesiredState)
	if !ok {
		s.Sink.Add(key, value, pointer)
		return
	}
	detached := proto.Clone(doc).(*ngfwv1.DesiredState)
	var references []*ngfwv1.WanGroup
	for _, group := range s.groups {
		reference := &ngfwv1.WanGroup{Name: proto.String(group.GetName())}
		for _, member := range group.GetMembers() {
			if detached.GetInterfaces()[member.GetInterface()].GetPppoe() == nil {
				continue
			}
			reference.Members = append(reference.Members, &ngfwv1.WanMember{Interface: proto.String(member.GetInterface())})
		}
		if len(reference.Members) > 0 {
			references = append(references, reference)
		}
	}
	if len(references) > 0 {
		detached.Routing = &ngfwv1.RoutingConfig{WanGroups: references}
	}
	s.Sink.Add(key, detached, pointer)
}
