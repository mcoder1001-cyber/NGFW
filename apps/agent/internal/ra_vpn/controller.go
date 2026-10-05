package ravpn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/protobuf/types/known/structpb"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers/strongswan"
)

const EngineName = "remote-access.engine"

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

func (s EngineSpec) Validate() error {
	if !safeOwnerName(s.Owner) || !safeOwnerName(s.Profile) || s.Instance != InstanceID(s.Owner, s.Profile) || s.Configuration == nil || s.Proposal == nil || s.OuterID == s.InnerID || s.OuterID > 8192 || s.InnerID > 8192 {
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
func (s EngineSpec) Proto() (*structpb.Struct, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return nil, ErrEngine
	}
	var value map[string]any
	if json.Unmarshal(data, &value) != nil {
		return nil, ErrEngine
	}
	return structpb.NewStruct(value)
}
func DecodeEngine(value *structpb.Struct) (EngineSpec, error) {
	var s EngineSpec
	if value == nil {
		return s, ErrEngine
	}
	data, err := json.Marshal(value.AsMap())
	if err != nil {
		return s, ErrEngine
	}
	if len(data) > 262144 || json.Unmarshal(data, &s) != nil || s.Validate() != nil {
		return EngineSpec{}, ErrEngine
	}
	return s, nil
}

// SnapshotPreparation is supplied by the sealed-cache/PKI owner. Prepare runs
// only after transport/VRF/routes/both ACL directions have been read back.
type SnapshotPreparation interface {
	Prepare(context.Context, EngineSpec) (*PreparedEngine, error)
}
type PreparedEngine struct {
	ConnectionName string
	// Load closes over private bytes, never exported through generic formatting.
	Load    func(context.Context, strongswan.ViciConn) error
	Cleanup func(context.Context) error
}

func (*PreparedEngine) String() string     { return "remote-access prepared generation <redacted>" }
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
