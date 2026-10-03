// Disposable-only loss injection for F-tunnels-host. Deletes exactly the three
// fully identified tunnels supplied by the private rig, through binary API.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"go.fd.io/govpp"
	"ngfw/agent/binapi/gre"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ipip"
	"ngfw/agent/binapi/vxlan"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if os.Getenv("NGFW_DISPOSABLE_VPP") != "1" || len(os.Args) != 3 {
		return fmt.Errorf("requires disposable VPP, source and slot base")
	}
	src := os.Args[1]
	base, err := strconv.ParseUint(os.Args[2], 10, 32)
	if err != nil || base < 1000 || base > 32000 || base%1000 != 0 {
		return fmt.Errorf("invalid slot base")
	}
	if src != fmt.Sprintf("10.%d.7.1", base/1000) {
		return fmt.Errorf("source is not the reserved fixture address")
	}
	dst := func(last int) string { return fmt.Sprintf("10.%d.7.%d", base/1000, last) }
	c, err := govpp.Connect("/run/vpp/api.sock")
	if err != nil {
		return err
	}
	defer c.Disconnect()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	gs, is, vs := gre.NewServiceClient(c), ipip.NewServiceClient(c), vxlan.NewServiceClient(c)
	gd, err := gs.GreTunnelDump(ctx, &gre.GreTunnelDump{SwIfIndex: interface_types.InterfaceIndex(^uint32(0))})
	if err != nil {
		return err
	}
	var g *gre.GreTunnelDetails
	for {
		d, e := gd.Recv()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
		if d.Tunnel.Instance == uint32(base+1) && d.Tunnel.Src.String() == src && d.Tunnel.Dst.String() == dst(2) && d.Tunnel.OuterTableID == 0 && d.Tunnel.Type == 0 && d.Tunnel.Mode == 0 {
			if g != nil {
				return fmt.Errorf("ambiguous GRE")
			}
			g = d
		}
	}
	id, err := is.IpipTunnelDump(ctx, &ipip.IpipTunnelDump{SwIfIndex: interface_types.InterfaceIndex(^uint32(0))})
	if err != nil {
		return err
	}
	var i *ipip.IpipTunnelDetails
	for {
		d, e := id.Recv()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
		if d.Tunnel.Instance == uint32(base+2) && d.Tunnel.Src.String() == src && d.Tunnel.Dst.String() == dst(3) && d.Tunnel.TableID == 0 && d.Tunnel.Mode == 0 {
			if i != nil {
				return fmt.Errorf("ambiguous IPIP")
			}
			i = d
		}
	}
	vd, err := vs.VxlanTunnelV2Dump(ctx, &vxlan.VxlanTunnelV2Dump{SwIfIndex: interface_types.InterfaceIndex(^uint32(0))})
	if err != nil {
		return err
	}
	var v *vxlan.VxlanTunnelV2Details
	for {
		d, e := vd.Recv()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
		if d.Instance == uint32(base+3) && d.SrcAddress.String() == src && d.DstAddress.String() == dst(4) && d.Vni == uint32(base+3) && d.EncapVrfID == 0 && d.SrcPort == 4789 && d.DstPort == 4789 && d.McastSwIfIndex == interface_types.InterfaceIndex(^uint32(0)) && d.DecapNextIndex == 1 {
			if v != nil {
				return fmt.Errorf("ambiguous VXLAN")
			}
			v = d
		}
	}
	if g == nil || i == nil || v == nil {
		return fmt.Errorf("expected exactly identified GRE/IPIP/VXLAN before mutation")
	}
	gr, err := gs.GreTunnelAddDel(ctx, &gre.GreTunnelAddDel{IsAdd: false, Tunnel: g.Tunnel})
	if err != nil {
		return err
	}
	if gr.Retval != 0 {
		return fmt.Errorf("GRE delete retval %d", gr.Retval)
	}
	ir, err := is.IpipDelTunnel(ctx, &ipip.IpipDelTunnel{SwIfIndex: i.Tunnel.SwIfIndex})
	if err != nil {
		return err
	}
	if ir.Retval != 0 {
		return fmt.Errorf("IPIP delete retval %d", ir.Retval)
	}
	vr, err := vs.VxlanAddDelTunnelV3(ctx, &vxlan.VxlanAddDelTunnelV3{IsAdd: false, Instance: v.Instance, SrcAddress: v.SrcAddress, DstAddress: v.DstAddress, SrcPort: v.SrcPort, DstPort: v.DstPort, McastSwIfIndex: v.McastSwIfIndex, EncapVrfID: v.EncapVrfID, DecapNextIndex: v.DecapNextIndex, Vni: v.Vni, IsL3: true})
	if err != nil {
		return err
	}
	if vr.Retval != 0 {
		return fmt.Errorf("VXLAN delete retval %d", vr.Retval)
	}
	fmt.Println("deleted exactly owned GRE/IPIP/VXLAN via binary API")
	return nil
}
