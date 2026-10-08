package chrony

import (
	"context"
	"encoding/hex"
	"google.golang.org/protobuf/proto"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/secretchannel"
	"ngfw/agent/internal/secretvalue"
	"os"
	"strings"
	"testing"
)

func TestSealedGenerationsRotationRollbackRestartAndRemoval(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cache, err := secretchannel.Open(root, "chrony-generation")
	if err != nil {
		t.Fatal(err)
	}
	activate := func(material string) {
		id, e := cache.Stage(map[string][]byte{"key/upstream": []byte(material), "key/backup": []byte(material + "_backup")})
		if e != nil {
			t.Fatal(e)
		}
		if e = cache.Activate(id); e != nil {
			t.Fatal(e)
		}
	}
	oldMaterial, newMaterial := "NGFW_TEST_PSK_NTP_OLD", "NGFW_TEST_PSK_NTP_NEW"
	activate(oldMaterial)
	p := tmpPaths(t)
	create := func() *Descriptor {
		return NewDescriptor(New(renderers.NewRecordingRunner().Succeed(ChronydBin, "").Succeed(ChronycBin, "200 OK"), WithPaths(p), WithSecretGenerations(func(ctx context.Context, ref string) ([]byte, error) {
			generation, ok := secretvalue.Binding(ctx, ref)
			if !ok {
				return nil, secretvalue.ErrInvalid
			}
			return cache.Resolve(ctx, generation)
		})), nil)
	}
	d := create()
	in := Input(clientNTP())
	bound := func() proto.Message {
		refs := map[string]string{}
		for _, ref := range secretvalue.References(in) {
			generation, e := cache.Ref(ctx, ref)
			if e != nil {
				t.Fatal(e)
			}
			refs[ref] = generation
		}
		v, e := secretvalue.Wrap(in, refs)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	oldValue := bound()
	if _, err = d.Create(ctx, oldValue); err != nil {
		t.Fatal(err)
	}
	activate(newMaterial)
	newValue := bound()
	if proto.Equal(oldValue, newValue) {
		t.Fatal("same-reference rotation unchanged")
	}
	if !proto.Equal(retrieveOne(t, d), oldValue) {
		t.Fatal("active candidate changed installed readback")
	}
	if _, err = d.Update(ctx, oldValue, newValue, nil); err != nil {
		t.Fatal(err)
	}
	assertKey := func(want, absent string) {
		raw, e := os.ReadFile(p.Keys())
		if e != nil {
			t.Fatal(e)
		}
		if !strings.Contains(string(raw), strings.ToUpper(hex.EncodeToString([]byte(want)))) || strings.Contains(string(raw), strings.ToUpper(hex.EncodeToString([]byte(absent)))) {
			t.Fatal("wrong key generation written")
		}
	}
	assertKey(newMaterial, oldMaterial)
	cache, err = secretchannel.Open(root, "chrony-generation")
	if err != nil {
		t.Fatal(err)
	}
	d = create()
	if !proto.Equal(retrieveOne(t, d), newValue) {
		t.Fatal("restart lost installed bindings")
	}
	if _, err = d.Update(ctx, newValue, oldValue, nil); err != nil {
		t.Fatal(err)
	}
	assertKey(oldMaterial, newMaterial)
	if _, err = d.Create(ctx, in); err == nil {
		t.Fatal("unbound secret configuration accepted")
	}
	assertKey(oldMaterial, newMaterial)
	empty, e := cache.Stage(map[string][]byte{})
	if e != nil {
		t.Fatal(e)
	}
	if e = cache.Activate(empty); e != nil {
		t.Fatal(e)
	}
	if _, e = cache.Ref(ctx, "key/upstream"); e == nil {
		t.Fatal("revoked selected key remains available")
	}
	if err = d.Delete(ctx, oldValue, nil); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p.Keys())
	if strings.Contains(string(raw), strings.ToUpper(hex.EncodeToString([]byte(oldMaterial)))) {
		t.Fatal("removed key retained in daemon file")
	}
}
