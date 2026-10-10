package nftables_test

import (
	"net/netip"
	"ngfw/agent/internal/renderers/nftables/nftest"
	"os/exec"
	"testing"
	"time"
)

// Same real API list; protectHost enabled only in the owned nftest namespace.
func TestLabGlobalBlockingSameListProtectHost(t *testing.T) {
	h := nftest.New(t)
	oldHost := h.HostAddr
	for _, entry := range []struct{ ns, dev, ip string }{{h.Host, h.Prefix + "h0", "10.17.1.1/24"}, {h.Peer, h.Prefix + "p0", "10.17.1.2/24"}} {
		if out, err := exec.Command(nftest.IPBin, "-n", entry.ns, "addr", "add", entry.ip, "dev", entry.dev).CombinedOutput(); err != nil {
			t.Fatalf("owned alias: %v %s", err, out)
		}
	}
	h.HostAddr = netip.MustParseAddr("10.17.1.1")
	h.Listen(t, 2525)
	h.HostAddr = oldHost
	h.Listen(t, 2525)
	h.HostAddr = netip.MustParseAddr("10.17.1.1")
	a := newAgent(h)
	doc := gbDoc(t, `{"acl":{"globalBlocking":{"lists":{"proof":{"enabled":true,"source":{"kind":"upload"},"allInterfaces":false,"interfaces":["host-w17l0"],"direction":"both","protectHost":true,"log":false,"entries":["10.17.1.2/32","198.18.17.2/32"]}}}}}`)
	a.apply(t, value(t, doc), false)
	t.Logf("same-list actual namespace rules:\n%s", h.Nft(t, "list", "table", "inet", h.Paths().Table))
	if err := h.Dial(2525, 1500*time.Millisecond); err == nil {
		t.Fatal("listed local TCP passed")
	}
	out, err := exec.Command(nftest.IPBin, "netns", "exec", h.Peer, "ping", "-n", "-I", "10.17.1.2", "-c", "2", "-W", "1", "10.17.1.1").CombinedOutput()
	if err == nil {
		t.Fatalf("listed local ping passed: %s", out)
	}
	t.Logf("listed ping dropped:\n%s", out)
	if n := blockCounter(t, a); n < 3 {
		t.Fatalf("host counter=%d", n)
	} else {
		t.Logf("actual host drop counter=%d", n)
	}
	h.HostAddr = oldHost
	if err := h.Dial(2525, 3*time.Second); err != nil {
		t.Fatalf("unlisted local TCP: %v", err)
	}
	if out, err := exec.Command(nftest.IPBin, "netns", "exec", h.Peer, "ping", "-n", "-I", h.PeerAddr.String(), "-c", "2", "-W", "1", oldHost.String()).CombinedOutput(); err != nil {
		t.Fatalf("unlisted ping: %v %s", err, out)
	}
	t.Log("unlisted local TCP and ping passed")
	a.apply(t, nil, false)
	h.HostAddr = netip.MustParseAddr("10.17.1.1")
	if err := h.Dial(2525, 3*time.Second); err != nil {
		t.Fatalf("listed TCP after removal: %v", err)
	}
	t.Log("same-list protectHost rollback restored TCP")
}
