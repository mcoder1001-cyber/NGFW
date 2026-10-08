package secretchannel

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"
)

func TestWireguardSealedRotationRestartRevoke(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, "wg")
	if err != nil {
		t.Fatal(err)
	}
	stage := func(b byte) string {
		t.Helper()
		raw := bytes.Repeat([]byte{b}, 32)
		text := []byte(base64.StdEncoding.EncodeToString(raw))
		id, e := s.Stage(map[string][]byte{"key/wg": text, "psk/peer": text})
		if e != nil {
			t.Fatal(e)
		}
		if e = s.Activate(id); e != nil {
			t.Fatal(e)
		}
		return id
	}
	old := stage(1)
	private, err := s.WireguardRef("key/wg")
	if err != nil {
		t.Fatal(err)
	}
	psk, err := s.WireguardRef("psk/peer")
	if err != nil {
		t.Fatal(err)
	}
	newer := stage(2)
	next, _ := s.WireguardRef("key/wg")
	if next == private {
		t.Fatal("rotation did not change key identity")
	}
	s, err = Open(dir, "wg")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Activate(newer); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{private, psk} {
		raw, e := s.ResolveWireguard(context.Background(), ref)
		if e != nil || !bytes.Equal(raw, bytes.Repeat([]byte{1}, 32)) {
			t.Fatal("historical rollback key unavailable", e)
		}
	}
	if err = s.Activate(old); err != nil {
		t.Fatal(err)
	}
	got, _ := s.WireguardRef("key/wg")
	if got != private {
		t.Fatal("confirmed key not restored")
	}
	empty, err := s.Stage(nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Activate(empty)
	if _, err = s.WireguardRef("key/wg"); err == nil {
		t.Fatal("revoked active key usable")
	}
	if err = s.Retain(empty); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ResolveWireguard(context.Background(), private); err == nil {
		t.Fatal("obsolete key survived retention")
	}
}

func TestWireguardSealedRejectsNoncanonicalKey(t *testing.T) {
	s, e := Open(t.TempDir(), "wg")
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range []string{"not-a-key", string(bytes.Repeat([]byte{'a'}, 32)), base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)) + "\n"} {
		id, e := s.Stage(map[string][]byte{"key/wg": []byte(v)})
		if e != nil {
			t.Fatal(e)
		}
		_ = s.Activate(id)
		if _, e = s.WireguardRef("key/wg"); e == nil {
			t.Fatal("invalid key accepted")
		}
	}
}
