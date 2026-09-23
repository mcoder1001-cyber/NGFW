package prom

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/http_static"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/scheduler"
)

func TestHTTPStaticServer(t *testing.T) {
	f := dfkittest.NewFake()
	attached := false
	f.On("http_static_enable_v5", func(api.Message) ([]api.Message, error) {
		if attached {
			return []api.Message{&http_static.HTTPStaticEnableV5Reply{Retval: int32(api.APP_ALREADY_ATTACHED)}}, nil
		}
		attached = true
		return []api.Message{&http_static.HTTPStaticEnableV5Reply{}}, nil
	})
	ctx := context.Background()
	d := NewHTTPStaticServer(f)
	v := HTTPStaticServer{URI: "tcp://127.0.0.1/9152", WWWRoot: "/run/vrx-test/w5/www", MaxAge: 600, KeepaliveTimeout: 60, MaxBodySize: 8192}.Proto()
	if d.KeyOf(v) != KeyHTTPStaticServer || d.Dependencies(v) != nil {
		t.Fatal("key/deps")
	}
	for range 2 { // second: APP_ALREADY_ATTACHED, identical → re-apply success
		if _, err := d.Create(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	req := f.CallsNamed("http_static_enable_v5")[0].(*http_static.HTTPStaticEnableV5)
	if req.URI != "tcp://127.0.0.1/9152" || req.WwwRoot != "/run/vrx-test/w5/www" || req.MaxAge != 600 || req.MaxBodySize != 8192 {
		t.Fatalf("request %+v", req)
	}
	if _, err := NewHTTPStaticServer(f).Create(ctx, v); !errors.Is(err, ErrServerBusy) {
		t.Fatalf("other process's server: %v", err)
	}
	if _, err := d.Update(ctx, v, v, nil); !errors.Is(err, dfkit.ErrNotSupported) {
		t.Fatalf("update: %v", err)
	}
	if err := d.Delete(ctx, v, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Retrieve(ctx); !errors.Is(err, dfkit.ErrRetrieveUnsupported) {
		t.Fatal(err)
	}
	for _, bad := range []HTTPStaticServer{
		{URI: "http://127.0.0.1/80", WWWRoot: "/srv"},
		{URI: "tcp://127.0.0.1", WWWRoot: "/srv"},
		{URI: "tcp://localhost/80", WWWRoot: "/srv"},
		{URI: "tcp://127.0.0.1/0", WWWRoot: "/srv"},
		{URI: "tcp://127.0.0.1/80", WWWRoot: "srv"},
		{URI: "tcp://127.0.0.1/80", WWWRoot: "/srv/../etc"},
		{URI: "tcp://127.0.0.1/80", WWWRoot: "/srv dir"},
	} {
		if err := bad.Validate(); !errors.Is(err, dfkit.ErrSpec) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
	r := scheduler.NewRegistry()
	Register(r, f)
	if r.Len() != 1 {
		t.Fatal(r.Names())
	}
}
