// Package vpp defines the VPP binary-API client contract every descriptor is written against.
// The real implementation (a govpp connection manager with reconnect/backoff and health) is
// task P05; descriptor unit tests use internal/vpp/fake.
//
// # Messages
//
// Requests and replies are go.fd.io/govpp/api.Message values from the generated bindings in
// apps/agent/binapi (generated from the pinned VPP 26.06 .api.json by P04, owned by the
// manager). Message names and fields come only from there — never typed from memory
// (00-CONTEXT rule 6). govpp's own bundled binapi (go.fd.io/govpp/binapi/*) is generated
// from a different VPP version and must not be imported by production code.
//
// # Using the client from a descriptor (DF-*)
//
// Client is a superset of api.Connection, so the generated typed RPC clients accept it
// directly and are the preferred way to talk to VPP:
//
//	svc := interfaces.NewServiceClient(client)               // apps/agent/binapi/interface
//	rep, err := svc.CreateLoopbackInstance(ctx, &interfaces.CreateLoopbackInstance{...})
//	// err already includes a non-zero Retval, translated with api.RetvalToVPPApiError.
//
//	stream, err := svc.SwInterfaceDump(ctx, &interfaces.SwInterfaceDump{SwIfIndex: ^0})
//	for {
//		d, err := stream.Recv()                              // io.EOF after control_ping_reply
//		if errors.Is(err, io.EOF) { break }
//		...
//	}
//
// The generated dump helpers send the dump request followed by memclnt.ControlPing on one
// stream and read details until memclnt.ControlPingReply arrives. When Invoke is used
// directly, translate reply.Retval with api.RetvalToVPPApiError yourself.
//
// # Implementing the client (P05)
//
// Wrap *core.Connection (go.fd.io/govpp/core, connected through
// socketclient.NewVppClient(path) + core.AsyncConnect): it already implements Invoke,
// NewStream and WatchEvent; Connected reports the last core.ConnectionEvent state. Return
// ErrDisconnected from Invoke/NewStream while disconnected instead of blocking.
package vpp

import (
	"context"
	"errors"

	"go.fd.io/govpp/api"
)

// ErrDisconnected is returned while the VPP binary API is not connected.
var ErrDisconnected = errors.New("vpp: not connected")

// Client is the VPP binary-API client. It is a superset of api.Connection (see the
// compile-time assertion below), so generated *_rpc.ba.go service clients accept it.
// Implementations must be safe for concurrent use.
type Client interface {
	// Invoke performs one request/reply round trip, decoding the reply into reply. reply must
	// be a pointer to the reply type of req. A non-zero Retval is NOT an error here; the
	// generated service clients (or the caller) translate it with api.RetvalToVPPApiError.
	Invoke(ctx context.Context, req, reply api.Message) error
	// NewStream opens a low-level message stream for multipart requests (dumps) and other
	// sequences of messages. The caller owns the stream and must Close it; cancelling ctx
	// closes it as well.
	NewStream(ctx context.Context, opts ...api.StreamOption) (api.Stream, error)
	// WatchEvent subscribes to asynchronous event messages of the given type (for example
	// sw_interface_event after want_interface_events). Cancelling ctx closes the watcher.
	WatchEvent(ctx context.Context, event api.Message) (api.Watcher, error)
	// Connected reports whether the binary API is currently connected. It is cheap and
	// non-blocking; Health uses it.
	Connected() bool
}

// Client must stay assignable to api.Connection so generated service clients accept it.
var _ api.Connection = (Client)(nil)
