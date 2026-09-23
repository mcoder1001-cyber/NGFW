package strongswan

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"
	"sync"

	"github.com/strongswan/govici/vici"
)

// fakeCharon models the VICI surface of charon the renderer uses: loaded connections, shared
// secrets, pools, authorities, IKE_SAs and event subscribers. It records every request.
type fakeCharon struct {
	mu          sync.Mutex
	conns       map[string]*vici.Message // name → load-conn body
	connOrder   []string
	shared      map[string]sharedSecret
	pools       map[string]*vici.Message
	authorities map[string]*vici.Message
	sas         map[string][]*vici.Message // conn → list-sa bodies
	calls       []fakeCall
	// failOn makes a command fail with errmsg (after recording it).
	failOn map[string]string
	// ignoreLoad makes load-conn of that name report success without loading it.
	ignoreLoad map[string]bool
	// down makes Dial fail.
	down bool
	// subscribers get events pushed by the test.
	subscribers []chan<- vici.Event
}

type fakeCall struct {
	cmd string
	in  *vici.Message
}

func newFakeCharon() *fakeCharon {
	return &fakeCharon{
		conns: map[string]*vici.Message{}, shared: map[string]sharedSecret{}, pools: map[string]*vici.Message{},
		authorities: map[string]*vici.Message{}, sas: map[string][]*vici.Message{}, failOn: map[string]string{}, ignoreLoad: map[string]bool{},
	}
}

func (f *fakeCharon) dialer() Dialer {
	return func(_ context.Context, socket string) (ViciConn, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.down {
			return nil, errors.New("dial unix " + socket + ": connect: no such file or directory")
		}
		return &fakeSession{f: f}, nil
	}
}

func (f *fakeCharon) callNames() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		out = append(out, c.cmd)
	}
	return out
}

func (f *fakeCharon) loadedConns() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Sorted(mapKeys(f.conns))
}

func (f *fakeCharon) loadedShared() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Sorted(mapKeys(f.shared))
}

// push delivers an event to every subscriber (non-blocking like govici).
func (f *fakeCharon) push(ev vici.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.subscribers {
		select {
		case c <- ev:
		default:
		}
	}
}

// restart closes every subscriber channel (charon went away).
func (f *fakeCharon) restart(down bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.subscribers {
		close(c)
	}
	f.subscribers = nil
	f.down = down
}

type fakeSession struct {
	f      *fakeCharon
	closed bool
	notify []chan<- vici.Event
}

func (s *fakeSession) Close() error {
	s.f.mu.Lock()
	defer s.f.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	for _, c := range s.notify {
		if i := slices.Index(s.f.subscribers, c); i >= 0 {
			s.f.subscribers = slices.Delete(s.f.subscribers, i, i+1)
			close(c)
		}
	}
	return nil
}

func (s *fakeSession) Subscribe(_ ...string) error {
	s.f.mu.Lock()
	defer s.f.mu.Unlock()
	if s.f.failOn["subscribe"] != "" {
		return errors.New(s.f.failOn["subscribe"])
	}
	return nil
}

func (s *fakeSession) NotifyEvents(c chan<- vici.Event) {
	s.f.mu.Lock()
	defer s.f.mu.Unlock()
	s.notify = append(s.notify, c)
	s.f.subscribers = append(s.f.subscribers, c)
}

func (s *fakeSession) Call(_ context.Context, cmd string, in *vici.Message) (*vici.Message, error) {
	f := s.f
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{cmd: cmd, in: in})
	if e, ok := f.failOn[cmd]; ok {
		return msg("success", "no", "errmsg", e), fmt.Errorf("command failed: %s", e)
	}
	ok := msg("success", "yes")
	switch cmd {
	case "version":
		return msg("daemon", "charon-systemd", "version", "6.0.4", "sysname", "Linux", "release", "7.0.0", "machine", "x86_64"), nil
	case "stats":
		total := 0
		for _, l := range f.sas {
			total += len(l)
		}
		return msg("uptime", msg("running", "1 minute", "since", "Sep 24 01:00:00 2026"),
			"workers", msg("total", "16", "idle", "11"), "ikesas", msg("total", fmt.Sprint(total), "half-open", "0"),
			"plugins", []string{"charon-systemd", "vici"}), nil
	case "load-conn":
		for _, name := range in.Keys() {
			if f.ignoreLoad[name] {
				continue
			}
			if _, exists := f.conns[name]; !exists {
				f.connOrder = append(f.connOrder, name)
			}
			f.conns[name] = sub(in, name)
		}
		return ok, nil
	case "unload-conn":
		name := str(in, "name")
		if _, exists := f.conns[name]; !exists {
			return msg("success", "no", "errmsg", "unloading connection '"+name+"' failed"), errors.New("command failed")
		}
		delete(f.conns, name)
		return ok, nil
	case "get-conns":
		return msg("conns", slices.Sorted(mapKeys(f.conns))), nil
	case "load-shared":
		f.shared[str(in, "id")] = sharedSecret{id: str(in, "id"), data: []byte(str(in, "data")), owners: strs(in, "owners")}
		return ok, nil
	case "unload-shared":
		delete(f.shared, str(in, "id"))
		return ok, nil
	case "get-shared":
		return msg("keys", slices.Sorted(mapKeys(f.shared))), nil
	case "load-pool":
		for _, name := range in.Keys() {
			f.pools[name] = sub(in, name)
		}
		return ok, nil
	case "unload-pool":
		delete(f.pools, str(in, "name"))
		return ok, nil
	case "get-pools":
		m := vici.NewMessage()
		for _, name := range slices.Sorted(mapKeys(f.pools)) {
			_ = m.Set(name, msg("base", "10.99.0.0", "size", "254", "online", "0", "offline", "0"))
		}
		return m, nil
	case "load-authority":
		for _, name := range in.Keys() {
			f.authorities[name] = sub(in, name)
		}
		return ok, nil
	case "unload-authority":
		delete(f.authorities, str(in, "name"))
		return ok, nil
	case "get-authorities":
		return msg("authorities", slices.Sorted(mapKeys(f.authorities))), nil
	case "terminate":
		delete(f.sas, str(in, "ike"))
		return ok, nil
	}
	return msg("success", "no", "errmsg", "unknown command "+cmd), errors.New("unknown command " + cmd)
}

// listConn renders a loaded body the way charon's list-conn event shows it.
func listConnView(body *vici.Message) *vici.Message {
	m := vici.NewMessage()
	_ = m.Set("local_addrs", strs(body, "local_addrs"))
	_ = m.Set("remote_addrs", strs(body, "remote_addrs"))
	switch str(body, "version") {
	case "1":
		_ = m.Set("version", "IKEv1")
	case "2":
		_ = m.Set("version", "IKEv2")
	default:
		_ = m.Set("version", "0")
	}
	_ = m.Set("rekey_time", "14400")
	_ = m.Set("reauth_time", "0")
	for _, side := range []string{"local", "remote"} {
		a := sub(body, side)
		class := "public key"
		if str(a, "auth") == "psk" {
			class = "pre-shared key"
		}
		_ = m.Set(side+"-1", msg("class", class, "id", str(a, "id")))
	}
	children := vici.NewMessage()
	for _, name := range sub(body, "children").Keys() {
		c := sub(sub(body, "children"), name)
		mode := strings.ToUpper(str(c, "mode"))
		if mode == "" {
			mode = "TUNNEL"
		}
		lts, rts := strs(c, "local_ts"), strs(c, "remote_ts")
		if lts == nil {
			lts = []string{"dynamic"}
		}
		if rts == nil {
			rts = []string{"dynamic"}
		}
		_ = children.Set(name, msg("mode", mode, "rekey_time", "3600", "local-ts", lts, "remote-ts", rts))
	}
	_ = m.Set("children", children)
	return m
}

func (s *fakeSession) CallStreaming(_ context.Context, cmd, _ string, in *vici.Message) iter.Seq2[*vici.Message, error] {
	return func(yield func(*vici.Message, error) bool) {
		f := s.f
		f.mu.Lock()
		f.calls = append(f.calls, fakeCall{cmd: cmd, in: in})
		var events []*vici.Message
		var fail string
		if e, ok := f.failOn[cmd]; ok {
			fail = e
		}
		switch cmd {
		case "list-conns":
			for _, name := range f.connOrder {
				body, ok := f.conns[name]
				if !ok || (str(in, "ike") != "" && str(in, "ike") != name) {
					continue
				}
				events = append(events, msg(name, listConnView(body)))
			}
		case "list-sas":
			for _, name := range slices.Sorted(mapKeys(f.sas)) {
				if str(in, "ike") != "" && str(in, "ike") != name {
					continue
				}
				for _, sa := range f.sas[name] {
					events = append(events, msg(name, sa))
				}
			}
		case "initiate", "terminate":
			events = append(events, msg("group", "IKE", "level", "1", "msg", "fake "+cmd))
		}
		f.mu.Unlock()
		if fail != "" {
			yield(msg("success", "no", "errmsg", fail), errors.New("command failed: "+fail))
			return
		}
		for _, e := range events {
			if !yield(e, nil) {
				return
			}
		}
	}
}

// saMsg builds a list-sa body with one child.
func saMsg(uniqueID, state, childName, childID, childState string) *vici.Message {
	child := msg("name", childName, "uniqueid", childID, "reqid", "1", "state", childState, "mode", "TUNNEL", "protocol", "ESP",
		"encap", "no", "spi-in", "c1a2b3c4", "spi-out", "d5e6f708", "encr-alg", "AES_GCM_16", "encr-keysize", "256",
		"bytes-in", "840", "packets-in", "10", "bytes-out", "1680", "packets-out", "20", "rekey-time", "3000",
		"life-time", "3500", "install-time", "12", "local-ts", []string{"10.3.1.0/24"}, "remote-ts", []string{"10.3.2.0/24"})
	return msg("uniqueid", uniqueID, "version", "2", "state", state, "local-host", "10.3.250.1", "local-port", "500",
		"local-id", "10.3.250.1", "remote-host", "10.3.250.2", "remote-port", "500", "remote-id", "10.3.250.2",
		"initiator", "yes", "initiator-spi", "0102030405060708", "responder-spi", "1112131415161718",
		"encr-alg", "AES_GCM_16", "encr-keysize", "256", "prf-alg", "PRF_HMAC_SHA2_256", "dh-group", "CURVE_25519",
		"established", "12", "rekey-time", "13000", "child-sas", msg(childName+"-"+childID, child))
}
