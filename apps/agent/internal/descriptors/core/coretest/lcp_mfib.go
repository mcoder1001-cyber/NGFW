package coretest

// The API multicast source is distinct from linux-cp's source. Model its
// multipath updates so full agent/FRR lifecycle tests exercise real cleanup.
import (
	"fmt"
	"slices"
	"sort"

	"go.fd.io/govpp/api"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/mfib_types"
)

func init() { RegisterExtension("lcp-api-mfib", (*VPP).installLcpAPIMfib) }

func (v *VPP) installLcpAPIMfib() {
	routes := map[uint32]ip.IPMroute{}
	v.On("ip_mroute_add_del", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*ip.IPMrouteAddDel)
		v.mu.Lock()
		defer v.mu.Unlock()
		old := routes[r.Route.TableID]
		old.TableID, old.Prefix = r.Route.TableID, r.Route.Prefix
		if !r.IsMultipath {
			old.Paths = nil
		}
		for _, path := range r.Route.Paths {
			index := slices.Index(old.Paths, path)
			if r.IsAdd && index < 0 {
				old.Paths = append(old.Paths, path)
			}
			if !r.IsAdd && index >= 0 {
				old.Paths = slices.Delete(old.Paths, index, index+1)
			}
		}
		if len(old.Paths) > 255 {
			return nil, fmt.Errorf("model multicast path count exceeds uint8")
		}
		old.NPaths = 0
		for range old.Paths {
			old.NPaths++
		}
		if len(old.Paths) == 0 {
			delete(routes, old.TableID)
		} else {
			routes[old.TableID] = old
		}
		return reply(&ip.IPMrouteAddDelReply{})
	})
	v.On("ip_mroute_dump", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*ip.IPMrouteDump)
		v.mu.Lock()
		defer v.mu.Unlock()
		keys := make([]uint32, 0, len(routes))
		for table := range routes {
			if table == r.Table.TableID {
				keys = append(keys, table)
			}
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
		var out []api.Message
		for _, table := range keys {
			route := routes[table]
			route.Paths = append([]mfib_types.MfibPath(nil), route.Paths...)
			out = append(out, &ip.IPMrouteDetails{Route: route})
		}
		return out, nil
	})
}
