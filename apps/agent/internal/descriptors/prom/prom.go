// Package prom holds the reconciler descriptor for VPP's static HTTP server (http_static plugin,
// task DF-8, WBS D8.2/D8.3), the listener VPP's Prometheus exporter (prom plugin) serves its
// /stats.prom page from. Message names come only from apps/agent/binapi/http_static;
// docs/agent/descriptors/prom.md is the object ↔ message table.
//
// What VPP 26.06 cannot do through its binary API (so there is no descriptor for it):
//   - the prom exporter itself: the prom plugin has no .api file; it is enabled and tuned only by
//     the CLI ("prom enable", "prom stat-patterns", "prom min-scrape-interval", "prom used-only")
//     or startup.conf. Shelling out to the VPP CLI is forbidden (00-CONTEXT rule 1/9), so
//     "prom-exporter" is not API-configurable here — see DF-8-questions.md.
//   - disabling or reconfiguring http_static: there is only http_static_enable_v4/v5, which VPP
//     accepts once per process (APP_ALREADY_ATTACHED afterwards) and which also enables the
//     session layer. Update therefore returns ErrNotSupported and Delete is a no-op; both need a
//     VPP restart to take effect.
//   - reading the server's state: no dump or getter, so Retrieve returns ErrRetrieveUnsupported
//     (write-only, D-063).
package prom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"sync"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/binapi/http_static"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// NameHTTPStaticServer is the descriptor name.
const NameHTTPStaticServer = "prom.http-static-server"

// Plugin is the plugin name used in ErrPluginNotLoaded.
const Plugin = "http_static"

// ErrServerBusy means http_static is already enabled in this VPP process by someone else (or with
// other parameters) — VPP accepts one enable per process.
var ErrServerBusy = errors.New("http_static is already enabled in this VPP process")

// HTTPStaticServer is the prom.http-static-server singleton (http_static_enable_v5): the listen
// URI ("tcp://<ip>/<port>"), the www root (absolute path), FIFO and cache sizes, cache max age
// (s), keep-alive timeout (s), request body and rx buffer limits, preallocated FIFOs and the
// private segment size. Zero sizes mean VPP's defaults.
type HTTPStaticServer struct {
	URI                string `json:"uri"`
	WWWRoot            string `json:"www_root"`
	FifoSize           uint32 `json:"fifo_size"`
	CacheSizeLimit     uint32 `json:"cache_size_limit"`
	MaxAge             uint32 `json:"max_age"`
	KeepaliveTimeout   uint32 `json:"keepalive_timeout"`
	MaxBodySize        uint32 `json:"max_body_size"`
	RxBuffThresh       uint32 `json:"rx_buff_thresh"`
	PreallocFifos      uint32 `json:"prealloc_fifos"`
	PrivateSegmentSize uint32 `json:"private_segment_size"`
}

// Proto returns the canonical structpb document.
func (s HTTPStaticServer) Proto() *structpb.Struct { return dfkit.Encode(s) }

// Validate checks the URI ("tcp://<ip>/<port>" or "tls://…") and the www root (absolute, no "..",
// printable path characters only, 2..255 bytes).
func (s HTTPStaticServer) Validate() error {
	proto, rest, ok := strings.Cut(s.URI, "://")
	if !ok || (proto != "tcp" && proto != "tls") {
		return dfkit.Specf("http_static: uri %q must be tcp://<ip>/<port> or tls://<ip>/<port>", s.URI)
	}
	host, port, ok := strings.Cut(rest, "/")
	if !ok {
		return dfkit.Specf("http_static: uri %q has no /<port>", s.URI)
	}
	if a, err := netip.ParseAddr(host); err != nil || a.String() != host {
		return dfkit.Specf("http_static: uri %q: %q is not a canonical IP address", s.URI, host)
	}
	if p, err := strconv.ParseUint(port, 10, 16); err != nil || p == 0 || strconv.FormatUint(p, 10) != port {
		return dfkit.Specf("http_static: uri %q: bad port %q", s.URI, port)
	}
	if len(s.WWWRoot) < 2 || len(s.WWWRoot) > 255 || !strings.HasPrefix(s.WWWRoot, "/") || strings.Contains(s.WWWRoot, "..") {
		return dfkit.Specf("http_static: www_root %q must be an absolute path without \"..\" (2..255 bytes)", s.WWWRoot)
	}
	for _, r := range s.WWWRoot {
		ok := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || strings.ContainsRune("/-_.", r)
		if !ok {
			return dfkit.Specf("http_static: www_root %q: invalid character %q", s.WWWRoot, r)
		}
	}
	return nil
}

// RegisterGlobals registers this package's VPP-global singleton descriptors, constructed as the
// globals owner (D-071). Call it only in the designated globals owner's agent (config
// globalsOwner: true — never a test slot on the shared host).
func RegisterGlobals(r scheduler.Registry, client vpp.Client, owner string) {
	r.Register(NewHTTPStaticServer(client, owner, dfkit.GlobalsOwner(true)))
}

// HTTPStaticServerID is the object id of the singleton (key prom.http-static-server/global).
const HTTPStaticServerID = "global"

// KeyHTTPStaticServer is the key of the singleton.
var KeyHTTPStaticServer = scheduler.Join(NameHTTPStaticServer, HTTPStaticServerID)

// HTTPStaticServerDescriptor manages the prom.http-static-server singleton (see package doc).
type HTTPStaticServerDescriptor struct {
	client  vpp.Client
	owner   string
	globals dfkit.Globals
	mu      sync.Mutex
}

var _ scheduler.Descriptor = (*HTTPStaticServerDescriptor)(nil)

// NewHTTPStaticServer returns the prom.http-static-server descriptor.
func NewHTTPStaticServer(client vpp.Client, owner string, g dfkit.Globals) *HTTPStaticServerDescriptor {
	return &HTTPStaticServerDescriptor{client: client, owner: owner, globals: g}
}

// Name implements scheduler.Descriptor.
func (*HTTPStaticServerDescriptor) Name() string { return NameHTTPStaticServer }

// KeyOf implements scheduler.Descriptor.
func (*HTTPStaticServerDescriptor) KeyOf(proto.Message) scheduler.Key { return KeyHTTPStaticServer }

// Dependencies implements scheduler.Descriptor: none.
func (*HTTPStaticServerDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// Create implements scheduler.Descriptor: http_static_enable_v5. VPP accepts one enable per
// process (APP_ALREADY_ATTACHED afterwards), so the applied value is recorded in the owner's
// BootStore under the VPP boot identity and a re-apply on the same VPP process is skipped (D-076);
// any other attached server is ErrServerBusy.
func (d *HTTPStaticServerDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	var s HTTPStaticServer
	if err := dfkit.Decode(obj, &s); err != nil {
		return nil, err
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if !d.globals.Owner() {
		return nil, d.globals.Require(ctx, NameHTTPStaticServer, obj, nil)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	value, _ := json.Marshal(s)
	applied, id, err := dfkit.AppliedThisBoot(ctx, d.client, d.owner, KeyHTTPStaticServer, string(value))
	if err != nil {
		return nil, err
	}
	if applied {
		return nil, nil
	}
	req := &http_static.HTTPStaticEnableV5{
		FifoSize: s.FifoSize, CacheSizeLimit: s.CacheSizeLimit, MaxAge: s.MaxAge, KeepaliveTimeout: s.KeepaliveTimeout,
		MaxBodySize: uint64(s.MaxBodySize), RxBuffThresh: s.RxBuffThresh, PreallocFifos: s.PreallocFifos,
		PrivateSegmentSize: s.PrivateSegmentSize, WwwRoot: s.WWWRoot, URI: s.URI,
	}
	_, err = http_static.NewServiceClient(d.client).HTTPStaticEnableV5(ctx, req)
	if dfkit.IsVPPError(err, api.APP_ALREADY_ATTACHED) {
		return nil, fmt.Errorf("http_static_enable_v5: %w (%v)", ErrServerBusy, err)
	}
	if err != nil {
		return nil, fmt.Errorf("http_static_enable_v5(%s): %w", s.URI, dfkit.PluginError(Plugin, err))
	}
	return nil, dfkit.Record(d.owner, KeyHTTPStaticServer, id, string(value))
}

// Update implements scheduler.Descriptor: VPP cannot reconfigure a running http_static server.
func (*HTTPStaticServerDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, fmt.Errorf("%s: %w: http_static cannot be reconfigured; restart VPP", NameHTTPStaticServer, dfkit.ErrNotSupported)
}

// Delete implements scheduler.Descriptor: a no-op — VPP has no http_static disable; the server
// runs until VPP restarts.
func (d *HTTPStaticServerDescriptor) Delete(context.Context, proto.Message, any) error {
	return nil
}

// Retrieve implements scheduler.Descriptor: no getter (write-only, D-063).
func (*HTTPStaticServerDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, dfkit.RetrieveUnsupported(NameHTTPStaticServer)
}
