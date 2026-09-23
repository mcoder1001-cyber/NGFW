package pnat_test

import (
	"testing"

	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/descriptors/pnat"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestPnatOnHost: one integration check per pnat object type on the host VPP. Bindings use
// slot addresses (10.<N>.51–53.x) and are attached only to this slot's loopbacks. Attachments
// are removed before their bindings (a binding deleted while attached leaves a dangling flow
// entry in VPP 26.06). Nothing global is touched; the pnat flow hash stays initialised after
// the first attach (VPP never frees it on detach), which is harmless.
func TestPnatOnHost(t *testing.T) {
	c := nattest.Connect(t)
	ctx := nattest.Ctx(t)
	p := pnat.New(c, vpptest.Prefix(t))

	b1 := natcommon.MustEncode(&pnat.BindingSpec{Match: pnat.MatchSpec{Src: nattest.Addr4(t, 51, 1), Dst: nattest.Addr4(t, 52, 1), Proto: "udp", DstPort: 53},
		Rewrite: pnat.RewriteSpec{Dst: nattest.Addr4(t, 53, 1), DstPort: 5353}})
	b2 := natcommon.MustEncode(&pnat.BindingSpec{Match: pnat.MatchSpec{Dst: nattest.Addr4(t, 52, 2), Proto: "tcp", DstPort: 80},
		Rewrite: pnat.RewriteSpec{Src: nattest.Addr4(t, 53, 2), ClearByte: true, ClearOffset: 3}})
	nattest.CreateAll(ctx, t, p.Binding, b1, b2)
	nattest.AssertPlan(t, p.Binding, b1, b2)

	in, _ := nattest.Loopback(t, c, 50)
	out, _ := nattest.Loopback(t, c, 51)
	id1 := pnat.MatchSpec{Src: nattest.Addr4(t, 51, 1), Dst: nattest.Addr4(t, 52, 1), Proto: "udp", DstPort: 53}.ID()
	id2 := pnat.MatchSpec{Dst: nattest.Addr4(t, 52, 2), Proto: "tcp", DstPort: 80}.ID()
	a1 := natcommon.MustEncode(&pnat.AttachmentSpec{Interface: in, Point: pnat.PointInput, Binding: id1})
	a2 := natcommon.MustEncode(&pnat.AttachmentSpec{Interface: out, Point: pnat.PointOutput, Binding: id2})
	nattest.CreateAll(ctx, t, p.Attachment, a1, a2)
	nattest.AssertPlan(t, p.Attachment, a1, a2)

	nattest.DeleteAll(ctx, t, p.Attachment)
	nattest.DeleteAll(ctx, t, p.Binding)
}
