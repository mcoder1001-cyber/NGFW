package agent

import (
	"context"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/secretchannel"
	"testing"
)

func TestSnmpProjectionSelectsSealedOwner(t *testing.T) {
	const owner = "snmp-projection-bound"
	cache, err := secretchannel.Open(t.TempDir(), owner)
	if err != nil {
		t.Fatal(err)
	}
	other, err := secretchannel.Open(t.TempDir(), owner+"-other")
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]*secretchannel.Store{owner: cache, owner + "-other": other} {
		id, e := c.Stage(map[string][]byte{"password/snmp-community": []byte("NGFW_TEST_PSK_" + name)})
		if e != nil {
			t.Fatal(e)
		}
		if e = c.Activate(id); e != nil {
			t.Fatal(e)
		}
		desired.SetSnmpSecretGenerations(name, c.Ref)
		t.Cleanup(func() { desired.SetSnmpSecretGenerations(name, nil) })
	}
	desired.SetSnmpCheck(owner, func(_ *ngfwv1.SnmpService) error { return nil })
	desired.SetSnmpCheck(owner+"-other", func(_ *ngfwv1.SnmpService) error { t.Fatal("foreign owner's checker selected"); return nil })
	t.Cleanup(func() { desired.SetSnmpCheck(owner, nil); desired.SetSnmpCheck(owner+"-other", nil) })
	ds := doc(t, `{"services":{"snmp":{"enabled":true,"communities":{"ro":{"secretRef":"password/snmp-community"}}}}}`)
	p := projectOwned(ds, []string{"services"}, nil, nil, owner)
	if p.hasErrors() {
		t.Fatalf("bound owner rejected: %+v", p.issues)
	}
	found := false
	for _, kv := range p.kvs {
		if kv.Key != desired.SnmpKey {
			continue
		}
		found = true
		_, bindings, e := desired.ParseSnmpValue(kv.Value)
		want, e2 := cache.Ref(context.Background(), "password/snmp-community")
		foreign, e3 := other.Ref(context.Background(), "password/snmp-community")
		if e != nil || e2 != nil || e3 != nil || bindings["password/snmp-community"] != want || want == foreign {
			t.Fatal("incorrect owner binding", e, e2, e3)
		}
	}
	if !found {
		t.Fatal("SNMP object absent")
	}
	for _, name := range []string{"", owner + "-missing"} {
		refused := projectOwned(ds, []string{"services"}, nil, nil, name)
		if !refused.hasErrors() {
			t.Fatal("unbound owner accepted", name)
		}
		for _, kv := range refused.kvs {
			if kv.Key == desired.SnmpKey {
				t.Fatal("refused owner emitted SNMP")
			}
		}
	}
	// Even without parser checks, ownerless fingerprint lookup stays ambiguous.
	desired.SetSnmpCheck(owner, nil)
	desired.SetSnmpCheck(owner+"-other", nil)
	ambiguous := projectOwned(ds, []string{"services"}, nil, nil, "")
	if !ambiguous.hasErrors() {
		t.Fatal("ambiguous secret owners accepted")
	}
	for _, kv := range ambiguous.kvs {
		if kv.Key == desired.SnmpKey {
			t.Fatal("ambiguous secrets emitted SNMP")
		}
	}
	empty, err := cache.Stage(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = cache.Activate(empty); err != nil {
		t.Fatal(err)
	}
	missing := projectOwned(ds, []string{"services"}, nil, nil, owner)
	if !missing.hasErrors() {
		t.Fatal("missing selected generation fell back to another owner")
	}
	for _, kv := range missing.kvs {
		if kv.Key == desired.SnmpKey {
			t.Fatal("missing generation emitted SNMP")
		}
	}

}
