package ikev2_test

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"
	ikev2api "ngfw/agent/binapi/ikev2"
	"ngfw/agent/binapi/ikev2_types"
	ikev2d "ngfw/agent/internal/descriptors/ikev2"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/pki"
	"ngfw/agent/internal/scheduler"
)

func nativeSnapshotCfg(t *testing.T, v *fakeVPP) (ikev2d.Config, string, string, string) {
	t.Helper()
	mat1, mat2, peer := make([]byte, 32), make([]byte, 32), make([]byte, 32)
	for _, m := range [][]byte{mat1, mat2, peer} {
		if _, e := rand.Read(m); e != nil {
			t.Fatal(e)
		}
	}
	cfg := newCfg(v)
	cfg.GlobalsOwner = true
	cfg.NativeRoot = filepath.Join(t.TempDir(), "native")
	cfg.Secrets = vpn.NewMapResolver(keys, mat1, mat2, peer)
	key1, cert1, e := ikev2d.NativePaths(cfg.NativeRoot, keys.Ref(mat1), keys.Ref(peer))
	if e != nil {
		t.Fatal(e)
	}
	key2, _, e := ikev2d.NativePaths(cfg.NativeRoot, keys.Ref(mat2), keys.Ref(peer))
	if e != nil {
		t.Fatal(e)
	}
	for _, m := range [][]byte{mat1, mat2, peer} {
		clear(m)
	}
	return cfg, key1, key2, cert1
}
func TestNativeKeyImmutableSnapshotsCompensation(t *testing.T) {
	for _, restoreFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "restored", true: "uncertain"}[restoreFails], func(t *testing.T) {
			v := newFakeVPP()
			cfg, key1, key2, _ := nativeSnapshotCfg(t, v)
			d := ikev2d.NewLocalKey(cfg)
			old := &vpnpb.Ikev2LocalKey{KeyFile: key1}
			if _, e := d.Create(ctx, old); e != nil {
				t.Fatal(e)
			}
			if st, e := os.Stat(key1); e != nil || st.Mode().Perm() != 0600 {
				t.Fatal("private snapshot permissions", e)
			}
			calls := 0
			v.On("ikev2_set_local_key", func(m api.Message) ([]api.Message, error) {
				calls++
				r := m.(*ikev2api.Ikev2SetLocalKey)
				v.localKey = "" // native setter frees old key before loading
				if r.KeyFile == key2 || restoreFails {
					return []api.Message{&ikev2api.Ikev2SetLocalKeyReply{Retval: rvInvalid}}, nil
				}
				v.localKey = r.KeyFile
				return []api.Message{&ikev2api.Ikev2SetLocalKeyReply{}}, nil
			})
			_, e := d.Update(ctx, old, &vpnpb.Ikev2LocalKey{KeyFile: key2}, nil)
			if e == nil || errors.Is(e, scheduler.ErrUncertainOutcome) != restoreFails || calls != 2 {
				t.Fatal("failed load compensation outcome", e, calls)
			}
			if !restoreFails && v.localKey != key1 {
				t.Fatal("old key not restored")
			}
			if _, e := os.Stat(key1); e != nil {
				t.Fatal("rollback generation removed", e)
			}
		})
	}
}
func TestNativeKeyFirstLoadFailureUncertain(t *testing.T) {
	v := newFakeVPP()
	cfg, key1, _, _ := nativeSnapshotCfg(t, v)
	d := ikev2d.NewLocalKey(cfg)
	v.On("ikev2_set_local_key", func(api.Message) ([]api.Message, error) {
		return []api.Message{&ikev2api.Ikev2SetLocalKeyReply{Retval: rvInvalid}}, nil
	})
	if _, e := d.Create(ctx, &vpnpb.Ikev2LocalKey{KeyFile: key1}); !errors.Is(e, scheduler.ErrUncertainOutcome) {
		t.Fatal("first load falsely claims known rollback", e)
	}
}
func TestNativeKeyForeignProfileBeforeWrites(t *testing.T) {
	v := newFakeVPP()
	cfg, key1, _, _ := nativeSnapshotCfg(t, v)
	v.order = append(v.order, "foreign-rsa")
	v.profiles["foreign-rsa"] = &fakeProfile{p: ikev2_types.Ikev2Profile{Name: "foreign-rsa", Auth: ikev2_types.Ikev2Auth{Method: 1}}}
	if _, e := ikev2d.NewLocalKey(cfg).Create(ctx, &vpnpb.Ikev2LocalKey{KeyFile: key1}); e == nil {
		t.Fatal("foreign key owner ignored")
	}
	if _, e := os.Stat(cfg.NativeRoot); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("foreign conflict wrote private snapshots", e)
	}
	if v.localKey != "" {
		t.Fatal("foreign key overwritten")
	}
}
func TestNativeKeySnapshotsRejectSymlinkAndTampering(t *testing.T) {
	for _, tamper := range []bool{false, true} {
		t.Run(map[bool]string{false: "symlink", true: "content"}[tamper], func(t *testing.T) {
			v := newFakeVPP()
			cfg, key1, _, _ := nativeSnapshotCfg(t, v)
			d := ikev2d.NewLocalKey(cfg)
			if tamper {
				if _, e := d.Create(ctx, &vpnpb.Ikev2LocalKey{KeyFile: key1}); e != nil {
					t.Fatal(e)
				}
				if e := os.WriteFile(key1, []byte("invalid"), 0600); e != nil {
					t.Fatal(e)
				}
			} else {
				if e := os.Symlink(t.TempDir(), cfg.NativeRoot); e != nil {
					t.Fatal(e)
				}
			}
			if _, e := d.Create(ctx, &vpnpb.Ikev2LocalKey{KeyFile: key1}); e == nil {
				t.Fatal("unsafe immutable generation accepted")
			}
		})
	}
}
func TestNativeCertificateDependenciesAndRecreation(t *testing.T) {
	v := newFakeVPP()
	cfg, key1, _, cert := nativeSnapshotCfg(t, v)
	kd := ikev2d.NewLocalKey(cfg)
	pd := ikev2d.NewProfile(cfg)
	dep := kd.Dependencies(&vpnpb.Ikev2LocalKey{KeyFile: key1})
	if len(dep) != 1 || dep[0].Key != pki.Key || dep[0].Optional {
		t.Fatal(dep)
	}
	p := &vpnpb.Ikev2Profile{Name: "site", Auth: &vpnpb.Ikev2Auth{Method: ikev2d.AuthRSASig, CertFile: cert}}
	dep = pd.Dependencies(p)
	if len(dep) != 2 || dep[0].Key != ikev2d.LocalKeyKey || dep[0].Optional || dep[1].Key != pki.Key || dep[1].Optional {
		t.Fatal(dep)
	}
	changed := proto.Clone(p).(*vpnpb.Ikev2Profile)
	changed.LocalId = &vpnpb.Ikev2Id{Type: "fqdn", Value: "new.test"}
	if _, e := pd.Update(ctx, p, changed, nil); !errors.Is(e, scheduler.ErrRecreate) {
		t.Fatal("RSA identity update was not atomic recreate", e)
	}
}

func TestNativeKeyRotationSessionRetirementUncertain(t *testing.T) {
	v := newFakeVPP()
	cfg, key1, _, _ := nativeSnapshotCfg(t, v)
	v.order = append(v.order, "w4-site")
	v.profiles["w4-site"] = &fakeProfile{p: ikev2_types.Ikev2Profile{Name: "w4-site", Auth: ikev2_types.Ikev2Auth{Method: 1, Data: []byte("old-generation.cert.pem")}}}
	v.sas = []ikev2_types.Ikev2SaV3{{ProfileName: "w4-site", Ispi: 1}}
	v.On("ikev2_initiate_del_ike_sa", func(api.Message) ([]api.Message, error) {
		return []api.Message{&ikev2api.Ikev2InitiateDelIkeSaReply{Retval: rvInvalid}}, nil
	})
	_, e := ikev2d.NewLocalKey(cfg).Create(ctx, &vpnpb.Ikev2LocalKey{KeyFile: key1})
	if !errors.Is(e, scheduler.ErrUncertainOutcome) {
		t.Fatal("failed retirement must not claim known rollback", e)
	}
	if v.localKey != "" {
		t.Fatal("key changed after unsuccessful retirement")
	}
}
