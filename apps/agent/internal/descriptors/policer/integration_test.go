package policer

import (
	"errors"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/classify"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/policer"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Integration test against the VPP on this host (VRX_INTEGRATION=1, shared lab lock): policers
// are named "<VRX_TEST_PREFIX>:…", attachments sit on this slot's loopbacks (loop<slot>10…),
// everything is removed in t.Cleanup. Retrieve assertions only see this owner's policers.
func TestPolicerOnHost(t *testing.T) {
	h := df7test.StartHost(t)
	pd := NewPolicer(h.C, h.Owner)
	h.CleanupOwned(pd)

	ifA, idxA := h.Loopback(10, true, true)

	gold := Policer{Name: "gold", CIR: 1000, EIR: 2000, CB: 16000, EB: 32000, RateType: RateKbps, RoundType: RoundClosest,
		Type: Type2R3C2698, Conform: Action{Type: ActTransmit}, Exceed: Action{Type: ActMark, DSCP: 10}, Violate: Action{Type: ActDrop}}
	bronze := Policer{Name: "bronze", CIR: 500, CB: 8000, RateType: RatePps, RoundType: RoundUp, Type: Type1R2C, ColorAware: true,
		Conform: Action{Type: ActMark, DSCP: 46}, Exceed: Action{Type: ActDrop}, Violate: Action{Type: ActDrop}}
	desired := []scheduler.KV{df7test.Desired(pd, df7.Encode(gold)), df7test.Desired(pd, df7.Encode(bronze))}

	created := h.Apply(pd, desired...)
	t.Cleanup(func() { h.DeleteAll(pd, created) })
	actual := h.ExpectRetrieved(pd, desired...)
	for _, kv := range actual {
		t.Logf("retrieved %s meta %+v", kv.Key, kv.Meta)
	}

	t.Run("update in place", func(t *testing.T) {
		gold2 := gold
		gold2.CIR, gold2.EIR = 1500, 3000
		m, err := pd.Update(h.Ctx, df7.Encode(gold), df7.Encode(gold2), created[0].Meta)
		h.Must("update", err)
		if m != created[0].Meta {
			t.Fatalf("update changed meta %v → %v", created[0].Meta, m)
		}
		desired[0].Value = df7.Encode(gold2)
		h.ExpectRetrieved(pd, desired...)
	})

	t.Run("interface attach (write-only)", func(t *testing.T) {
		ad := NewInterface(h.C, h.Owner)
		in := df7.Encode(Attachment{Interface: ifA, Direction: DirInput, Policer: "gold"})
		out := df7.Encode(Attachment{Interface: ifA, Direction: DirOutput, Policer: "bronze"})
		att := h.Apply(ad, df7test.Desired(ad, in), df7test.Desired(ad, out))
		// idempotent re-apply (write-only resync)
		h.Apply(ad, df7test.Desired(ad, in))
		if _, err := ad.Retrieve(h.Ctx); !errors.Is(err, df7.ErrRetrieveUnsupported) {
			t.Fatalf("Retrieve: %v, want ErrRetrieveUnsupported", err)
		}
		h.Hold("policers attached to " + ifA)
		h.DeleteAll(ad, att)

		// VPP 26.06 quirk recorded in policer.md: the policer_input_v2 handler replies with the
		// v1 reply id. Probe it once and log what govpp makes of it; then undo with v1.
		idx := created[0].Meta.(Meta).Index
		_, err := policer.NewServiceClient(h.C).PolicerInputV2(h.Ctx, &policer.PolicerInputV2{PolicerIndex: idx, SwIfIndex: interface_types.InterfaceIndex(idxA), Apply: true})
		t.Logf("policer_input_v2 probe: err=%v", err)
		_, err = policer.NewServiceClient(h.C).PolicerInput(h.Ctx, &policer.PolicerInput{Name: h.Owner + ":gold", SwIfIndex: interface_types.InterfaceIndex(idxA), Apply: false})
		h.Must("undo v2 probe", err)
	})

	t.Run("bind to worker", func(t *testing.T) {
		bd := NewBind(h.C, h.Owner)
		_, err := bd.Create(h.Ctx, df7.Encode(Bind{Policer: "gold", Worker: 0}))
		var apiErr api.VPPApiError
		if errors.As(err, &apiErr) && apiErr == api.INVALID_WORKER {
			t.Skipf("skip: no workers on host (policer_bind → %v)", err)
		}
		h.Must("bind", err)
		h.Must("unbind", bd.Delete(h.Ctx, df7.Encode(Bind{Policer: "gold"}), nil))
	})

	t.Run("classify (write-only)", func(t *testing.T) {
		svc := classify.NewServiceClient(h.C)
		rep, err := svc.ClassifyAddDelTable(h.Ctx, &classify.ClassifyAddDelTable{IsAdd: true, TableIndex: df7.NoIndex, Nbuckets: 2,
			MemorySize: 2 << 20, MatchNVectors: 1, NextTableIndex: df7.NoIndex, MissNextIndex: df7.NoIndex, MaskLen: 16, Mask: make([]byte, 16)})
		h.Must("classify_add_del_table", err)
		t.Cleanup(func() {
			_, err := svc.ClassifyAddDelTable(h.Ctx, &classify.ClassifyAddDelTable{IsAdd: false, TableIndex: rep.NewTableIndex, DelChain: true})
			if err != nil {
				t.Errorf("cleanup classify table %d: %v", rep.NewTableIndex, err)
			}
		})
		tables := map[string]uint32{"w-cls": rep.NewTableIndex}
		cd := NewClassify(h.C, h.Owner, df7.WithClassifyTables(func(n string) (uint32, bool) { v, ok := tables[n]; return v, ok }))
		v := df7.Encode(Classify{Interface: ifA, IP4Table: "w-cls"})
		kv := h.Apply(cd, df7test.Desired(cd, v))
		h.Apply(cd, df7test.Desired(cd, v)) // idempotent
		// policer_classify_dump(~0) answers nothing in 26.06 — shown here, never used by Retrieve
		stream, err := svc.PolicerClassifyDump(h.Ctx, &classify.PolicerClassifyDump{Type: classify.POLICER_CLASSIFY_API_TABLE_IP4, SwIfIndex: interface_types.InterfaceIndex(df7.NoIndex)})
		h.Must("policer_classify_dump", err)
		dets, err := df7.Collect(stream.Recv)
		t.Logf("policer_classify_dump(~0) with a table bound on %s: %d details, err %v", ifA, len(dets), err)
		h.DeleteAll(cd, kv)
	})

	t.Run("restart simulation", func(t *testing.T) {
		h.RestartSimulation(func(c vpp.Client) scheduler.Descriptor { return NewPolicer(c, h.Owner) }, desired...)
	})

	h.DeleteAll(pd, created)
	created = nil
	h.ExpectNone(pd)
}
