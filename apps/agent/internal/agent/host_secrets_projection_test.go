package agent

import (
	"context"
	"google.golang.org/protobuf/proto"
	"ngfw/agent/internal/secretchannel"
	"ngfw/agent/internal/subsystems"
	"testing"
)

func TestHostCredentialProjectionSelectsOwnerAndRotation(t *testing.T) {
	const owner = "host-credential-projection"
	cache, err := secretchannel.Open(t.TempDir(), owner)
	if err != nil {
		t.Fatal(err)
	}
	if err = subsystems.SetHostServiceSecrets(owner, cache.Ref, cache.Resolve); err != nil {
		t.Fatal(err)
	}
	ds := doc(t, `{"services":{"ntp":{"enabled":true,"servers":[{"address":"192.0.2.1","keyRef":"key/ntp"}]},"hostStack":{"namespaces":{"app":{"secretRef":"key/app"}}}},"management":{"syslog":[{"protocol":"tls","tls":{"caRef":"cert/ca"}}]}}`)
	projectGeneration := func(value string) *projected {
		t.Helper()
		id, e := cache.Stage(map[string][]byte{"key/ntp": []byte(value), "key/app": []byte(value), "cert/ca": []byte(value)})
		if e != nil {
			t.Fatal(e)
		}
		if e = cache.Activate(id); e != nil {
			t.Fatal(e)
		}
		p := projectOwned(ds, []string{"services", "management"}, nil, nil, owner)
		if p.hasErrors() {
			t.Fatalf("projection rejected selected credentials: %+v", p.issues)
		}
		return p
	}
	old := projectGeneration("NGFW_TEST_OLD")
	newer := projectGeneration("NGFW_TEST_NEW")
	if len(old.kvs) != 3 || len(newer.kvs) != 3 {
		t.Fatalf("expected three selected consumers, got %d/%d", len(old.kvs), len(newer.kvs))
	}
	for i, kv := range old.kvs {
		if proto.Equal(kv.Value, newer.kvs[i].Value) {
			t.Fatalf("rotation did not change %s", kv.Key)
		}
	}
	unavailable := projectOwned(ds, []string{"services", "management"}, nil, nil, owner+"-other")
	if !unavailable.hasErrors() {
		t.Fatal("projection used another owner's credential store")
	}
	empty, e := cache.Stage(nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = cache.Activate(empty); e != nil {
		t.Fatal(e)
	}
	unavailable = projectOwned(ds, []string{"services", "management"}, nil, nil, owner)
	if !unavailable.hasErrors() {
		t.Fatal("removed references fell back to historical selection")
	}
	if _, err = cache.Ref(context.Background(), "key/ntp"); err == nil {
		t.Fatal("removed key still selected")
	}
}
