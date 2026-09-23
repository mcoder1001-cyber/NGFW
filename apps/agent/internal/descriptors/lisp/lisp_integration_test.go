package lisp_test

import (
	"os"
	"strconv"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/df6/df6test"
	"ngfw/agent/internal/descriptors/lisp"
	"ngfw/agent/internal/scheduler"
)

// EnvLISPHost opts in to the LISP host test: every LISP object needs the global LISP switch,
// which the test turns on only when it is off and restores in Cleanup (DF-6 prompt: "read /
// restore"). EnvLISPUpTo limits the run to the first N object types (stepwise bring-up on a
// shared VPP, manager rule after the gtpu incident).
const (
	EnvLISPHost = "VRX_DF6_LISP_HOST"
	EnvLISPUpTo = "VRX_DF6_LISP_UPTO"
)

func TestLISPOnHost(t *testing.T) {
	if os.Getenv(EnvLISPHost) != "1" {
		t.Skipf("LISP host test is opt-in (%s=1): it toggles the global LISP switch on the shared VPP", EnvLISPHost)
	}
	h := df6test.Connect(t)
	lispOn, gpeOn, err := lisp.Status(h.Ctx, h.Client)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("before: lisp=%v gpe=%v", lispOn, gpeOn)
	en := lisp.NewEnable(h.Client)
	if !lispOn {
		if _, err := en.Create(h.Ctx, &lisp.Enable{}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := en.Delete(h.Ctx, &lisp.Enable{}, nil); err != nil {
				t.Errorf("restore lisp disabled: %v", err)
			}
			if on, g, _ := lisp.Status(h.Ctx, h.Client); on || g != gpeOn {
				t.Errorf("after restore: lisp=%v gpe=%v, want false/%v", on, g, gpeOn)
			}
		})
	}
	loop, _ := h.Loopback(14, h.IP4(14, 1)+"/24")
	vrf := h.Table(13)
	h.IPTable(vrf, false)
	vni := h.Table(100)
	ls := h.Name("ls1")
	leid := h.IP4(14, 0) + "/24"
	reid := h.IP4(15, 0) + "/24"
	steps := []struct {
		d   scheduler.Descriptor
		obj proto.Message
	}{
		{lisp.NewLocatorSet(h.Client, h.Scope), &lisp.LocatorSet{Name: ls}},
		{lisp.NewLocator(h.Client, h.Scope), &lisp.Locator{LocatorSet: ls, Interface: loop, Priority: 1, Weight: 10}},
		{lisp.NewEidTableMap(h.Client, h.Scope), &lisp.EidTableMap{Vni: vni, DpTable: vrf}},
		{lisp.NewLocalEid(h.Client, h.Scope), &lisp.LocalEid{Vni: vni, Eid: leid, LocatorSet: ls}},
		{lisp.NewMapResolver(h.Client, h.Scope), &lisp.MapResolver{Address: h.IP4(14, 100)}},
		{lisp.NewMapServer(h.Client, h.Scope), &lisp.MapServer{Address: h.IP4(14, 101)}},
		{lisp.NewRemoteMapping(h.Client, h.Scope), &lisp.RemoteMapping{Vni: vni, Eid: reid, Rlocs: []*lisp.Rloc{{Address: h.IP4(14, 2), Priority: 1, Weight: 1}}}},
		{lisp.NewAdjacency(h.Client, h.Scope), &lisp.Adjacency{Vni: vni, Reid: reid, Leid: leid}},
		{lisp.NewGpeFwdEntry(h.Client, h.Scope), &lisp.GpeFwdEntry{Vni: vni, DpTable: vrf, Reid: h.IP4(17, 0) + "/24", Leid: leid, Pairs: []*lisp.LocatorPair{{Local: h.IP4(14, 1), Remote: h.IP4(14, 9), Weight: 1}}}},
	}
	if s := os.Getenv(EnvLISPUpTo); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > len(steps) {
			t.Fatalf("%s=%q", EnvLISPUpTo, s)
		}
		steps = steps[:n]
	}
	for _, s := range steps {
		if _, err := s.d.Create(h.Ctx, s.obj); err != nil {
			t.Fatalf("create %s: %v", s.d.KeyOf(s.obj), err)
		}
		t.Cleanup(func() { _ = s.d.Delete(h.Ctx, s.obj, nil) })
	}
	h.Hold()
	for _, s := range steps {
		if !observed(t, s.d, s.obj) {
			t.Errorf("%s not observed", s.d.KeyOf(s.obj))
		}
		t.Logf("observed %s: %v", s.d.KeyOf(s.obj), s.obj)
		if _, err := s.d.Retrieve(h.Ctx); err == nil {
			h.AssertEmptyPlan(s.d, s.obj)
		}
	}
	for i := len(steps) - 1; i >= 0; i-- {
		if err := steps[i].d.Delete(h.Ctx, steps[i].obj, nil); err != nil {
			t.Fatalf("delete %s: %v", steps[i].d.KeyOf(steps[i].obj), err)
		}
		if observedAny(t, steps[i].d, steps[i].obj) {
			t.Fatalf("after delete %s still present", steps[i].d.KeyOf(steps[i].obj))
		}
	}
}
