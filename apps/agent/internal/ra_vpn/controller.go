package ravpn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers/strongswan"
)

// EngineName names the private remote-access engine descriptor.
const EngineName = "remote-access.engine"

// MaxEngineSpecBytes accommodates all 1024 bounded EAP users and immutable refs.
const MaxEngineSpecBytes = 1 << 20

// ErrEngine is a bounded public error that never exposes credential or daemon details.
var ErrEngine = errors.New("remote-access: verified engine lifecycle unavailable")

// EngineSpec is the scheduler contract. It contains references and keyed
// fingerprints only; decrypted credentials never enter scheduler state.
type EngineSpec struct {
	ServerCertificate *ngfwv1.PkiCertificate      `json:"serverCertificate"`
	ClientCA          *ngfwv1.PkiCa               `json:"clientCA,omitempty"`
	Owner             string                      `json:"owner"`
	Profile           string                      `json:"profile"`
	Instance          string                      `json:"instance"`
	Configuration     *ngfwv1.RemoteAccessProfile `json:"configuration"`
	Proposal          *ngfwv1.IpsecProposal       `json:"proposal"`
	OuterID           uint32                      `json:"outerId"`
	InnerID           uint32                      `json:"innerId"`
	OuterTable        uint32                      `json:"outerTable"`
	InnerTable        uint32                      `json:"innerTable"`
	Fingerprints      map[string]string           `json:"fingerprints"`
}

// Validate checks the material-free instance, policy and immutable fingerprint contract.
func (s EngineSpec) Validate() error {
	if !safeOwnerName(s.Owner) || !safeOwnerName(s.Profile) || s.Instance != InstanceID(s.Owner, s.Profile) || s.Configuration == nil || s.Proposal == nil || s.OuterID == s.InnerID || s.OuterID > 8191 || s.InnerID > 8191 {
		return ErrEngine
	}
	if _, err := BuildNetworkPlan(s.Owner, s.Profile, s.Configuration); err != nil {
		return ErrEngine
	}
	for _, p := range []*ngfwv1.RemoteAccessPolicy{s.Configuration.GetOuterPolicy(), s.Configuration.GetAccessPolicy()} {
		if p == nil || len(p.GetIngress()) == 0 || len(p.GetEgress()) == 0 {
			return ErrEngine
		}
	}
	if len(s.Fingerprints) > 1040 {
		return ErrEngine
	}
	for ref, digest := range s.Fingerprints {
		if ref == "" || len(ref) > 256 || !strings.HasPrefix(digest, "hmac:") || !ValidInstance(strings.TrimPrefix(digest, "hmac:")) {
			return ErrEngine
		}
	}
	return nil
}

// Proto serializes only public references and authenticated fingerprints.
func (s EngineSpec) Proto() (*structpb.Struct, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	// An omitted enabled field has the same activation semantics as true.
	s.Configuration = proto.Clone(s.Configuration).(*ngfwv1.RemoteAccessProfile)
	enabled := true
	if s.Configuration.Enabled == nil {
		s.Configuration.Enabled = &enabled
	}
	data, err := json.Marshal(s)
	if err != nil || len(data) > MaxEngineSpecBytes {
		return nil, ErrEngine
	}
	var value map[string]any
	if json.Unmarshal(data, &value) != nil {
		return nil, ErrEngine
	}
	return structpb.NewStruct(value)
}

// DecodeEngine decodes a bounded material-free engine specification.
func DecodeEngine(value *structpb.Struct) (EngineSpec, error) {
	var s EngineSpec
	if value == nil {
		return s, ErrEngine
	}
	data, err := json.Marshal(value.AsMap())
	if err != nil {
		return s, ErrEngine
	}
	if len(data) > MaxEngineSpecBytes || json.Unmarshal(data, &s) != nil || s.Validate() != nil {
		return EngineSpec{}, ErrEngine
	}
	return s, nil
}

// SnapshotPreparation is supplied by the sealed-cache/PKI owner. Prepare runs
// only after transport/VRF/routes/both ACL directions have been read back.
type SnapshotPreparation interface {
	Prepare(context.Context, EngineSpec) (*PreparedEngine, error)
}

// PreparedEngine keeps daemon loading and snapshot cleanup behind private callbacks.
type PreparedEngine struct {
	ConnectionName string
	// Load closes over private bytes, never exported through generic formatting.
	Load func(context.Context, strongswan.ViciConn) error
	// Unload removes only this generation's owned connection and pools and verifies empty daemon readback.
	Unload  func(context.Context, strongswan.ViciConn) error
	Cleanup func(context.Context) error
}

func (*PreparedEngine) String() string { return "remote-access prepared generation <redacted>" }

// GoString redacts private preparation callbacks in diagnostic formatting.
func (p *PreparedEngine) GoString() string { return p.String() }
func (s EngineSpec) String() string        { return fmt.Sprintf("remote-access engine %s", s.Instance) }

// EngineReadiness verifies actual installed components without starting any
// daemon. Readiness is distinct from the existence of an active profile (D236).
type EngineReadiness interface{ Preflight(context.Context) error }

// SnapshotValidation verifies immutable credentials and rendered syntax for
// DryRun before any namespace, VPP, unit or snapshot mutation.
type SnapshotValidation interface {
	Validate(context.Context, EngineSpec) error
}

// NamespacePlanInventory reads protected full-instance plans independently of
// daemon observation receipts. The trusted construction seam exists for owned
// private fixtures; profile/API data never supplies a root, path or inventory.
// Errors and unknown entries must fail closed; returned plans remain subject to
// owner validation and actual fixed-unit positive quiescence checks.
type NamespacePlanInventory func(context.Context, string) ([]*NetworkPlan, error)
