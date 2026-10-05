package ikev2_test

import (
	"errors"
	"strings"
	"testing"

	"go.fd.io/govpp/api"
	ikev2api "ngfw/agent/binapi/ikev2"
	"ngfw/agent/binapi/ikev2_types"
	ikev2d "ngfw/agent/internal/descriptors/ikev2"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
)

func TestNativeKeyReplayOptInBoundaries(t *testing.T) {
	v := newFakeVPP()
	cfg, _, _, _ := nativeSnapshotCfg(t, v)
	for _, native := range []bool{false, true} {
		for _, owner := range []bool{false, true} {
			config := cfg
			config.GlobalsOwner = owner
			if !native {
				config.NativeRoot = ""
			}
			d := ikev2d.NewLocalKey(config)
			if d.CreatePreservesDependents() != (native && owner) {
				t.Fatal("invalid native/global scope", native, owner)
			}
			wrapped := vpn.Global(owner, d, nil)
			opted, ok := wrapped.(scheduler.WriteOnlyInPlaceCreator)
			if owner {
				if !ok || opted.CreatePreservesDependents() != native {
					t.Fatal("owner forwarding incorrect")
				}
			} else if ok {
				t.Fatal("non-owner can opt in")
			}
		}
	}
}

func TestNativeKeyFreshReplayLoadsAndRetiresOnlyChangedGeneration(t *testing.T) {
	v := newFakeVPP()
	cfg, key1, key2, cert := nativeSnapshotCfg(t, v)
	var operations []string
	v.On("ikev2_set_local_key", func(request api.Message) ([]api.Message, error) {
		operations = append(operations, "set")
		v.localKey = request.(*ikev2api.Ikev2SetLocalKey).KeyFile
		return []api.Message{&ikev2api.Ikev2SetLocalKeyReply{}}, nil
	})
	v.On("ikev2_initiate_del_ike_sa", func(api.Message) ([]api.Message, error) {
		operations = append(operations, "retire")
		v.sas = nil
		return []api.Message{&ikev2api.Ikev2InitiateDelIkeSaReply{}}, nil
	})
	if _, err := ikev2d.NewLocalKey(cfg).Create(ctx, &vpnpb.Ikev2LocalKey{KeyFile: key1}); err != nil {
		t.Fatal(err)
	}
	v.order = append(v.order, "w4-site")
	v.profiles["w4-site"] = &fakeProfile{p: ikev2_types.Ikev2Profile{Name: "w4-site", Auth: ikev2_types.Ikev2Auth{Method: 1, Data: []byte(cert)}}}
	v.sas = []ikev2_types.Ikev2SaV3{{ProfileName: "w4-site", Ispi: 77}}
	operations = nil
	// Independent fresh descriptors never infer native key presence or skip its setter.
	for range 2 {
		if _, err := ikev2d.NewLocalKey(cfg).Create(ctx, &vpnpb.Ikev2LocalKey{KeyFile: key1}); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(operations, ",") != "set,set" || len(v.sas) != 1 || v.sas[0].Ispi != 77 {
		t.Fatal("same-generation replay lost SA or skipped real setter", operations)
	}
	operations = nil
	if _, err := ikev2d.NewLocalKey(cfg).Create(ctx, &vpnpb.Ikev2LocalKey{KeyFile: key2}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(operations, ",") != "retire,set" || len(v.sas) != 0 || v.localKey != key2 {
		t.Fatal("rotation did not retire before setter", operations)
	}
}

func TestNativeFreshReplayFailureAndForeignGuard(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		t.Run(map[bool]string{false: "setter-uncertain", true: "foreign"}[foreign], func(t *testing.T) {
			v := newFakeVPP()
			cfg, key1, _, _ := nativeSnapshotCfg(t, v)
			sets := 0
			v.On("ikev2_set_local_key", func(api.Message) ([]api.Message, error) {
				sets++
				return []api.Message{&ikev2api.Ikev2SetLocalKeyReply{Retval: rvInvalid}}, nil
			})
			if foreign {
				v.order = append(v.order, "other-site")
				v.profiles["other-site"] = &fakeProfile{p: ikev2_types.Ikev2Profile{Name: "other-site", Auth: ikev2_types.Ikev2Auth{Method: 1}}}
			}
			_, err := ikev2d.NewLocalKey(cfg).Create(ctx, &vpnpb.Ikev2LocalKey{KeyFile: key1})
			if err == nil {
				t.Fatal("unsafe replay accepted")
			}
			if foreign {
				if sets != 0 {
					t.Fatal("foreign key overwritten")
				}
			} else if sets != 1 || !errors.Is(err, scheduler.ErrUncertainOutcome) {
				t.Fatal("first replay setter failure hidden", sets, err)
			}
		})
	}
}
