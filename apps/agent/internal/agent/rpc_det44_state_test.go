package agent

import (
	"context"
	"io"
	"net/netip"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	cnatapi "ngfw/agent/binapi/cnat"
	"ngfw/agent/binapi/det44"
	"ngfw/agent/binapi/ip_types"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
)

// F-det44-map-dslite-cnat: DET44 sessions / lookup / close and CNAT sessions / purge over the gRPC server on the fake
// VPP. The det44 map 10.7.1.0/24 → 10.7.2.200/30 has a sharing ratio of 64 and 1008 ports per host (VPP's formulas).

func det44Seed(v *coretest.VPP) netip.Addr {
	d := v.Det44()
	d.Lock()
	defer d.Unlock()
	d.Enabled = true
	d.Maps = append(d.Maps, &det44.Det44MapDetails{InAddr: [4]byte{10, 7, 1, 0}, InPlen: 24, OutAddr: [4]byte{10, 7, 2, 200}, OutPlen: 30,
		SharingRatio: 64, PortsPerHost: 1008})
	user := netip.MustParseAddr("10.7.1.5")
	for i := uint16(0); i < 3; i++ {
		d.Sessions[user] = append(d.Sessions[user], det44.Det44SessionDetails{InPort: 40000 + i, OutPort: 6064 + i,
			ExtAddr: [4]byte{10, 7, 2, 2}, ExtPort: 80, State: 3, Expire: 100})
	}
	return user
}

func actionRun(t *testing.T, c ngfwv1.DataplaneClient, req *ngfwv1.ActionRequest) ([]*ngfwv1.ActionOutput, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	st, err := c.Action(ctx, req)
	if err != nil {
		return nil, err
	}
	var out []*ngfwv1.ActionOutput
	for {
		o, err := st.Recv()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out = append(out, o)
	}
}

func closeReq(dir, addr string, port uint32) *ngfwv1.ActionRequest {
	return &ngfwv1.ActionRequest{Action: &ngfwv1.ActionRequest_Det44SessionClose{Det44SessionClose: &ngfwv1.Det44SessionCloseAction{
		Direction: dir, Address: addr, Port: port, ExternalAddress: "10.7.2.2", ExternalPort: 80}}}
}

func TestDet44StateOnFake(t *testing.T) {
	v := coretest.New()
	det44Seed(v)
	s := newSvc(t, v, t.TempDir())
	c := natServer(t, s)
	ctx := context.Background()

	r, err := c.Det44Sessions(ctx, &ngfwv1.Det44SessionsRequest{User: "10.7.1.5", Limit: 2})
	if err != nil || len(r.GetSessions()) != 2 || r.GetNextOffset() != 2 || r.GetTotalSessions() != 3 ||
		r.GetOutsideAddress() != "10.7.2.200" || r.GetPortLo() != 6064 || r.GetPortHi() != 7071 ||
		r.GetSessions()[0].GetState() != "tcp-established" || r.GetOwner() != testOwner {
		t.Fatalf("page 1: %v %v", r, err)
	}
	r, err = c.Det44Sessions(ctx, &ngfwv1.Det44SessionsRequest{User: "10.7.1.5", Offset: 2, Limit: 2})
	if err != nil || len(r.GetSessions()) != 1 || r.NextOffset != nil {
		t.Fatalf("page 2: %v %v", r, err)
	}
	for _, tc := range []struct {
		req  *ngfwv1.Det44SessionsRequest
		code codes.Code
	}{
		{&ngfwv1.Det44SessionsRequest{User: "10.7.1.5", Limit: 1001}, codes.InvalidArgument},
		{&ngfwv1.Det44SessionsRequest{User: "nope"}, codes.InvalidArgument},
		{&ngfwv1.Det44SessionsRequest{User: "10.9.1.5"}, codes.PermissionDenied},
		{&ngfwv1.Det44SessionsRequest{User: "10.7.9.9"}, codes.NotFound},
		{&ngfwv1.Det44SessionsRequest{User: "10.7.1.5", Owner: "w3"}, codes.InvalidArgument},
	} {
		if _, err := c.Det44Sessions(ctx, tc.req); grpcCode(err) != tc.code {
			t.Errorf("%v: %v (want %s)", tc.req, err, tc.code)
		}
	}

	// lookup both ways (CGNAT logging)
	fw, err := c.Det44Lookup(ctx, &ngfwv1.Det44LookupRequest{InsideAddress: proto.String("10.7.1.70")})
	if err != nil || fw.GetOutsideAddress() != "10.7.2.201" || fw.GetPortLo() != 1024+1008*6 || fw.GetPortHi() != 1024+1008*7-1 {
		t.Fatalf("forward: %v %v", fw, err)
	}
	rv, err := c.Det44Lookup(ctx, &ngfwv1.Det44LookupRequest{OutsideAddress: proto.String("10.7.2.201"), OutsidePort: proto.Uint32(fw.GetPortLo() + 7)})
	if err != nil || rv.GetInsideAddress() != "10.7.1.70" {
		t.Fatalf("reverse: %v %v", rv, err)
	}
	for _, req := range []*ngfwv1.Det44LookupRequest{
		{},
		{InsideAddress: proto.String("10.7.1.5"), OutsideAddress: proto.String("10.7.2.200")},
		{OutsideAddress: proto.String("10.7.2.200")},
		{OutsideAddress: proto.String("10.7.2.200"), OutsidePort: proto.Uint32(80)},
	} {
		if _, err := c.Det44Lookup(ctx, req); grpcCode(err) != codes.InvalidArgument {
			t.Errorf("%v: %v", req, err)
		}
	}

	// close by the inside endpoint, then again (gone), then by the outside endpoint
	out, err := actionRun(t, c, closeReq("in", "10.7.1.5", 40000))
	if err != nil || len(out) != 1 || out[0].GetDone().GetExitCode() != 0 {
		t.Fatalf("close in: %v %v", out, err)
	}
	if out, err = actionRun(t, c, closeReq("in", "10.7.1.5", 40000)); err != nil || out[0].GetDone().GetExitCode() != 1 {
		t.Fatalf("close in again: %v %v", out, err)
	}
	if out, err = actionRun(t, c, closeReq("out", "10.7.2.200", 6065)); err != nil || out[0].GetDone().GetExitCode() != 0 {
		t.Fatalf("close out: %v %v", out, err)
	}
	if _, err = actionRun(t, c, closeReq("out", "10.7.9.9", 6065)); grpcCode(err) != codes.NotFound || strings.Contains(err.Error(), "10.7.9.9") {
		t.Fatalf("close out, unmapped: %v (want NotFound without the endpoint)", err)
	}
	if _, err = actionRun(t, c, closeReq("sideways", "10.7.1.5", 1)); grpcCode(err) != codes.InvalidArgument {
		t.Fatalf("bad direction: %v", err)
	}
	if _, err = actionRun(t, c, closeReq("in", "10.9.1.5", 1)); grpcCode(err) != codes.PermissionDenied {
		t.Fatalf("foreign close: %v", err)
	}
	d := v.Det44()
	d.Lock()
	left, closes, disables := len(d.Sessions[netip.MustParseAddr("10.7.1.5")]), d.Closes, d.Disables
	d.Unlock()
	if left != 1 || closes != 2 || disables != 0 {
		t.Fatalf("model: %d sessions left, %d closes, %d disables", left, closes, disables)
	}
}

func TestCnatStateOnFake(t *testing.T) {
	v := coretest.New()
	cn := v.Cnat()
	cn.Lock()
	for i := byte(0); i < 5; i++ {
		cn.Sessions = append(cn.Sessions, cnatapi.CnatSession{
			Tuple: cnatapi.Cnat5tuple{Addr: [2]ip_types.Address{ip_types.NewAddress(netip.AddrFrom4([4]byte{10, 7, 2, 100}).AsSlice()),
				ip_types.NewAddress(netip.AddrFrom4([4]byte{10, 7, 1, 10 + i}).AsSlice())}, Port: []uint16{80, 40000}, IPProto: ip_types.IP_API_PROTO_TCP},
			TsIndex: 1})
	}
	// two rows of another slot (w9: 10.9.0.0/16) interleaved: a w7 agent never sees them (review BLOCK 2)
	for _, i := range []int{1, 3} {
		foreign := cnatapi.CnatSession{Tuple: cnatapi.Cnat5tuple{Addr: [2]ip_types.Address{ip_types.NewAddress(netip.AddrFrom4([4]byte{10, 9, 2, 100}).AsSlice()),
			ip_types.NewAddress(netip.AddrFrom4([4]byte{10, 9, 1, 10}).AsSlice())}, Port: []uint16{80, 40000}, IPProto: ip_types.IP_API_PROTO_TCP}}
		cn.Sessions = append(cn.Sessions[:i], append([]cnatapi.CnatSession{foreign}, cn.Sessions[i:]...)...)
	}
	cn.Unlock()
	t.Setenv("NGFW_GLOBALS_OWNER", "0")
	s := newSvc(t, v, t.TempDir())
	c := natServer(t, s)
	ctx := context.Background()
	r, err := c.CnatSessions(ctx, &ngfwv1.CnatSessionsRequest{Offset: 4, Limit: 2})
	if err != nil || len(r.GetSessions()) != 1 || r.NextOffset != nil || r.GetTotalSessions() != 5 || r.GetTruncated() {
		t.Fatalf("page: %v %v", r, err)
	}
	row := r.GetSessions()[0]
	if row.GetDstAddress() != "10.7.2.100" || row.GetDstPort() != 80 || row.GetSrcAddress() != "10.7.1.14" || row.GetProtocol() != "tcp" {
		t.Fatalf("row %v", row)
	}
	if r, err = c.CnatSessions(ctx, &ngfwv1.CnatSessionsRequest{Limit: 2}); err != nil || r.GetNextOffset() != 2 {
		t.Fatalf("first page: %v %v", r, err)
	}
	old := cnatSessionCap
	cnatSessionCap = 3
	r, err = c.CnatSessions(ctx, &ngfwv1.CnatSessionsRequest{})
	cnatSessionCap = old
	if err != nil || !r.GetTruncated() || r.GetTotalSessions() != 2 || len(r.GetSessions()) != 2 { // the cap counts rows looked at (one is foreign)
		t.Fatalf("capped: %v %v", r, err)
	}
	for _, x := range r.GetSessions() {
		if strings.HasPrefix(x.GetDstAddress(), "10.9.") {
			t.Fatalf("slot w7 saw another owner's session %v", x)
		}
	}
	// the globals owner sees the whole table
	t.Setenv("NGFW_GLOBALS_OWNER", "1")
	if r, err = c.CnatSessions(ctx, &ngfwv1.CnatSessionsRequest{}); err != nil || r.GetTotalSessions() != 7 {
		t.Fatalf("globals owner: %v %v", r, err)
	}
	t.Setenv("NGFW_GLOBALS_OWNER", "0")
	if _, err := c.CnatSessions(ctx, &ngfwv1.CnatSessionsRequest{Limit: 5000}); grpcCode(err) != codes.InvalidArgument {
		t.Fatalf("limit: %v", err)
	}

	purge := &ngfwv1.ActionRequest{Action: &ngfwv1.ActionRequest_CnatSessionPurge{CnatSessionPurge: &ngfwv1.CnatSessionPurgeAction{}}}
	t.Setenv("NGFW_GLOBALS_OWNER", "0")
	if _, err := actionRun(t, c, purge); grpcCode(err) != codes.PermissionDenied {
		t.Fatalf("slot purge: %v", err)
	}
	t.Setenv("NGFW_GLOBALS_OWNER", "1")
	out, err := actionRun(t, c, purge)
	if err != nil || len(out) != 1 || out[0].GetDone().GetExitCode() != 0 {
		t.Fatalf("purge: %v %v", out, err)
	}
	cn.Lock()
	n, purges := len(cn.Sessions), cn.Purges
	cn.Unlock()
	if n != 0 || purges != 1 {
		t.Fatalf("after purge: %d sessions, %d purges", n, purges)
	}
}
