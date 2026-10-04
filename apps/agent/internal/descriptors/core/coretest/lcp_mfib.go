package coretest

// The API multicast source is distinct from linux-cp's source. Model its
// multipath updates so full agent/FRR lifecycle tests exercise real cleanup.
import (
	"fmt"
	"slices"
	"sort"

	"go.fd.io/govpp/api"
	"ngfw/agent/binapi/fib_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/mfib_types"
)

func init() { RegisterExtension("lcp-api-mfib", (*VPP).installLcpAPIMfib) }

func (v *VPP) installLcpAPIMfib() {
	type routeKey struct {
		table  uint32
		prefix ip_types.Mprefix
	}
	routes := map[routeKey]ip.IPMroute{}
	v.On("ip_mroute_add_del", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*ip.IPMrouteAddDel)
		v.mu.Lock()
		defer v.mu.Unlock()

		if r.Route.Prefix.Af == ip_types.ADDRESS_IP4 && r.Route.Prefix.GrpAddressLength == 24 && [4]byte(r.Route.Prefix.GrpAddress.GetIP4()) == [4]byte{224, 0, 0, 0} {
			if !r.IsMultipath {
				return nil, fmt.Errorf("LCP multicast source requires multipath")
			}
			if _, ok := v.Tables[tableKey{r.Route.TableID, false}]; !ok {
				return reply(&ip.IPMrouteAddDelReply{Retval: int32(api.NO_SUCH_FIB)})
			}
			for _, path := range r.Route.Paths {
				if path.Path.SwIfIndex == ^uint32(0) {
					if path.ItfFlags != mfib_types.MFIB_API_ITF_FLAG_FORWARD || path.Path.Type != fib_types.FIB_API_PATH_TYPE_LOCAL {
						return nil, fmt.Errorf("invalid LCP multicast local path")
					}
				} else {
					iface, ok := v.Ifaces[path.Path.SwIfIndex]
					if !ok {
						return reply(&ip.IPMrouteAddDelReply{Retval: int32(api.INVALID_SW_IF_INDEX)})
					}
					if iface.Table4 != r.Route.TableID || path.ItfFlags != mfib_types.MFIB_API_ITF_FLAG_ACCEPT {
						return nil, fmt.Errorf("invalid LCP multicast accept path")
					}
				}
			}
		}
		key := routeKey{r.Route.TableID, r.Route.Prefix}
		old := routes[key]
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
			delete(routes, key)
		} else {
			routes[key] = old
		}
		return reply(&ip.IPMrouteAddDelReply{})
	})
	v.On("ip_mroute_dump", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*ip.IPMrouteDump)
		v.mu.Lock()
		defer v.mu.Unlock()
		keys := make([]routeKey, 0, len(routes))
		for table := range routes {
			if table.table == r.Table.TableID {
				keys = append(keys, table)
			}
		}
		sort.Slice(keys, func(i, j int) bool { return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j]) })
		var out []api.Message
		for _, table := range keys {
			route := routes[table]
			route.Paths = append([]mfib_types.MfibPath(nil), route.Paths...)
			out = append(out, &ip.IPMrouteDetails{Route: route})
		}
		return out, nil
	})
}
