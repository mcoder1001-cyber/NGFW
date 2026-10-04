package desired

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/scheduler"
	"strings"
	"testing"
)

type pppoeSink struct {
	kvs              []scheduler.KV
	errors, warnings []string
}

func (s *pppoeSink) Add(k scheduler.Key, v proto.Message, _ string) {
	s.kvs = append(s.kvs, scheduler.KV{Key: k, Value: v})
}
func (s *pppoeSink) Errorf(p, r, f string, a ...any) {
	s.errors = append(s.errors, p+":"+r+":"+fmt.Sprintf(f, a...))
}
func (s *pppoeSink) Warnf(p, r, f string, a ...any) {
	s.warnings = append(s.warnings, p+":"+r+":"+fmt.Sprintf(f, a...))
}
func TestPppoeProjectionRequiresLCPAndKeepsOnlyReferences(t *testing.T) {
	ifs := map[string]*ngfwv1.Interface{"wan/0": {Pppoe: &ngfwv1.Pppoe{Username: proto.String("user"), PasswordRef: proto.String("password/test")}}}
	missing := &pppoeSink{}
	Pppoe(missing, ifs, false)
	if len(missing.errors) != 1 || !strings.HasPrefix(missing.errors[0], "/interfaces/wan~10/pppoe/parent:") {
		t.Fatal(missing.errors)
	}
	ifs["wan/0"].Lcp = &ngfwv1.InterfaceLcp{HostIfName: proto.String("tap0")}
	success := &pppoeSink{}
	Pppoe(success, ifs, false)
	if len(success.errors) > 0 || len(success.kvs) != 1 || len(success.warnings) != 2 {
		t.Fatal(success)
	}
	doc := success.kvs[0].Value.(*ngfwv1.DesiredState)
	if doc.Interfaces["wan/0"].Pppoe.GetPasswordRef() != "password/test" || doc.Interfaces["wan/0"].Pppoe.GetParent() != "wan/0" {
		t.Fatal(doc)
	}
	ifs["wan/0"].Pppoe.Enabled = proto.Bool(false)
	disabled := &pppoeSink{}
	Pppoe(disabled, ifs, false)
	if len(disabled.kvs) > 0 {
		t.Fatal("disabled session dialled")
	}
}
