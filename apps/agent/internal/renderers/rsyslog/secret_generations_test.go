package rsyslog

import (
	"context"
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/rfkit"
	"ngfw/agent/internal/secretchannel"
	"ngfw/agent/internal/secretvalue"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTLSSealedGenerationsRotationRollbackRestartAndRemoval(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cache, err := secretchannel.Open(root, "syslog-generation")
	if err != nil {
		t.Fatal(err)
	}
	oldKey, newKey := keyPEM, strings.ReplaceAll(keyPEM, "RF4_tlskey", "RF4_rotated_tlskey")
	activate := func(key string) {
		id, e := cache.Stage(map[string][]byte{"cert/syslog-ca": []byte(certPEM), "cert/syslog-client": []byte(certPEM), "key/syslog-client": []byte(key)})
		if e != nil {
			t.Fatal(e)
		}
		if e = cache.Activate(id); e != nil {
			t.Fatal(e)
		}
	}
	activate(oldKey)
	p, dc := slotPaths(t)
	p.ModuleDir = t.TempDir()
	if err = os.WriteFile(filepath.Join(p.ModuleDir, "lmnsd_ossl.so"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	create := func() *Descriptor {
		return NewDescriptor(New(renderers.NewRecordingRunner().Succeed(RsyslogdBin, ""), WithPaths(p), WithController(dc), WithSecretResolver(rfkit.SecretResolverFunc(func(ctx context.Context, ref string) (string, error) {
			generation, ok := secretvalue.Binding(ctx, ref)
			if !ok {
				return "", secretvalue.ErrInvalid
			}
			raw, e := cache.Resolve(ctx, generation)
			defer clear(raw)
			return string(raw), e
		}))), nil)
	}
	d := create()
	in := typed(&ngfwv1.SyslogTarget{Address: proto.String("logs.example.net"), Port: proto.Uint32(6514), Protocol: proto.String("tls"), Tls: &ngfwv1.SyslogTls{CaRef: proto.String("cert/syslog-ca"), CertRef: proto.String("cert/syslog-client"), KeyRef: proto.String("key/syslog-client"), AuthMode: proto.String("x509/name")}})
	bound := func() proto.Message {
		refs := map[string]string{}
		for _, ref := range secretvalue.References(in) {
			generation, e := cache.Ref(ctx, ref)
			if e != nil {
				t.Fatal(e)
			}
			refs[ref] = generation
		}
		value, e := secretvalue.Wrap(in, refs)
		if e != nil {
			t.Fatal(e)
		}
		return value
	}
	oldValue := bound()
	if _, err = d.Create(ctx, oldValue); err != nil {
		t.Fatal(err)
	}
	activate(newKey)
	newValue := bound()
	if proto.Equal(oldValue, newValue) {
		t.Fatal("rotation missing from scheduler value")
	}
	if !proto.Equal(retrieveOne(t, d), oldValue) {
		t.Fatal("candidate overwrote installed generation")
	}
	if _, err = d.Update(ctx, oldValue, newValue, nil); err != nil {
		t.Fatal(err)
	}
	assertKey := func(want, absent string) {
		entries, e := os.ReadDir(p.TLSDir)
		if e != nil {
			t.Fatal(e)
		}
		found := false
		for _, entry := range entries {
			raw, e := os.ReadFile(filepath.Join(p.TLSDir, entry.Name()))
			if e != nil {
				t.Fatal(e)
			}
			if string(raw) == want {
				found = true
			}
			if string(raw) == absent {
				t.Fatal("obsolete credential retained")
			}
		}
		if !found {
			t.Fatal("selected key not installed")
		}
	}
	assertKey(newKey, oldKey)
	cache, err = secretchannel.Open(root, "syslog-generation")
	if err != nil {
		t.Fatal(err)
	}
	d = create()
	if !proto.Equal(retrieveOne(t, d), newValue) {
		t.Fatal("restart lost generation metadata")
	}
	if _, err = d.Update(ctx, newValue, oldValue, nil); err != nil {
		t.Fatal(err)
	}
	assertKey(oldKey, newKey)
	if _, err = d.Create(ctx, in); err == nil {
		t.Fatal("unbound TLS accepted")
	}
	assertKey(oldKey, newKey)
	empty, e := cache.Stage(map[string][]byte{})
	if e != nil {
		t.Fatal(e)
	}
	if e = cache.Activate(empty); e != nil {
		t.Fatal(e)
	}
	if _, e = cache.Ref(ctx, "key/syslog-client"); e == nil {
		t.Fatal("revoked selected key available")
	}
	if err = d.Delete(ctx, oldValue, nil); err != nil {
		t.Fatal(err)
	}
	entries, e := os.ReadDir(p.TLSDir)
	if e != nil || len(entries) != 0 {
		t.Fatal("removed TLS material retained", e)
	}
}
