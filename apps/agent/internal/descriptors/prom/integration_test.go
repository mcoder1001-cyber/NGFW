package prom

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"ngfw/agent/binapi/http_static"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/vpp/vpptest"
)

// Integration test against the host VPP. http_static can be enabled once per VPP process and never
// disabled, and enabling it switches the session layer on for everyone: the full run is opt-in
// (VRX_DF8_HTTP_STATIC=1) and listens on 127.0.0.1:$((VRX_METRICS_PORT+1)) with a www root under
// /run/vrx-test/<prefix>/. By default the test only checks that VPP knows the message.
func TestHTTPStaticOnHost(t *testing.T) {
	h := dfkittest.ConnectHost(t)
	h.SkipUnlessCompatible(t, Plugin, &http_static.HTTPStaticEnableV5{})
	c := h.Client()
	d := NewHTTPStaticServer(c)
	if _, err := d.Retrieve(context.Background()); !errors.Is(err, dfkit.ErrRetrieveUnsupported) {
		t.Fatal(err)
	}
	if os.Getenv("VRX_DF8_HTTP_STATIC") != "1" {
		t.Skip("http_static_enable_v5 is irreversible until a VPP restart and enables the session layer; set VRX_DF8_HTTP_STATIC=1 to run")
	}
	port := 9100 + 10*vpptest.Slot(t) + 2
	if p := os.Getenv("VRX_METRICS_PORT"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil {
			t.Fatal(err)
		}
		port = n + 1
	}
	root := filepath.Join("/run/vrx-test", h.Owner, "www")
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	v := HTTPStaticServer{URI: fmt.Sprintf("tcp://127.0.0.1/%d", port), WWWRoot: root, MaxAge: 600, KeepaliveTimeout: 60, MaxBodySize: 8192}.Proto()
	if _, err := d.Create(context.Background(), v); errors.Is(err, ErrServerBusy) {
		t.Skipf("http_static is already enabled in this VPP process: %v", err)
	} else if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Create(context.Background(), v); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	t.Logf("http_static listening on 127.0.0.1:%d until the next VPP restart (no disable API)", port)
}
