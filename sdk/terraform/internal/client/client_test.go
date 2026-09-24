package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPointers(t *testing.T) {
	for in, want := range map[string]string{"interfaces/loop1/": "/interfaces/loop1", "/system/hostname": "/system/hostname"} {
		if got, err := NormalizePointer(in); err != nil || got != want {
			t.Errorf("NormalizePointer(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "/", "/a/~2", "/a//b"} {
		if _, err := NormalizePointer(bad); err == nil {
			t.Errorf("NormalizePointer(%q) accepted", bad)
		}
	}
	if got := URLPath("/interfaces/" + EscapeToken("TenGigabitEthernet0/0/0") + "/a b"); got != "interfaces/TenGigabitEthernet0~10~10/a%20b" {
		t.Errorf("URLPath = %q", got)
	}
}

func TestNoRedirectNoKeyLeak(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("the API key followed a redirect to another origin")
		}
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/steal", http.StatusFound)
	}))
	defer srv.Close()
	c, err := New(Options{URL: srv.URL, APIKey: "VRX_TEST_PSK_client"})
	if err != nil {
		t.Fatal(err)
	}
	err = c.Do(context.Background(), http.MethodGet, "/api/v1/config", nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "302") {
		t.Fatalf("want the 302 as an error, got %v", err)
	}
	if strings.Contains(c.String(), "VRX_TEST_PSK_client") || strings.Contains(err.Error(), "VRX_TEST_PSK_client") {
		t.Fatal("key in String() or error")
	}
}

func TestProblemParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"type":"x","title":"Locked","status":409,"detail":"candidate locked","lock":{"owner":"alice"}}`))
	}))
	defer srv.Close()
	c, _ := New(Options{URL: srv.URL, APIKey: "VRX_TEST_PSK_client"})
	err := c.Put(context.Background(), "/interfaces/loop1", map[string]any{})
	ae, ok := err.(*APIError)
	if !ok || ae.Status != 409 || !strings.Contains(err.Error(), `lock: {"owner":"alice"}`) {
		t.Fatalf("got %v", err)
	}
}
