package subsystems

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/internal/vpp/fake"
	"ngfw/agent/internal/vpp/ifsanitize/sanitizetest"
)

func sentinelWiring(globals bool) (*Wiring, *fake.Client, *sanitizetest.Model, *bytes.Buffer) {
	f := fake.New()
	m := sanitizetest.NewModel()
	m.Install(f)
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return &Wiring{env: Env{Client: f, Owner: "w5", GlobalsOwner: globals, Log: log}}, f, m, &buf
}

// D-071: only the globals owner places the sentinel; a test slot sends no classify message at all.
func TestClassifySentinelGlobalsOwnerOnly(t *testing.T) {
	w, f, m, _ := sentinelWiring(false)
	w.classifySentinelConnected(context.Background())
	if n := len(f.Calls()); n != 0 || m.Created != 0 {
		t.Fatalf("slot agent sent %d messages, created %d tables", n, m.Created)
	}

	w, _, m, buf := sentinelWiring(true)
	w.classifySentinelConnected(context.Background())
	if !m.Tables[0] || m.Created != 1 || !strings.Contains(buf.String(), "sentinel created at index 0") {
		t.Fatalf("globals owner: tables %v created %d log %s", m.Tables, m.Created, buf)
	}
	w.classifySentinelConnected(context.Background()) // reconnect without a VPP restart
	if m.Created != 1 || !strings.Contains(buf.String(), "sentinel present") {
		t.Fatalf("reconnect: created %d log %s", m.Created, buf)
	}
}

// A VPP failure is logged, never fatal or panicking (Connected goes on with the boot identity).
func TestClassifySentinelFailureIsLogged(t *testing.T) {
	w, f, _, buf := sentinelWiring(true)
	f.Fail("classify_table_ids", api.VPPApiError(-1))
	w.classifySentinelConnected(context.Background())
	if !strings.Contains(buf.String(), "level=ERROR") || !strings.Contains(buf.String(), "classify_table_ids") {
		t.Fatalf("log %s", buf)
	}
	w, _, m, buf := sentinelWiring(true)
	m.Tables[0] = true // another client's table
	w.classifySentinelConnected(context.Background())
	if !strings.Contains(buf.String(), "level=WARN") || m.Created != 0 {
		t.Fatalf("taken: log %s created %d", buf, m.Created)
	}
}

// Connected calls the sentinel first, before the boot identity (it must win index 0 after a start).
func TestConnectedRunsSentinelFirst(t *testing.T) {
	raw, err := os.ReadFile("subsystems.go")
	if err != nil {
		t.Fatal(err)
	}
	_, body, ok := strings.Cut(string(raw), "func (w *Wiring) Connected(ctx context.Context) {\n")
	if first, _, _ := strings.Cut(body, "\n"); !ok || !strings.Contains(first, "w.classifySentinelConnected(ctx)") {
		t.Fatalf("first statement of Connected: %q", first)
	}
}
