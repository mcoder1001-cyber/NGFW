// Package ipfix holds the reconciler descriptors for VPP's IPFIX exporter (vnet ipfix-export,
// task DF-8, WBS D7.6): the default exporter (exporter 0, the only one flowprobe and the classify
// reports send to), additional exporters, the classify report stream and the classify tables
// reported over IPFIX. Message names come only from apps/agent/binapi/ipfix_export;
// docs/agent/descriptors/ipfix.md is the object ↔ message table.
//
// Ownership on the shared VPP: additional exporters are owned through their collector address,
// which must be inside the collector scope given to Register (tests: 10.<slot>.0.0/16);
// classify tables through the table scope. The default exporter and the classify stream are
// VPP-global singletons managed only by the globals owner (D-071, dfkit.Globals): for it Retrieve
// reports exporter 0 while its collector is set; every other agent can only require a value.
package ipfix

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"sort"
	"strconv"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/binapi/ipfix_export"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	NameExporter        = "ipfix.exporter"
	NameDefaultExporter = "ipfix.default-exporter"
	NameClassifyStream  = "ipfix.classify-stream"
	NameClassifyTable   = "ipfix.classify-table"
)

// DefaultCollectorPort is the IPFIX port VPP uses when none is given (UDP 4739).
const DefaultCollectorPort = 4739

// NoVRF is the vrf_id meaning "no FIB bound" (VPP sends from the default route lookup).
const NoVRF = ^uint32(0)

// Exporter is an IPFIX exporter: collector address and port, source address, VRF, path MTU
// (68..1450), template interval (seconds) and UDP checksum. Addresses are canonical and of one
// family (the default exporter takes IPv4 only).
type Exporter struct {
	Collector        string `json:"collector"`
	CollectorPort    uint16 `json:"collector_port"`
	Src              string `json:"src"`
	VRF              uint32 `json:"vrf"`
	PathMTU          uint32 `json:"path_mtu"`
	TemplateInterval uint32 `json:"template_interval"`
	UDPChecksum      bool   `json:"udp_checksum"`
}

// Proto returns the canonical structpb document.
func (s Exporter) Proto() *structpb.Struct { return dfkit.Encode(s) }

// Validate checks what VPP rejects or silently rewrites (port 0, MTU out of range, interval 0,
// mixed families).
func (s Exporter) Validate() error {
	c, err := dfkit.ParseAddr(s.Collector)
	if err != nil {
		return err
	}
	src, err := dfkit.ParseAddr(s.Src)
	if err != nil {
		return err
	}
	switch {
	case c.String() != s.Collector || src.String() != s.Src:
		return dfkit.Specf("ipfix exporter: addresses must be canonical (%s, %s)", c, src)
	case c.Is4() != src.Is4():
		return dfkit.Specf("ipfix exporter: collector %s and src %s are not the same family", c, src)
	case c.IsUnspecified() || src.IsUnspecified():
		return dfkit.Specf("ipfix exporter: collector and src must be specified")
	case s.CollectorPort == 0:
		return dfkit.Specf("ipfix exporter: collector_port must be set (default %d)", DefaultCollectorPort)
	case s.PathMTU < 68 || s.PathMTU > 1450:
		return dfkit.Specf("ipfix exporter: path_mtu %d outside 68..1450", s.PathMTU)
	case s.TemplateInterval == 0:
		return dfkit.Specf("ipfix exporter: template_interval must be > 0")
	}
	return nil
}

func decodeExporter(obj proto.Message) (Exporter, netip.Addr, netip.Addr, error) {
	var s Exporter
	if err := dfkit.Decode(obj, &s); err != nil {
		return s, netip.Addr{}, netip.Addr{}, err
	}
	if err := s.Validate(); err != nil {
		return s, netip.Addr{}, netip.Addr{}, err
	}
	c, _ := dfkit.ParseAddr(s.Collector)
	src, _ := dfkit.ParseAddr(s.Src)
	return s, c, src, nil
}

func exporterFrom(c, src netip.Addr, port uint16, vrf, mtu, interval uint32, csum bool) Exporter {
	return Exporter{
		Collector: c.String(), CollectorPort: port, Src: src.String(), VRF: vrf,
		PathMTU: mtu, TemplateInterval: interval, UDPChecksum: csum,
	}
}

// Option configures the descriptors of this package.
type Option func(*options)

type options struct {
	vrfKey      dfkit.KeyFunc
	tableKey    dfkit.KeyFunc
	collectorIn func(netip.Addr) bool
	tableIn     func(uint32) bool
	globals     dfkit.Globals
}

func buildOptions(opts []Option) options {
	o := options{
		vrfKey:      dfkit.DefaultVRFKey,
		tableKey:    dfkit.DefaultClassifyTableKey,
		collectorIn: func(netip.Addr) bool { return true },
		tableIn:     func(uint32) bool { return true },
	}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithVRFKey sets the VRF key scheme of Dependencies (default "vrf/<id>").
func WithVRFKey(f dfkit.KeyFunc) Option {
	return func(o *options) {
		if f != nil {
			o.vrfKey = f
		}
	}
}

// WithClassifyTableKey sets the classify table key scheme (DF-2; default "classify-table/<id>").
func WithClassifyTableKey(f dfkit.KeyFunc) Option {
	return func(o *options) {
		if f != nil {
			o.tableKey = f
		}
	}
}

// WithCollectorScope limits the additional exporters this agent owns to collectors for which in
// returns true (default: all). Tests pass their slot's 10.<N>.0.0/16.
func WithCollectorScope(in func(netip.Addr) bool) Option {
	return func(o *options) {
		if in != nil {
			o.collectorIn = in
		}
	}
}

// WithClassifyTableScope limits the IPFIX classify tables this agent owns (default: all).
func WithClassifyTableScope(in func(uint32) bool) Option {
	return func(o *options) {
		if in != nil {
			o.tableIn = in
		}
	}
}

// WithGlobals sets the D-071 role for the VPP-global default exporter and classify stream
// (default: not the globals owner — they are then only required, never set or reset).
func WithGlobals(g dfkit.Globals) Option { return func(o *options) { o.globals = g } }

// Register constructs and registers the ipfix descriptors in dependency order.
func Register(r scheduler.Registry, client vpp.Client, opts ...Option) {
	r.Register(NewExporter(client, opts...))
	r.Register(NewClassifyTable(client, opts...))
}

// RegisterGlobals registers this package's VPP-global singleton descriptors, constructed as the
// globals owner (D-071). Call it only in the designated globals owner's agent (config
// globalsOwner: true — never a test slot on the shared host), before Register.
func RegisterGlobals(r scheduler.Registry, client vpp.Client, opts ...Option) {
	opts = append(opts, WithGlobals(dfkit.GlobalsOwner(true)))
	r.Register(NewDefaultExporter(client, opts...))
	r.Register(NewClassifyStream(client, opts...))
}

func (o options) vrfDeps(vrf uint32) []scheduler.Dependency {
	if vrf == 0 || vrf == NoVRF {
		return nil
	}
	return []scheduler.Dependency{{Key: o.vrfKey(strconv.FormatUint(uint64(vrf), 10)), Optional: true}}
}

// ---- ipfix.exporter (additional exporters) --------------------------------------------------

// ExporterMeta is the Meta of an ipfix.exporter: the stats index VPP returned on create (0 after
// a Retrieve, which has no way to learn it).
type ExporterMeta struct {
	StatIndex uint32
}

// ExporterDescriptor manages additional exporters (ipfix_exporter_create_delete): key
// ipfix.exporter/<collector>. VPP identifies them by collector address alone.
type ExporterDescriptor struct {
	client vpp.Client
	o      options
}

var _ scheduler.Descriptor = (*ExporterDescriptor)(nil)

// NewExporter returns the ipfix.exporter descriptor.
func NewExporter(client vpp.Client, opts ...Option) *ExporterDescriptor {
	return &ExporterDescriptor{client: client, o: buildOptions(opts)}
}

// Name implements scheduler.Descriptor.
func (*ExporterDescriptor) Name() string { return NameExporter }

// KeyOf implements scheduler.Descriptor.
func (*ExporterDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	var s Exporter
	if err := dfkit.Decode(obj, &s); err != nil {
		return scheduler.Join(NameExporter, "invalid")
	}
	if a, err := dfkit.ParseAddr(s.Collector); err == nil {
		return scheduler.Join(NameExporter, a.String())
	}
	return scheduler.Join(NameExporter, s.Collector)
}

// Dependencies implements scheduler.Descriptor: the VRF (optional).
func (d *ExporterDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	var s Exporter
	if err := dfkit.Decode(obj, &s); err != nil {
		return nil
	}
	return d.o.vrfDeps(s.VRF)
}

func (d *ExporterDescriptor) createDelete(ctx context.Context, obj proto.Message, create bool) (any, error) {
	s, c, src, err := decodeExporter(obj)
	if err != nil {
		return nil, err
	}
	if !d.o.collectorIn(c) {
		return nil, dfkit.Specf("ipfix exporter: collector %s is outside this agent's scope", c)
	}
	rep, err := ipfix_export.NewServiceClient(d.client).IpfixExporterCreateDelete(ctx, &ipfix_export.IpfixExporterCreateDelete{
		IsCreate: create, CollectorAddress: dfkit.ToAPIAddress(c), CollectorPort: s.CollectorPort,
		SrcAddress: dfkit.ToAPIAddress(src), VrfID: s.VRF, PathMtu: s.PathMTU,
		TemplateInterval: s.TemplateInterval, UDPChecksum: s.UDPChecksum,
	})
	if err != nil {
		return nil, fmt.Errorf("ipfix_exporter_create_delete(create=%t, %s): %w", create, c, err)
	}
	return ExporterMeta{StatIndex: rep.StatIndex}, nil
}

// Create implements scheduler.Descriptor. VPP updates an exporter with the same collector in
// place, so a re-apply is idempotent.
func (d *ExporterDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	return d.createDelete(ctx, obj, true)
}

// Update implements scheduler.Descriptor: create_delete(is_create=1) on an existing collector
// reconfigures it in place.
func (d *ExporterDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.createDelete(ctx, newObj, true)
}

// Delete implements scheduler.Descriptor. It first checks that the exporter still exists (D-074:
// VPP would pool_put exporter 0 for a collector that matches it); NO_SUCH_ENTRY counts as deleted.
func (d *ExporterDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	kvs, err := d.Retrieve(ctx)
	if err != nil {
		return err
	}
	found := false
	for _, kv := range kvs {
		found = found || kv.Key == d.KeyOf(obj)
	}
	if !found {
		return nil
	}
	_, err = d.createDelete(ctx, obj, false)
	if dfkit.IsVPPError(err, api.NO_SUCH_ENTRY) {
		return nil
	}
	return err
}

// allExporters reads every exporter (ipfix_all_exporter_get, cursor-paged); index 0 first.
func allExporters(ctx context.Context, c vpp.Client) ([]*ipfix_export.IpfixAllExporterDetails, error) {
	svc := ipfix_export.NewServiceClient(c)
	var out []*ipfix_export.IpfixAllExporterDetails
	cursor := uint32(0)
	for {
		stream, err := svc.IpfixAllExporterGet(ctx, &ipfix_export.IpfixAllExporterGet{Cursor: cursor})
		if err != nil {
			return nil, fmt.Errorf("ipfix_all_exporter_get: %w", err)
		}
		var last *ipfix_export.IpfixAllExporterGetReply
		for {
			det, rep, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				last = rep
				break
			}
			if err != nil && rep != nil && dfkit.IsVPPError(err, api.EAGAIN) {
				last = rep
				break
			}
			if err != nil {
				return nil, fmt.Errorf("ipfix_all_exporter_get: %w", err)
			}
			out = append(out, det)
		}
		if last == nil || last.Retval == 0 || last.Cursor == cursor {
			return out, nil
		}
		cursor = last.Cursor
	}
}

// Retrieve implements scheduler.Descriptor: ipfix_all_exporter_get without exporter 0 (the
// default exporter, the first entry), collectors in scope.
func (d *ExporterDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	all, err := allExporters(ctx, d.client)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for i, e := range all {
		if i == 0 {
			continue
		}
		c := dfkit.FromAPIAddress(e.CollectorAddress)
		if !c.IsValid() || c.IsUnspecified() || !d.o.collectorIn(c) {
			continue
		}
		s := exporterFrom(c, dfkit.FromAPIAddress(e.SrcAddress), e.CollectorPort, e.VrfID, e.PathMtu, e.TemplateInterval, e.UDPChecksum)
		out = append(out, scheduler.KV{Key: scheduler.Join(NameExporter, s.Collector), Value: s.Proto(), Meta: ExporterMeta{}})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return dfkit.Dedupe(out), nil
}

// ---- ipfix.default-exporter (exporter 0) ----------------------------------------------------

// DefaultExporterID is the object id of the singleton (key ipfix.default-exporter/global).
const DefaultExporterID = "global"

// KeyDefaultExporter is the key of the singleton.
var KeyDefaultExporter = scheduler.Join(NameDefaultExporter, DefaultExporterID)

// DefaultExporterDescriptor manages exporter 0 (set_ipfix_exporter / ipfix_exporter_dump): the
// exporter flowprobe and the classify reports use. IPv4 collector only (VPP decodes exporter 0's
// collector as IPv4). Retrieve reports it while its collector is set; Delete sets the collector
// to 0.0.0.0, which is how VPP disables it (exporter 0 itself cannot be deleted).
type DefaultExporterDescriptor struct {
	client vpp.Client
	o      options
}

var _ scheduler.Descriptor = (*DefaultExporterDescriptor)(nil)

// NewDefaultExporter returns the ipfix.default-exporter descriptor.
func NewDefaultExporter(client vpp.Client, opts ...Option) *DefaultExporterDescriptor {
	return &DefaultExporterDescriptor{client: client, o: buildOptions(opts)}
}

// Name implements scheduler.Descriptor.
func (*DefaultExporterDescriptor) Name() string { return NameDefaultExporter }

// KeyOf implements scheduler.Descriptor.
func (*DefaultExporterDescriptor) KeyOf(proto.Message) scheduler.Key { return KeyDefaultExporter }

// Dependencies implements scheduler.Descriptor: the VRF (optional).
func (d *DefaultExporterDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	var s Exporter
	if err := dfkit.Decode(obj, &s); err != nil {
		return nil
	}
	return d.o.vrfDeps(s.VRF)
}

func (d *DefaultExporterDescriptor) set(ctx context.Context, obj proto.Message) error {
	s, c, src, err := decodeExporter(obj)
	if err != nil {
		return err
	}
	if !c.Is4() {
		return dfkit.Specf("ipfix default exporter: collector %s must be IPv4", c)
	}
	if !d.o.globals.Owner() {
		return d.o.globals.Require(ctx, NameDefaultExporter, s.Proto(), func(ctx context.Context) (proto.Message, bool, error) {
			cur, ok, err := d.Current(ctx)
			return cur.Proto(), ok, err
		})
	}
	_, err = ipfix_export.NewServiceClient(d.client).SetIpfixExporter(ctx, &ipfix_export.SetIpfixExporter{
		CollectorAddress: dfkit.ToAPIAddress(c), CollectorPort: s.CollectorPort, SrcAddress: dfkit.ToAPIAddress(src),
		VrfID: s.VRF, PathMtu: s.PathMTU, TemplateInterval: s.TemplateInterval, UDPChecksum: s.UDPChecksum,
	})
	if err != nil {
		return fmt.Errorf("set_ipfix_exporter(%s): %w", c, err)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *DefaultExporterDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	return nil, d.set(ctx, obj)
}

// Update implements scheduler.Descriptor (in place).
func (d *DefaultExporterDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return nil, d.set(ctx, newObj)
}

// Delete implements scheduler.Descriptor: collector and src 0.0.0.0 disable exporter 0 (globals
// owner only; a no-op for everyone else).
func (d *DefaultExporterDescriptor) Delete(ctx context.Context, _ proto.Message, _ any) error {
	if !d.o.globals.Owner() {
		return nil
	}
	zero := dfkit.ToAPIAddress(netip.IPv4Unspecified())
	_, err := ipfix_export.NewServiceClient(d.client).SetIpfixExporter(ctx, &ipfix_export.SetIpfixExporter{
		CollectorAddress: zero, CollectorPort: DefaultCollectorPort, SrcAddress: zero, VrfID: NoVRF,
		PathMtu: 512, TemplateInterval: 20,
	})
	if err != nil {
		return fmt.Errorf("set_ipfix_exporter(disable): %w", err)
	}
	return nil
}

// Current reads exporter 0 as VPP has it (ok=false when its collector is unset).
func (d *DefaultExporterDescriptor) Current(ctx context.Context) (Exporter, bool, error) {
	stream, err := ipfix_export.NewServiceClient(d.client).IpfixExporterDump(ctx, &ipfix_export.IpfixExporterDump{})
	if err != nil {
		return Exporter{}, false, fmt.Errorf("ipfix_exporter_dump: %w", err)
	}
	details, err := dfkit.Drain(stream, stream.Recv)
	if err != nil {
		return Exporter{}, false, fmt.Errorf("ipfix_exporter_dump: %w", err)
	}
	if len(details) == 0 {
		return Exporter{}, false, nil
	}
	e := details[0]
	c := dfkit.FromAPIAddress(e.CollectorAddress)
	if !c.IsValid() || c.IsUnspecified() {
		return Exporter{}, false, nil
	}
	return exporterFrom(c, dfkit.FromAPIAddress(e.SrcAddress), e.CollectorPort, e.VrfID, e.PathMtu, e.TemplateInterval, e.UDPChecksum), true, nil
}

// Retrieve implements scheduler.Descriptor: exporter 0 while its collector is set (for a
// non-owner: write-only requirement, see dfkit.Globals).
func (d *DefaultExporterDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	if !d.o.globals.Owner() {
		return d.o.globals.NonOwnerRetrieve(NameDefaultExporter)
	}
	s, ok, err := d.Current(ctx)
	if err != nil || !ok {
		return nil, err
	}
	return []scheduler.KV{{Key: KeyDefaultExporter, Value: s.Proto()}}, nil
}
