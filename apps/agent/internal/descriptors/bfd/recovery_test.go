package bfd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"
	binbfd "ngfw/agent/binapi/bfd"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/descriptors/dfkit"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
)

type failingInterfaceClaims struct {
	iface.ClaimStore
	fail bool
}

func (c *failingInterfaceClaims) Persistent() bool { return true }
func (c *failingInterfaceClaims) Claim(n, h string) error {
	if c.fail {
		return errors.New("interface claim disk full")
	}
	return c.ClaimStore.Claim(n, h)
}

type failingEndpointClaims struct {
	testEndpointClaims
	fail bool
}

func (c *failingEndpointClaims) Persistent() bool { return true }
func (c *failingEndpointClaims) Claim(l, p, n string) error {
	if c.fail {
		return errors.New("endpoint claim disk full")
	}
	return c.testEndpointClaims.Claim(l, p, n)
}

// The native interface is provisioned before this single-descriptor scheduler
// fixture. Only its BFD object participates in this tested transaction.
type recoverySchedulerDescriptor struct{ *SessionDescriptor }

func (*recoverySchedulerDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }
func recoveryScheduler(d *SessionDescriptor) *scheduler.Scheduler {
	r := scheduler.NewRegistry()
	r.Register(&recoverySchedulerDescriptor{d})
	s := scheduler.New(r, nil)
	s.VerifyRetries = 0
	return s
}

func TestSessionPartialCreateDegradedAndDurableRestartCleanup(t *testing.T) {
	for _, stage := range []string{"interface-claim", "endpoint-claim", "flags"} {
		t.Run(stage, func(t *testing.T) {
			f, _, sessions := fakeBFD()
			path := filepath.Join(t.TempDir(), "successful-add.json")
			store, err := dfkit.NewFileBootStore(path)
			if err != nil {
				t.Fatal(err)
			}
			df7.SetBootStore(df7test.Owner, store)
			claims := &failingInterfaceClaims{ClaimStore: iface.NewMemoryClaimStore(), fail: stage == "interface-claim"}
			iface.SetClaimStore(df7test.Owner, claims)
			endpoints := &failingEndpointClaims{testEndpointClaims: testEndpointClaims{}, fail: stage == "endpoint-claim"}
			SetMultihopEnvironment(df7test.Owner, &MultihopEnvironment{Claims: endpoints, Enable: func(context.Context) error { return nil }})
			t.Cleanup(func() {
				df7.SetBootStore(df7test.Owner, nil)
				iface.SetClaimStore(df7test.Owner, nil)
				SetMultihopEnvironment(df7test.Owner, nil)
			})
			failDelete := true
			f.On("bfd_udp_del", func(m api.Message) ([]api.Message, error) {
				if failDelete {
					return []api.Message{&binbfd.BfdUDPDelReply{Retval: -1}}, nil
				}
				r := m.(*binbfd.BfdUDPDel)
				delete(sessions, fmt.Sprintf("%s>%s@%d", r.LocalAddr.String(), r.PeerAddr.String(), r.SwIfIndex))
				return []api.Message{&binbfd.BfdUDPDelReply{}}, nil
			})
			if stage == "flags" {
				f.Reply("bfd_udp_session_set_flags", &binbfd.BfdUDPSessionSetFlagsReply{Retval: -1})
			}
			spec := Session{Interface: "eth0", Local: "10.0.0.1", Peer: "10.0.0.2", Multihop: stage != "interface-claim", AdminDown: stage == "flags", DesiredMinTx: 300000, RequiredMinRx: 300000, DetectMult: 3}
			d := NewSession(f, df7test.Owner)
			result := recoveryScheduler(d).Apply(t.Context(), []scheduler.KV{df7.KV(KeySession(spec.Interface, spec.Local, spec.Peer), spec, nil)}, nil)
			if result.Outcome != scheduler.OutcomeDegraded || len(result.Results) != 1 || result.Results[0].Code != scheduler.CodeRevertFailed || len(sessions) != 1 {
				t.Fatalf("must expose orphan as degraded: %+v, native=%d", result, len(sessions))
			}
			if len(f.CallsNamed("bfd_udp_del")) != 2 {
				t.Fatal("compensation plus scheduler rollback must both attempt exact deletion")
			}
			// Lose every in-process metadata value: reopen the persisted successful-add
			// store and use a fresh descriptor and actual scheduler to recover the orphan.
			reopened, err := dfkit.NewFileBootStore(path)
			if err != nil {
				t.Fatal(err)
			}
			df7.SetBootStore(df7test.Owner, reopened)
			fresh := NewSession(f, df7test.Owner)
			actual, err := fresh.Retrieve(t.Context())
			if err != nil || len(actual) != 1 {
				t.Fatalf("durable orphan readback: %v %v", actual, err)
			}
			failDelete = false
			result = recoveryScheduler(fresh).Apply(t.Context(), nil, nil)
			if result.Outcome != scheduler.OutcomeApplied || len(sessions) != 0 {
				t.Fatalf("restart recovery: %+v native=%d", result, len(sessions))
			}
			if _, ok := reopened.Get(recoveryKey(func() uint32 {
				if spec.Multihop {
					return df7.NoIndex
				}
				return 4
			}(), spec.Local, spec.Peer)); ok {
				t.Fatal("successful-add recovery record leaked")
			}
		})
	}
}

func TestSessionRecoveryProofRefusesBootAndIndexReuse(t *testing.T) {
	for _, reuse := range []string{"boot-same-pid", "interface-index"} {
		t.Run(reuse, func(t *testing.T) {
			f, _, sessions := fakeBFD()
			df7.SetBootStore(df7test.Owner, nil)
			t.Cleanup(func() { df7.SetBootStore(df7test.Owner, nil) })
			spec := Session{Interface: "eth0", Local: "10.0.0.1", Peer: "10.0.0.2", DesiredMinTx: 300000, RequiredMinRx: 300000, DetectMult: 3}
			d := NewSession(f, df7test.Owner)
			metadata, err := d.Create(t.Context(), df7.Encode(spec))
			if err != nil {
				t.Fatal(err)
			}
			if reuse == "boot-same-pid" {
				f.RestartSamePID()
			} else {
				f.RemoveIf(4)
				f.AddIf(df7test.FakeIf{Index: 7, Name: "eth0"})
			}
			calls := len(f.CallsNamed("bfd_udp_del"))
			if err := d.Delete(t.Context(), df7.Encode(spec), metadata); err != nil {
				t.Fatal(err)
			}
			if len(f.CallsNamed("bfd_udp_del")) != calls || len(sessions) != 1 {
				t.Fatal("expired proof touched reused native session")
			}
		})
	}
}

func TestSessionFailedAddNeverWritesRecoveryProof(t *testing.T) {
	f, _, _ := fakeBFD()
	df7.SetBootStore(df7test.Owner, nil)
	t.Cleanup(func() { df7.SetBootStore(df7test.Owner, nil) })
	f.Reply("bfd_udp_add", &binbfd.BfdUDPAddReply{Retval: int32(api.BFD_EEXIST)})
	spec := Session{Interface: "eth0", Local: "10.0.0.1", Peer: "10.0.0.2", DesiredMinTx: 300000, RequiredMinRx: 300000, DetectMult: 3}
	metadata, err := NewSession(f, df7test.Owner).Create(t.Context(), df7.Encode(spec))
	if err == nil || scheduler.IsPartialCreate(err) || metadata != nil {
		t.Fatal("existing foreign session adopted as partial create")
	}
	if _, ok := df7.BootStoreFor(df7test.Owner).Get(recoveryKey(4, spec.Local, spec.Peer)); ok || len(f.CallsNamed("bfd_udp_del")) != 0 {
		t.Fatal("failed add acquired proof or deleted pre-existing session")
	}
}

type failingProofStore struct {
	dfkit.BootStore
	fail bool
}

func (*failingProofStore) Persistent() bool { return true }
func (s *failingProofStore) Put(r dfkit.BootRecord) error {
	if s.fail {
		return errors.New("successful-add proof disk full")
	}
	return s.BootStore.Put(r)
}

func TestSessionProofWriteFailureRemainsPartialOnCleanupFailure(t *testing.T) {
	f, _, sessions := fakeBFD()
	store := &failingProofStore{BootStore: dfkit.NewMemoryBootStore(), fail: true}
	df7.SetBootStore(df7test.Owner, store)
	t.Cleanup(func() { df7.SetBootStore(df7test.Owner, nil) })
	d := NewSession(f, df7test.Owner)
	spec := Session{Interface: "eth0", Local: "10.0.0.1", Peer: "10.0.0.2", DesiredMinTx: 300000, RequiredMinRx: 300000, DetectMult: 3}
	f.Reply("bfd_udp_del", &binbfd.BfdUDPDelReply{Retval: -1})
	metadata, err := d.Create(t.Context(), df7.Encode(spec))
	if !scheduler.IsPartialCreate(err) || metadata == nil || len(sessions) != 1 {
		t.Fatalf("failed proof plus cleanup must remain partial: %v %v", metadata, err)
	}
	if _, ok := store.Get(recoveryKey(4, spec.Local, spec.Peer)); ok {
		t.Fatal("unsuccessful durable write falsely claimed")
	}
	// Recovery within the still-running process uses exact successful-add metadata.
	// A total storage outage cannot be represented as crash-durable acceptance.
	f.On("bfd_udp_del", func(m api.Message) ([]api.Message, error) {
		r := m.(*binbfd.BfdUDPDel)
		delete(sessions, fmt.Sprintf("%s>%s@%d", r.LocalAddr.String(), r.PeerAddr.String(), r.SwIfIndex))
		return []api.Message{&binbfd.BfdUDPDelReply{}}, nil
	})
	if err := d.Delete(t.Context(), df7.Encode(spec), metadata); err != nil || len(sessions) != 0 {
		t.Fatalf("same-process precise recovery: %v", err)
	}
}
