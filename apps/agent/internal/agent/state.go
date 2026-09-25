package agent

// Persistent agent state in the state dir (VRX_AGENT_STATE_DIR, product /var/lib/vrx/agent):
//
//	agent-state.json  THE state, written atomically as one file (temp + fsync + rename): managed
//	                  domains, last/pending txn, confirm deadline, "revert owed" flag, recent Apply
//	                  responses, and both documents (current desired state and confirmed baseline,
//	                  protobuf JSON). One file means a crash can never pair a pending document with
//	                  metadata that says "confirmed" (review M3).
//	desired.pb        binary mirror of the current desired state for operators/tools, written after
//	                  agent-state.json and never read back (except to migrate a pre-review state dir)
//	owned-<owner>.json  owner table (internal/ownertable)
//
// DesiredState has no secret fields (D-040), so neither file carries secrets.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/ownertable"
)

// historySize is how many Apply responses are kept for txn_id retry idempotency (contract ≥ 16).
const historySize = 32

// stateFile is the authoritative state file name.
const stateFile = "agent-state.json"

// saveHook, when set (tests only), is called before each write stage ("state", "mirror"); a
// non-nil error aborts save at that point, simulating a crash there.
var saveHook func(stage string) error

// errMirror wraps a failure to write the desired.pb mirror: agent-state.json — the state — was written
// already, so the transaction is durable (TD-9, ARCH-01: only a failed state write answers DEGRADED).
var errMirror = errors.New("state: desired.pb mirror")

// txnRecord is one remembered Apply.
type txnRecord struct {
	TxnID       string `json:"txn_id"`
	Fingerprint string `json:"fingerprint"`
	// Response is the protojson ApplyResponse.
	Response json.RawMessage `json:"response"`
}

// persisted is agent-state.json.
type persisted struct {
	Owner            string     `json:"owner"`
	Managed          []string   `json:"managed"`
	ConfirmedManaged []string   `json:"confirmed_managed"`
	LastTxnID        string     `json:"last_txn_id,omitempty"`
	PendingTxnID     string     `json:"pending_txn_id,omitempty"`
	ConfirmDeadline  *time.Time `json:"confirm_deadline,omitempty"`
	// Reverting: the pending transaction's deadline passed and the revert to the confirmed baseline
	// is owed (Desired already equals Confirmed); it is retried on every resync until it succeeds
	// (review H3).
	Reverting bool            `json:"reverting,omitempty"`
	History   []txnRecord     `json:"history,omitempty"`
	Desired   json.RawMessage `json:"desired,omitempty"`
	Confirmed json.RawMessage `json:"confirmed,omitempty"`
}

// state is the in-memory copy; the service guards it with its transaction lock.
type state struct {
	dir     string
	desired *vrxv1.DesiredState // current (possibly pending)
	confirm *vrxv1.DesiredState // confirmed baseline
	meta    persisted
}

func newState(dir, owner string) *state {
	return &state{dir: dir, desired: &vrxv1.DesiredState{}, confirm: &vrxv1.DesiredState{}, meta: persisted{Owner: owner}}
}

var docJSON = protojson.UnmarshalOptions{DiscardUnknown: true}

// loadState reads the state dir; a missing dir or file means a fresh agent.
func loadState(dir, owner string) (*state, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("state dir: %w", err)
	}
	s := newState(dir, owner)
	b, err := os.ReadFile(filepath.Join(dir, stateFile)) //nolint:gosec // fixed name in the configured state dir
	switch {
	case errors.Is(err, os.ErrNotExist):
		return s, nil
	case err != nil:
		return nil, fmt.Errorf("state: %w", err)
	}
	if err := json.Unmarshal(b, &s.meta); err != nil {
		return nil, fmt.Errorf("state: %s: %w", stateFile, err)
	}
	if s.meta.Owner != owner {
		return nil, fmt.Errorf("state: %s belongs to owner %q, this agent is %q", dir, s.meta.Owner, owner)
	}
	if len(s.meta.Desired) > 0 {
		if err := docJSON.Unmarshal(s.meta.Desired, s.desired); err != nil {
			return nil, fmt.Errorf("state: desired: %w", err)
		}
		if err := docJSON.Unmarshal(s.meta.Confirmed, s.confirm); err != nil {
			return nil, fmt.Errorf("state: confirmed: %w", err)
		}
		return s, nil
	}
	// pre-review layout: documents in desired.pb / confirmed.pb
	if s.desired, err = readPB(filepath.Join(dir, "desired.pb")); err != nil {
		return nil, err
	}
	if s.confirm, err = readPB(filepath.Join(dir, "confirmed.pb")); err != nil {
		return nil, err
	}
	return s, nil
}

func readPB(path string) (*vrxv1.DesiredState, error) {
	ds := &vrxv1.DesiredState{}
	b, err := os.ReadFile(path) //nolint:gosec // fixed names in the configured state dir
	if errors.Is(err, os.ErrNotExist) {
		return ds, nil
	}
	if err != nil {
		return nil, fmt.Errorf("state: %w", err)
	}
	if err := proto.Unmarshal(b, ds); err != nil {
		return nil, fmt.Errorf("state: %s: %w", path, err)
	}
	return ds, nil
}

// save writes the state atomically as one file, then the desired.pb mirror.
func (s *state) save() error {
	var err error
	if s.meta.Desired, err = protojson.Marshal(s.desired); err != nil {
		return err
	}
	if s.meta.Confirmed, err = protojson.Marshal(s.confirm); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.meta, "", "  ")
	if err != nil {
		return err
	}
	if saveHook != nil {
		if err := saveHook("state"); err != nil {
			return err
		}
	}
	if err := ownertable.WriteAtomic(filepath.Join(s.dir, stateFile), append(b, '\n'), 0o640); err != nil {
		return fmt.Errorf("state: %s: %w", stateFile, err)
	}
	if saveHook != nil {
		if err := saveHook("mirror"); err != nil {
			return fmt.Errorf("%w: %w", errMirror, err)
		}
	}
	pb, err := proto.MarshalOptions{Deterministic: true}.Marshal(s.desired)
	if err != nil {
		return fmt.Errorf("%w: %w", errMirror, err)
	}
	if err := ownertable.WriteAtomic(filepath.Join(s.dir, "desired.pb"), pb, 0o640); err != nil {
		return fmt.Errorf("%w: %w", errMirror, err)
	}
	_ = os.Remove(filepath.Join(s.dir, "confirmed.pb")) // pre-review layout
	return nil
}

// remember stores resp for txnID (bounded history, newest last).
func (s *state) remember(txnID, fingerprint string, resp *vrxv1.ApplyResponse) {
	b, err := protojson.Marshal(resp)
	if err != nil {
		return
	}
	for i, r := range s.meta.History {
		if r.TxnID == txnID {
			s.meta.History = append(s.meta.History[:i], s.meta.History[i+1:]...)
			break
		}
	}
	s.meta.History = append(s.meta.History, txnRecord{TxnID: txnID, Fingerprint: fingerprint, Response: b})
	if n := len(s.meta.History); n > historySize {
		s.meta.History = s.meta.History[n-historySize:]
	}
}

// recall returns the stored response of txnID.
func (s *state) recall(txnID string) (fingerprint string, resp *vrxv1.ApplyResponse, ok bool) {
	for _, r := range s.meta.History {
		if r.TxnID != txnID {
			continue
		}
		out := &vrxv1.ApplyResponse{}
		if err := protojson.Unmarshal(r.Response, out); err != nil {
			return "", nil, false
		}
		return r.Fingerprint, out, true
	}
	return "", nil, false
}

// mergeDomains returns base with every domain in domains replaced by the one in update (cleared
// when update does not carry it).
func mergeDomains(base, update *vrxv1.DesiredState, domains []string) *vrxv1.DesiredState {
	out := proto.Clone(base).(*vrxv1.DesiredState)
	if update == nil {
		update = &vrxv1.DesiredState{}
	}
	src, dst := update.ProtoReflect(), out.ProtoReflect()
	for _, d := range domains {
		fd := dst.Descriptor().Fields().ByName(protoName(d))
		if fd == nil {
			continue
		}
		if src.Has(fd) {
			dst.Set(fd, src.Get(fd))
		} else {
			dst.Clear(fd)
		}
	}
	return proto.Clone(out).(*vrxv1.DesiredState)
}

func union(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, k := range rootKeys {
		for _, l := range [][]string{a, b} {
			for _, x := range l {
				if x == k && !seen[k] {
					seen[k] = true
					out = append(out, k)
				}
			}
		}
	}
	return out
}
