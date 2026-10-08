package agent

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
)

func TestUnnumberedDomainApplyRetrieveRevoke(t *testing.T) {
	v := coretest.New()
	dir := t.TempDir()
	s := newSvc(t, v, dir)
	cfg := doc(t, `{"interfaces":{"loop901":{"ipv4":["192.0.2.1/32"]},"loop902":{"unnumbered":"loop901"}}}`)
	resp := apply(t, s, &ngfwv1.ApplyRequest{TxnId: "un-1", DesiredState: cfg})
	mustStatus(t, resp, ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	got := retrieveIfs(t, s)
	if got.GetInterfaces()["loop902"].GetUnnumbered() != "loop901" {
		t.Fatalf("live assembly lost donor: %v", got)
	}
	resp = apply(t, s, &ngfwv1.ApplyRequest{TxnId: "un-2", DesiredState: cfg})
	mustStatus(t, resp, ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if len(resp.GetResults()) != 0 {
		t.Fatalf("nonempty repeat plan: %v", resp)
	}
	s.Close()
	s = newSvc(t, v, dir)
	r := s.Resync(context.Background())
	mustStatus(t, r, ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if retrieveIfs(t, s).GetInterfaces()["loop902"].GetUnnumbered() != "loop901" {
		t.Fatal("restart lost borrowing")
	}
	cfg.Interfaces["loop903"] = &ngfwv1.Interface{Ipv4: []string{"198.51.100.1/32"}}
	cfg.Interfaces["loop902"].Unnumbered = proto.String("loop903")
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "un-update", DesiredState: cfg}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if retrieveIfs(t, s).GetInterfaces()["loop902"].GetUnnumbered() != "loop903" {
		t.Fatal("update did not replace donor")
	}
	cfg.Interfaces["loop902"].Unnumbered = proto.String("loop901")
	delete(cfg.Interfaces, "loop903")
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "un-rollback", DesiredState: cfg}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if retrieveIfs(t, s).GetInterfaces()["loop902"].GetUnnumbered() != "loop901" {
		t.Fatal("rollback did not restore donor")
	}
	delete(cfg.Interfaces, "loop902")
	resp = apply(t, s, &ngfwv1.ApplyRequest{TxnId: "un-3", DesiredState: cfg})
	mustStatus(t, resp, ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if retrieveIfs(t, s).GetInterfaces()["loop902"] != nil {
		t.Fatal("borrower remained after revoke")
	}
}
