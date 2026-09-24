package ipsec_test

import (
	"errors"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/ipsec"
	ipsecd "ngfw/agent/internal/descriptors/ipsec"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
)

// TestWriteAheadRecords (review M4): a failed add leaves no record when the object does not
// exist; an add that went through in VPP but whose reply was lost leaves the object recorded as
// ours, so Retrieve reports it and a retried Create adopts it instead of being stuck.
func TestWriteAheadRecords(t *testing.T) {
	v := newFakeVPP()
	cfg := newCfg(v)
	v.Fail("ipsec_spd_add_del", errors.New("vpp: boom"))
	if _, err := ipsecd.NewSpd(cfg).Create(ctx, &vpnpb.IpsecSpd{SpdId: 4007}); err == nil {
		t.Fatal("VPP error must surface")
	}
	if _, ok := cfg.Boot.Get("ipsec.spd/4007"); ok {
		t.Fatal("a failed add must not leave a record")
	}

	// the SA add succeeds in VPP, the reply is lost
	orig := v.sas
	v.On("ipsec_sad_entry_add_v2", func(req api.Message) ([]api.Message, error) {
		e := req.(*ipsec.IpsecSadEntryAddV2).Entry
		e.CryptoKey.Data = append([]byte(nil), e.CryptoKey.Data...)
		e.Salt = e.Salt<<24 | (e.Salt>>8&0xff)<<16 | (e.Salt>>16&0xff)<<8 | e.Salt>>24
		orig[e.SadID] = e
		v.saLocks[e.SadID] = 1
		return nil, errors.New("reply lost")
	})
	d := ipsecd.NewSa(cfg)
	if _, err := d.Create(ctx, transportSA()); err == nil {
		t.Fatal("lost reply must surface")
	}
	kvs, err := d.Retrieve(ctx)
	if err != nil || len(kvs) != 1 {
		t.Fatalf("the SA added before the lost reply must be ours: %v %v", kvs, err)
	}
	if _, err := d.Create(ctx, transportSA()); err != nil {
		t.Fatalf("retry must adopt our own SA: %v", err)
	}
	if err := d.Delete(ctx, transportSA(), nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := v.sas[4001]; ok {
		t.Fatal("not cleaned")
	}
}
