package subsystems

import (
	"bytes"
	"context"
	"google.golang.org/protobuf/proto"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/secretchannel"
	"os"
	"testing"
)

func TestSnmpSealedRotationRollbackRestart(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cache, err := secretchannel.Open(dir, "snmp-sealed")
	if err != nil {
		t.Fatal(err)
	}
	st, paths, logs := newTestStage(t, nil)
	owner := "snmp-sealed"
	st.owner = owner
	snmpStages.Store(owner, st)
	t.Cleanup(st.Close)
	if err = SetSnmpSecrets(owner, cache.Ref, cache.Resolve); err != nil {
		t.Fatal(err)
	}
	cfg := snmpValue()
	cfg.V3Users = nil
	makeValue := func(secret string) proto.Message {
		t.Helper()
		id, e := cache.Stage(map[string][]byte{"password/snmp-ro": []byte(secret)})
		if e != nil {
			t.Fatal(e)
		}
		if e = cache.Activate(id); e != nil {
			t.Fatal(e)
		}
		ref, e := cache.Ref(ctx, "password/snmp-ro")
		if e != nil {
			t.Fatal(e)
		}
		v, e := desired.SnmpBoundValue(cfg, map[string]string{"password/snmp-ro": ref})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	old := makeValue("NGFW_TEST_SNMP_OLD")
	if _, err = st.Create(ctx, old); err != nil {
		t.Fatal(err)
	}
	newer := makeValue("NGFW_TEST_SNMP_NEW")
	if proto.Equal(old, newer) {
		t.Fatal("rotation invisible to scheduler")
	}
	if _, err = st.Update(ctx, old, newer, nil); err != nil {
		t.Fatal(err)
	}
	cache, err = secretchannel.Open(dir, "snmp-sealed")
	if err != nil {
		t.Fatal(err)
	}
	if err = SetSnmpSecrets(owner, cache.Ref, cache.Resolve); err != nil {
		t.Fatal(err)
	}
	if _, err = st.Create(ctx, old); err != nil {
		t.Fatal("rollback after restart", err)
	}
	//nolint:gosec // Test-owned renderer path, checking exact output and secret-free metadata.
	raw, err := os.ReadFile(paths.ConfFile)
	if err != nil || !bytes.Contains(raw, []byte("NGFW_TEST_SNMP_OLD")) || bytes.Contains(raw, []byte("NGFW_TEST_SNMP_NEW")) {
		t.Fatal("rollback selected wrong secret", err)
	}
	value, err := st.loadValue()
	if err != nil || !proto.Equal(value, old) {
		t.Fatal("generation not durably preserved", err)
	}
	//nolint:gosec // Test-owned private state file.
	record, err := os.ReadFile(st.record)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{record, logs.Bytes()} {
		if bytes.Contains(raw, []byte("NGFW_TEST_SNMP_")) {
			t.Fatal("plaintext metadata/log leak")
		}
	}
	if _, err = st.Create(ctx, cfg); err == nil {
		t.Fatal("unbound production reference accepted")
	}
	if err = st.Delete(ctx, old, nil); err != nil {
		t.Fatal(err)
	}
	empty, err := cache.Stage(nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = cache.Activate(empty)
	if err = cache.Retain(empty); err != nil {
		t.Fatal(err)
	}
	if _, err = st.Create(ctx, old); err == nil {
		t.Fatal("revoked generation accepted")
	}
}
