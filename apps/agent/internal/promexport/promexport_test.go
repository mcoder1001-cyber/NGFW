package promexport

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type fakeSource struct {
	snap Snapshot
	err  error
}

func (f fakeSource) Read(context.Context) (Snapshot, error) { return f.snap, f.err }

func sample() Snapshot {
	return Snapshot{
		Interfaces: []InterfaceStats{
			{Name: "loop0", RxBytes: 100, TxBytes: 200, RxPackets: 1, TxPackets: 2, RxDrops: 3, AdminUp: true, LinkUp: true},
			{Name: `weird"if`, RxBytes: 5, AdminUp: false, LinkUp: false},
		},
		Workers: []WorkerStats{{Name: "vpp_main", VectorsPerCall: 1.5, Clocks: 42}},
		Buffers: []BufferStats{{Pool: "default-numa-0", Used: 25, Available: 75}},
		NodeErrors: []NodeError{
			{Node: "ip4-input", Reason: "ttl-expired", Count: 2},
			{Node: "ip4-input", Reason: "checksum", Count: 9},
		},
	}
}

// parseFamilies is a minimal text-format parser: returns name{labels}->value, and asserts # TYPE precedes samples.
func parseFamilies(t *testing.T, b []byte) map[string]string {
	t.Helper()
	out := map[string]string{}
	types := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "# TYPE ") {
			types[strings.Fields(line)[2]] = true
			continue
		}
		if strings.HasPrefix(line, "# HELP ") {
			continue
		}
		if strings.HasPrefix(line, "#") {
			t.Fatalf("unexpected comment line: %q", line)
		}
		sp := strings.LastIndexByte(line, ' ')
		if sp < 0 {
			t.Fatalf("no value in %q", line)
		}
		key, val := line[:sp], line[sp+1:]
		name := key
		if i := strings.IndexByte(key, '{'); i >= 0 {
			name = key[:i]
		}
		if !types[name] {
			t.Fatalf("sample before its # TYPE: %q", line)
		}
		out[key] = val
	}
	return out
}

func TestCollect(t *testing.T) {
	var buf bytes.Buffer
	if err := Collect(context.Background(), fakeSource{snap: sample()}, "w1-", &buf); err != nil {
		t.Fatal(err)
	}
	fam := parseFamilies(t, buf.Bytes())
	if fam[`vrx_interface_rx_bytes_total{interface="w1-loop0"}`] != "100" {
		t.Fatalf("rx bytes: %v", fam)
	}
	if fam[`vrx_interface_rx_drops_total{interface="w1-loop0"}`] != "3" {
		t.Fatal("drops")
	}
	if fam[`vrx_interface_admin_up{interface="w1-loop0"}`] != "1" || fam[`vrx_interface_link_up{interface="w1-weird\"if"}`] != "0" {
		t.Fatalf("admin/link or label escaping wrong: %v", fam)
	}
	if fam[`vrx_worker_vectors_per_call{worker="vpp_main"}`] != "1.5" {
		t.Fatal("worker")
	}
	if fam[`vrx_buffer_used_percent{pool="default-numa-0"}`] != "25" {
		t.Fatalf("buffer pct: %v", fam[`vrx_buffer_used_percent{pool="default-numa-0"}`])
	}
	// top-N ordering: highest count first (both present, count 9 and 2)
	if fam[`vrx_node_errors_total{node="ip4-input",reason="checksum"}`] != "9" {
		t.Fatal("node errors")
	}
}

func TestCollectErrorWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	if err := Collect(context.Background(), fakeSource{err: io.ErrUnexpectedEOF}, "", &buf); err == nil {
		t.Fatal("want the source error")
	}
	if buf.Len() != 0 {
		t.Fatalf("wrote %d bytes on a source error", buf.Len())
	}
}

func TestHandlerAllowList(t *testing.T) {
	allow, err := ParseAllow([]string{"10.0.0.0/8", "192.0.2.7"})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(fakeSource{snap: sample()}, "", allow)
	for remote, want := range map[string]int{
		"10.1.2.3:5000":  200,
		"192.0.2.7:1":    200,
		"203.0.113.1:20": 403,
	} {
		rec := httptestRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/metrics", nil)
		req.RemoteAddr = remote
		h.ServeHTTP(rec, req)
		if rec.code != want {
			t.Fatalf("%s: got %d want %d", remote, rec.code, want)
		}
	}
	// empty allow-list = allow any
	open := NewHandler(fakeSource{snap: sample()}, "", nil)
	rec := httptestRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "203.0.113.9:1"
	open.ServeHTTP(rec, req)
	if rec.code != 200 || !strings.Contains(rec.body.String(), "vrx_interface_rx_bytes_total") {
		t.Fatalf("open handler: %d %q", rec.code, rec.body.String())
	}
}

func TestListenerServes(t *testing.T) {
	var l Listener
	if err := l.Start("127.0.0.1:0", NewHandler(fakeSource{snap: sample()}, "", nil)); err != nil {
		t.Fatal(err)
	}
	defer l.Stop()
	resp, err := http.Get("http://" + l.Addr() + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(string(body), "vrx_worker_vectors_per_call") {
		t.Fatalf("listener: %d %q", resp.StatusCode, body)
	}
	r2, err := http.Get("http://" + l.Addr() + "/other")
	if err != nil {
		t.Fatal(err)
	}
	_ = r2.Body.Close()
	if r2.StatusCode != http.StatusNotFound {
		t.Fatal("non-/metrics path should 404")
	}
}

// tiny ResponseRecorder (avoid importing httptest just for two fields)
type recorder struct {
	code    int
	body    *bytes.Buffer
	headers http.Header
}

func httptestRecorder() *recorder {
	return &recorder{code: 200, body: &bytes.Buffer{}, headers: http.Header{}}
}
func (r *recorder) Header() http.Header         { return r.headers }
func (r *recorder) Write(b []byte) (int, error) { return r.body.Write(b) }
func (r *recorder) WriteHeader(c int)           { r.code = c }

func TestListenerStopCancelsActiveRequestAfterGracePeriod(t *testing.T) {
	entered := make(chan struct{})
	cancelled := make(chan struct{})
	finished := make(chan struct{})
	var l Listener
	h := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
		close(cancelled)
		close(finished)
	})
	if err := l.Start("127.0.0.1:0", h); err != nil {
		t.Fatal(err)
	}
	defer l.Stop()
	response := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + l.Addr() + "/metrics")
		if resp != nil {
			_ = resp.Body.Close()
		}
		response <- err
	}()
	waitSignal(t, entered)
	l.Stop()
	waitSignal(t, cancelled)
	waitSignal(t, finished)
	if l.Addr() != "" {
		t.Fatal("stopped listener still bound")
	}
	select {
	case <-response:
	case <-time.After(5 * time.Second):
		t.Fatal("request survived shutdown")
	}
}
