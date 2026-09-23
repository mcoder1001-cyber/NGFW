package ipfix

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/ipfix_export"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// ClassifyStream is the ipfix.classify-stream singleton (set_ipfix_classify_stream): the
// observation domain and UDP source port of the classify-table reports. VPP's default is
// "unset" (src_port 0); the tables can only be added once it is set.
type ClassifyStream struct {
	DomainID uint32 `json:"domain_id"`
	SrcPort  uint16 `json:"src_port"`
}

// Proto returns the canonical structpb document.
func (s ClassifyStream) Proto() *structpb.Struct { return dfkit.Encode(s) }

// ClassifyTable is one classify table reported over IPFIX (ipfix_classify_table_add_del): the
// VPP classify table index (DF-2's table), the IP version of its records and the transport
// protocol whose ports are reported (0 = none).
type ClassifyTable struct {
	Table     uint32 `json:"table"`
	IPVersion string `json:"ip_version"` // "ip4" | "ip6"
	Protocol  uint8  `json:"protocol"`
}

// Proto returns the canonical structpb document.
func (s ClassifyTable) Proto() *structpb.Struct { return dfkit.Encode(s) }

// ---- ipfix.classify-stream ------------------------------------------------------------------

// ClassifyStreamID is the object id of the singleton (key ipfix.classify-stream/global).
const ClassifyStreamID = "global"

// KeyClassifyStream is the key of the singleton.
var KeyClassifyStream = scheduler.Join(NameClassifyStream, ClassifyStreamID)

// ClassifyStreamDescriptor manages the ipfix.classify-stream singleton. Retrieve reports it while
// src_port is set; Delete resets it to domain 0 / port 0 ("unset").
type ClassifyStreamDescriptor struct{ client vpp.Client }

var _ scheduler.Descriptor = (*ClassifyStreamDescriptor)(nil)

// NewClassifyStream returns the ipfix.classify-stream descriptor.
func NewClassifyStream(client vpp.Client) *ClassifyStreamDescriptor {
	return &ClassifyStreamDescriptor{client: client}
}

// Name implements scheduler.Descriptor.
func (*ClassifyStreamDescriptor) Name() string { return NameClassifyStream }

// KeyOf implements scheduler.Descriptor.
func (*ClassifyStreamDescriptor) KeyOf(proto.Message) scheduler.Key { return KeyClassifyStream }

// Dependencies implements scheduler.Descriptor: the default exporter (the reports go to exporter
// 0; optional, the stream can be set before the collector).
func (*ClassifyStreamDescriptor) Dependencies(proto.Message) []scheduler.Dependency {
	return []scheduler.Dependency{{Key: KeyDefaultExporter, Optional: true}}
}

func (d *ClassifyStreamDescriptor) set(ctx context.Context, s ClassifyStream) error {
	_, err := ipfix_export.NewServiceClient(d.client).SetIpfixClassifyStream(ctx,
		&ipfix_export.SetIpfixClassifyStream{DomainID: s.DomainID, SrcPort: s.SrcPort})
	if err != nil {
		return fmt.Errorf("set_ipfix_classify_stream(%d, %d): %w", s.DomainID, s.SrcPort, err)
	}
	return nil
}

func (d *ClassifyStreamDescriptor) apply(ctx context.Context, obj proto.Message) error {
	var s ClassifyStream
	if err := dfkit.Decode(obj, &s); err != nil {
		return err
	}
	if s.SrcPort == 0 {
		return dfkit.Specf("ipfix classify stream: src_port must be set (0 means unset)")
	}
	return d.set(ctx, s)
}

// Create implements scheduler.Descriptor.
func (d *ClassifyStreamDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	return nil, d.apply(ctx, obj)
}

// Update implements scheduler.Descriptor (in place; VPP moves the existing reports).
func (d *ClassifyStreamDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return nil, d.apply(ctx, newObj)
}

// Delete implements scheduler.Descriptor: back to the unset state.
func (d *ClassifyStreamDescriptor) Delete(ctx context.Context, _ proto.Message, _ any) error {
	return d.set(ctx, ClassifyStream{})
}

// ErrClassifyDumpBroken explains why the classify descriptors are write-only: VPP 26.06 sends
// ipfix_classify_stream_details and ipfix_classify_table_details with the bare message id
// (flow_api.c: ntohs (VL_API_IPFIX_CLASSIFY_*_DETAILS) without REPLY_MSG_ID_BASE), so the client
// receives them as an unrelated message (host run 2026-09-24: "No subscription found for the
// notification message … msgId=12") and the dump returns nothing. Recorded for docs/vpp-code-track.md
// in DF-8-questions.md. The dumps are not sent at all, so no stray message reaches govpp.
var ErrClassifyDumpBroken = fmt.Errorf("%w: ipfix_classify_*_details carry a wrong message id in VPP 26.06", dfkit.ErrRetrieveUnsupported)

// Retrieve implements scheduler.Descriptor: write-only (D-063), see ErrClassifyDumpBroken.
func (d *ClassifyStreamDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, fmt.Errorf("%s: %w", NameClassifyStream, ErrClassifyDumpBroken)
}

// ---- ipfix.classify-table -------------------------------------------------------------------

// ClassifyTableDescriptor manages ipfix.classify-table objects: key ipfix.classify-table/<table>.
type ClassifyTableDescriptor struct {
	client vpp.Client
	o      options
}

var _ scheduler.Descriptor = (*ClassifyTableDescriptor)(nil)

// NewClassifyTable returns the ipfix.classify-table descriptor.
func NewClassifyTable(client vpp.Client, opts ...Option) *ClassifyTableDescriptor {
	return &ClassifyTableDescriptor{client: client, o: buildOptions(opts)}
}

// Name implements scheduler.Descriptor.
func (*ClassifyTableDescriptor) Name() string { return NameClassifyTable }

func tableID(t uint32) string { return strconv.FormatUint(uint64(t), 10) }

// KeyOf implements scheduler.Descriptor.
func (*ClassifyTableDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	var s ClassifyTable
	if err := dfkit.Decode(obj, &s); err != nil {
		return scheduler.Join(NameClassifyTable, "invalid")
	}
	return scheduler.Join(NameClassifyTable, tableID(s.Table))
}

// Dependencies implements scheduler.Descriptor: the classify stream (VPP refuses tables before
// it is set) and the classify table itself (DF-2 key, optional until DF-2's key scheme is merged).
func (d *ClassifyTableDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	var s ClassifyTable
	if err := dfkit.Decode(obj, &s); err != nil {
		return nil
	}
	return []scheduler.Dependency{
		{Key: KeyClassifyStream},
		{Key: d.o.tableKey(tableID(s.Table)), Optional: true},
	}
}

func (d *ClassifyTableDescriptor) addDel(ctx context.Context, obj proto.Message, add bool) error {
	var s ClassifyTable
	if err := dfkit.Decode(obj, &s); err != nil {
		return err
	}
	af := ip_types.ADDRESS_IP4
	switch s.IPVersion {
	case "ip4":
	case "ip6":
		af = ip_types.ADDRESS_IP6
	default:
		return dfkit.Specf("ipfix classify table: ip_version %q must be ip4 or ip6", s.IPVersion)
	}
	if !d.o.tableIn(s.Table) {
		return dfkit.Specf("ipfix classify table %d is outside this agent's scope", s.Table)
	}
	_, err := ipfix_export.NewServiceClient(d.client).IpfixClassifyTableAddDel(ctx, &ipfix_export.IpfixClassifyTableAddDel{
		TableID: s.Table, IPVersion: af, TransportProtocol: ip_types.IPProto(s.Protocol), IsAdd: add,
	})
	if err != nil {
		return fmt.Errorf("ipfix_classify_table_add_del(%d, add=%t): %w", s.Table, add, err)
	}
	return nil
}

// Create implements scheduler.Descriptor; VALUE_EXIST (already reported) is success.
func (d *ClassifyTableDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	err := d.addDel(ctx, obj, true)
	if dfkit.IsVPPError(err, api.VALUE_EXIST) {
		return nil, nil
	}
	return nil, err
}

// Update implements scheduler.Descriptor: version/protocol are fixed per report — recreate.
func (d *ClassifyTableDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor; NO_SUCH_ENTRY counts as deleted.
func (d *ClassifyTableDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	err := d.addDel(ctx, obj, false)
	if dfkit.IsVPPError(err, api.NO_SUCH_ENTRY) {
		return nil
	}
	return err
}

// Retrieve implements scheduler.Descriptor: write-only (D-063), see ErrClassifyDumpBroken.
// DecodeClassifyTables is the decoder to switch to once VPP sends the details correctly.
func (d *ClassifyTableDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, fmt.Errorf("%s: %w", NameClassifyTable, ErrClassifyDumpBroken)
}

// DecodeClassifyTables turns ipfix_classify_table_details into KVs (tables in scope, sorted):
// the Retrieve body once the VPP message-id bug is fixed; unit-tested now.
func (d *ClassifyTableDescriptor) DecodeClassifyTables(details []*ipfix_export.IpfixClassifyTableDetails) []scheduler.KV {
	var out []scheduler.KV
	for _, t := range details {
		if !d.o.tableIn(t.TableID) {
			continue
		}
		s := ClassifyTable{Table: t.TableID, IPVersion: "ip4", Protocol: uint8(t.TransportProtocol)}
		if t.IPVersion == ip_types.ADDRESS_IP6 {
			s.IPVersion = "ip6"
		}
		out = append(out, scheduler.KV{Key: scheduler.Join(NameClassifyTable, tableID(s.Table)), Value: s.Proto()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
