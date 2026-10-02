package promexport

import (
	"bytes"
	"context"
	"errors"
	"go.fd.io/govpp/adapter"
	"go.fd.io/govpp/api"
	"go.fd.io/govpp/core"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type provider struct {
	api.StatsProvider
	err error
}

func (p provider) GetInterfaceStats(s *api.InterfaceStats) error {
	if p.err != nil {
		return p.err
	}
	s.Interfaces = []api.InterfaceCounters{{InterfaceName: "w1-test", Rx: api.InterfaceCounterCombined{Packets: 3, Bytes: 300}, Drops: 2}}
	return nil
}
func (provider) GetBufferStats(s *api.BufferStats) error {
	s.Buffer = map[string]api.BufferPool{"default": {Used: 10, Available: 90}}
	return nil
}
func (provider) GetErrorStats(s *api.ErrorStats) error {
	s.Errors = []api.ErrorCounter{{CounterName: "/err/ip4-input/bad length", Values: []uint64{2, 4}}}
	return nil
}

type rawReader struct{ entries []adapter.StatEntry }

func (r rawReader) DumpStats(...string) ([]adapter.StatEntry, error) { return r.entries, nil }
func TestGovppSnapshot(t *testing.T) {
	entries := []adapter.StatEntry{{StatIdentifier: adapter.StatIdentifier{Name: []byte(core.NodeStats_Calls)}, Data: adapter.SimpleCounterStat{{2, 3}, {4}}}, {StatIdentifier: adapter.StatIdentifier{Name: []byte(core.NodeStats_Vectors)}, Data: adapter.SimpleCounterStat{{20, 30}, {20}}}, {StatIdentifier: adapter.StatIdentifier{Name: []byte(core.NodeStats_Clocks)}, Data: adapter.SimpleCounterStat{{100, 150}, {80}}}}
	snap, err := readGovpp(context.Background(), provider{}, rawReader{entries})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Interfaces[0].RxBytes != 300 || snap.Interfaces[0].Drops != 2 || !snap.Interfaces[0].StatsOnly {
		t.Fatal(snap.Interfaces)
	}
	if snap.Workers[0].VectorsPerCall != 10 || snap.Workers[0].Clocks != 5 || snap.Workers[1].VectorsPerCall != 5 {
		t.Fatal(snap.Workers)
	}
	if snap.Buffers[0].Used != 10 || snap.NodeErrors[0].Count != 6 || snap.NodeErrors[0].Reason != "bad length" {
		t.Fatal(snap)
	}
	var b bytes.Buffer
	if err := Collect(context.Background(), fakeSource{snap: snap}, "", &b); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "vrx_interface_link_up{interface=") || strings.Contains(b.String(), "vrx_interface_rx_drops_total{interface=") {
		t.Fatal("invented link/directional state")
	}
	if !strings.Contains(b.String(), "vrx_interface_drops_total{interface=\"w1-test\"} 2") {
		t.Fatal(b.String())
	}
}
func TestGovppReadFailureAndCancel(t *testing.T) {
	sentinel := errors.New("segment unmapped")
	if _, err := readGovpp(context.Background(), provider{err: sentinel}, rawReader{}); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := NewGovppSource("/does/not/exist")
	if _, err := s.Read(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	s.Close()
	s.Close()
}
func TestGovppLiveStats(t *testing.T) {
	if os.Getenv("VRX_INTEGRATION") != "1" {
		t.Skip("requires real VPP stats segment")
	}
	s := NewGovppSource(os.Getenv("VRX_AGENT_VPP_STATS_SOCKET"))
	defer s.Close()
	snap, err := s.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Interfaces) == 0 {
		t.Fatal("real VPP returned no interface counters")
	}
	s.Close()
	if _, err := s.Read(context.Background()); err != nil {
		t.Fatal("reconnect:", err)
	}
}

type blockedSession struct {
	provider
	entered  chan struct{}
	release  chan struct{}
	disposed chan struct{}
}

func (s *blockedSession) GetInterfaceStats(st *api.InterfaceStats) error {
	close(s.entered)
	<-s.release
	return s.provider.GetInterfaceStats(st)
}
func (s *blockedSession) Disconnect() { close(s.disposed) }

// signaledContext reveals when Read has started installing its cancellation
// bridge before waiting for the occupied source mutex.
type signaledContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func (c *signaledContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.Context.Done()
}
func waitSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("barrier timed out")
	}
}
func TestGovppTerminalStopDisposesActiveAndQueuedReads(t *testing.T) {
	source := NewGovppSource("test")
	session := &blockedSession{entered: make(chan struct{}), release: make(chan struct{}), disposed: make(chan struct{})}
	var connects atomic.Int32
	source.connect = func(string) (statsSession, workerReader, error) { connects.Add(1); return session, rawReader{}, nil }
	active := make(chan error, 1)
	go func() { _, err := source.Read(context.Background()); active <- err }()
	waitSignal(t, session.entered)
	queuedCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	observed := &signaledContext{Context: queuedCtx, observed: make(chan struct{})}
	queued := make(chan error, 1)
	go func() { _, err := source.Read(observed); queued <- err }()
	waitSignal(t, observed.observed)
	stopped := make(chan struct{})
	go func() { source.Stop(); close(stopped) }()
	waitSignal(t, source.stopCtx.Done())
	close(session.release)
	waitSignal(t, stopped)
	waitSignal(t, session.disposed)
	for _, result := range []<-chan error{active, queued} {
		select {
		case err := <-result:
			if !errors.Is(err, context.Canceled) {
				t.Fatal("shutdown read:", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("read survived shutdown")
		}
	}
	source.Close()
	source.Stop()
	if _, err := source.Read(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatal("post-stop read:", err)
	}
	if connects.Load() != 1 {
		t.Fatal("source reconnected after terminal stop", connects.Load())
	}
}

type reusableSession struct {
	provider
	disposed *atomic.Int32
}

func (s reusableSession) Disconnect() { s.disposed.Add(1) }
func TestGovppCloseRemainsReconnectable(t *testing.T) {
	source := NewGovppSource("test")
	defer source.Stop()
	var connected, disposed atomic.Int32
	source.connect = func(string) (statsSession, workerReader, error) {
		connected.Add(1)
		return reusableSession{disposed: &disposed}, rawReader{}, nil
	}
	if _, err := source.Read(context.Background()); err != nil {
		t.Fatal(err)
	}
	source.Close()
	if disposed.Load() != 1 {
		t.Fatal("reset did not dispose mapping")
	}
	if _, err := source.Read(context.Background()); err != nil {
		t.Fatal("transient reset prevented reconnect", err)
	}
	if connected.Load() != 2 {
		t.Fatal("reset did not reconnect")
	}
	source.Stop()
	if disposed.Load() != 2 {
		t.Fatal("terminal shutdown did not dispose reconnected mapping")
	}
}
