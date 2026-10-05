package strongswan

import (
	"context"
	"github.com/strongswan/govici/vici"
	"iter"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

type raFake struct {
	events     chan<- vici.Event
	entries    []*vici.Message
	count      int
	foreign    bool
	dropped    bool
	terminated string
	after      int
}

func (f *raFake) Call(_ context.Context, cmd string, in *vici.Message) (*vici.Message, error) {
	switch cmd {
	case "get-conns":
		name := "ra-road"
		if f.foreign {
			name = "foreign"
		}
		return msg("conns", []string{name}), nil
	case "stats":
		n := f.count
		if f.after != 0 {
			n = f.after
		}
		return msg("ikesas", msg("total", strconv.Itoa(n))), nil
	case "list-sas":
		for index, entry := range f.entries {
			if f.dropped && index == 0 {
				continue
			}
			f.events <- vici.Event{Name: "list-sa", Message: msg("ra-road", entry)}
		}
		return msg("success", "yes"), nil
	case "terminate":
		f.terminated = str(in, "ike-id")
		f.entries = nil
		f.count = 0
		return msg("success", "yes"), nil
	}
	return msg("success", "yes"), nil
}
func (f *raFake) CallStreaming(context.Context, string, string, *vici.Message) iter.Seq2[*vici.Message, error] {
	return func(func(*vici.Message, error) bool) {}
}
func (f *raFake) Subscribe(...string) error        { return nil }
func (f *raFake) NotifyEvents(c chan<- vici.Event) { f.events = c }
func (f *raFake) Close() error                     { return nil }
func raSA(id int) *vici.Message {
	return msg("uniqueid", strconv.Itoa(id), "state", "ESTABLISHED", "remote-eap-id", "client", "remote-vips", []string{"10.19.200.5"}, "established", "42", "child-sas", msg("protected-1", msg("state", "INSTALLED", "if-id-in", "00000001", "if-id-out", "00000001", "bytes-in", "9007199254740993", "bytes-out", "7")))
}
func TestRAObservedSessionsBeyondGoviciDefaultBuffer(t *testing.T) {
	p, _ := raFixture(t)
	fake := &raFake{count: 200}
	for id := 1; id <= 200; id++ {
		fake.entries = append(fake.entries, raSA(id))
	}
	sessions, err := ObserveRASessions(context.Background(), fake, "road", "generation-one", p.GetPools())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 200 || sessions[0].BytesIn != 9007199254740993 {
		t.Fatal("complete exact session facts lost")
	}
}
func TestRARejectsIncompleteOrForeignObservation(t *testing.T) {
	p, _ := raFixture(t)
	for _, fake := range []*raFake{{count: 1, foreign: true}, {count: 1, entries: []*vici.Message{raSA(1)}, dropped: true}, {count: MaxRASessions + 1}, {count: 2, entries: []*vici.Message{raSA(1), raSA(1)}}} {
		if _, err := ObserveRASessions(context.Background(), fake, "road", "generation-one", p.GetPools()); err == nil {
			t.Fatal("unsafe snapshot accepted")
		}
	}
}
func TestRADisconnectRequiresObservedGenerationMembership(t *testing.T) {
	p, _ := raFixture(t)
	fake := &raFake{count: 1, entries: []*vici.Message{raSA(19)}}
	sessions, err := ObserveRASessions(context.Background(), fake, "road", "generation-one", p.GetPools())
	if err != nil {
		t.Fatal(err)
	}
	if err := DisconnectRASession(context.Background(), fake, "road", "generation-two", sessions[0].ID, p.GetPools()); err == nil || fake.terminated != "" {
		t.Fatal("stale generation terminated SA")
	}
	if err := DisconnectRASession(context.Background(), fake, "road", "generation-one", sessions[0].ID, p.GetPools()); err != nil {
		t.Fatal(err)
	}
	if fake.terminated != "19" {
		t.Fatal("did not terminate exact owned unique id")
	}
}

func TestRAVICISocketRequiresPrivateModeAndExactDaemonPID(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); /* #nosec G302 -- private fixture directory needs owner traversal. */ err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(root, "vici.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	if err := os.Chmod(socket, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := DialRAVICI(context.Background(), socket, os.Getpid()+1); err == nil {
		t.Fatal("foreign daemon PID accepted")
	}
	if os.Geteuid() == 0 {
		session, err := DialRAVICI(context.Background(), socket, os.Getpid())
		if err != nil {
			t.Fatal(err)
		}
		_ = session.Close()
	} else if _, err := DialRAVICI(context.Background(), socket, os.Getpid()); err == nil {
		t.Fatal("non-root private daemon accepted")
	}
	if err := os.Chmod(socket, 0640); /* #nosec G302 -- negative test verifies rejection of group-accessible sockets. */ err != nil {
		t.Fatal(err)
	}
	if _, err := DialRAVICI(context.Background(), socket, os.Getpid()); err == nil {
		t.Fatal("group-accessible VICI socket accepted")
	}
}
