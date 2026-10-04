package agent

// F-default-vpp-nics (D-164): the HostNics RPC — a read-only enumeration of the host's physical NICs
// (vppstartup.HostNICs over the same /sys + /proc reader ReadHost uses) so the API can seed the default
// dataplane document on first boot. It never binds a NIC, never touches /etc/vpp and never restarts VPP
// (D-012): whether a NIC is a management interface is exactly the decision the start-up generator makes
// (default-route / sshd-peer NIC + --mgmt-if/--mgmt-pci), and `bound_to_dpdk` comes from the PCI function's driver
// (vfio-pci / uio: the NIC was handed to DPDK and has no netdev) or, for bifurcated drivers, from a live VPP
// interface of type "dpdk" with the NIC's MAC.

import (
	"context"
	"os"
	"regexp"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers/vppstartup"
)

// hostNicsSources builds the host reader's sources for HostNics: the /sys+/proc root and the same
// management overrides the start-up generator honours, read from the environment for the lab slots.
//   - NGFW_SYS_ROOT      prefix of /sys and /proc ("/" in production)
//   - NGFW_VPP_PLUGIN_DIR VPP plugin directory (only used by ReadHost's plugin scan; HostNICs does not need it)
//   - NGFW_MGMT_IF       extra management kernel interfaces (comma/space separated) — like --mgmt-if
//   - NGFW_MGMT_PCI      extra management PCI addresses (comma/space separated) — like --mgmt-pci
//   - NGFW_CONTROL_PORTS local TCP ports whose established peers mark their NIC management (default: sshd, 22)
func hostNicsSources() vppstartup.HostSources {
	base := startupSourcesFromEnv()
	src := vppstartup.HostSources{
		Root:       base.sysRoot,
		PluginDir:  base.pluginDir,
		MgmtIfaces: splitList(os.Getenv("NGFW_MGMT_IF")),
		MgmtPCI:    splitList(os.Getenv("NGFW_MGMT_PCI")),
	}
	for _, p := range splitList(os.Getenv("NGFW_CONTROL_PORTS")) {
		if n, err := strconv.ParseUint(p, 10, 16); err == nil {
			src.ControlPorts = append(src.ControlPorts, uint16(n))
		}
	}
	return src
}

// splitList splits a comma/space/semicolon separated list, dropping empty fields.
func splitList(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\t' })
	if len(fields) == 0 {
		return nil
	}
	return fields
}

// HostNics implements the HostNics RPC.
func (g *server) HostNics(ctx context.Context, req *ngfwv1.HostNicsRequest) (*ngfwv1.HostNicsResponse, error) {
	if err := g.svc.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	nics, notes, err := vppstartup.HostNICs(hostNicsSources())
	if err != nil {
		return nil, err // wrapped in vppstartup.ErrHost → grpc mapping in the interceptor
	}
	dpdkMACs := g.dpdkMACs(ctx)
	resp := &ngfwv1.HostNicsResponse{
		Owner:           g.svc.owner,
		RetrievedAt:     timestamppb.New(g.svc.now()),
		ManagementNotes: reasonOnlyNotes(notes),
	}
	for _, n := range nics {
		resp.Nics = append(resp.Nics, &ngfwv1.HostNic{
			Netdev:       proto.String(n.Netdev),
			Pci:          proto.String(n.PCI),
			Driver:       proto.String(n.Driver),
			Mac:          proto.String(n.MAC),
			IsManagement: proto.Bool(n.IsManagement),
			// a DPDK user-space driver on the PCI function (vfio-pci/uio), or a live VPP dpdk interface with this MAC
			// (bifurcated drivers such as mlx5 keep their netdev while DPDK uses them)
			BoundToDpdk: proto.Bool(n.BoundToDpdk || (n.MAC != "" && dpdkMACs[n.MAC])),
			LinkUp:      proto.Bool(n.LinkUp),
		})
	}
	return resp, nil
}

// dpdkMACs is the set of MAC addresses of live VPP interfaces of type "dpdk" (lower-case). Empty when
// VPP is not connected or the dump fails — HostNics is a host reader and must answer without VPP so the
// API can seed the default document before the data plane is up.
func (g *server) dpdkMACs(ctx context.Context) map[string]bool {
	out := map[string]bool{}
	if !g.svc.vpp.Connected() {
		return out
	}
	tbl, err := g.svc.interfaceTable(ctx)
	if err != nil {
		return out
	}
	for _, st := range tbl {
		if st.GetType() == "dpdk" && st.GetMac() != "" {
			out[strings.ToLower(st.GetMac())] = true
		}
	}
	return out
}

// peerNote matches the peer address in a management note ("control connection from 10.1.2.3"): the admin's client
// address never crosses the API boundary, only the reason class (review R2R4 #5).
var peerNote = regexp.MustCompile(`control connection from [^)\s]+`)

func reasonOnlyNotes(notes []string) []string {
	out := make([]string, 0, len(notes))
	for _, n := range notes {
		out = append(out, peerNote.ReplaceAllString(n, "control connection"))
	}
	return out
}
