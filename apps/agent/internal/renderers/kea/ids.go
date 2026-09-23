package kea

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
)

// Kea subnet ids (review M1). Kea keys memfile leases and statistics by subnet id, so an id
// must never change while its subnet exists. The id of a new subnet is FNV-32a of
// "<server>/<subnet>" (salted "…#1", "…#2" on a collision); an existing subnet keeps the id it
// has in the configuration Kea runs. That assignment is persisted in the rendered config
// itself (user-context.vrx.server/subnet next to "id"): New and every Apply re-read it from the
// files on disk, so an agent restart or a rollback sees exactly what the daemon runs.
// Collisions are resolved without touching existing subnets: known names are placed first,
// new names (sorted) probe salted hashes until a free id is found.

func hashID(key string, salt int) uint32 {
	h := fnv.New32a()
	if salt == 0 {
		_, _ = h.Write([]byte(key))
	} else {
		_, _ = fmt.Fprintf(h, "%s#%d", key, salt)
	}
	return h.Sum32()%4294967294 + 1
}

// assignIDs gives every name (sorted) an id: previous ids first, then salted hashes.
func assignIDs(names []string, prev map[string]uint32) map[string]uint32 {
	out := make(map[string]uint32, len(names))
	used := map[uint32]bool{}
	for _, n := range names {
		if id, ok := prev[n]; ok && id != 0 && !used[id] {
			out[n], used[id] = id, true
		}
	}
	for _, n := range names {
		if _, ok := out[n]; ok {
			continue
		}
		for salt := 0; ; salt++ {
			if id := hashID(n, salt); !used[id] {
				out[n], used[id] = id, true
				break
			}
		}
	}
	return out
}

// readIDs returns "<server>/<subnet>" → id from a rendered kea-dhcp4/6.conf (missing or
// foreign file: empty map).
func readIDs(path string) map[string]uint32 {
	out := map[string]uint32{}
	b, err := os.ReadFile(path) //nolint:gosec // own config file
	if err != nil {
		return out
	}
	var root map[string]struct {
		Subnet4 []subnet `json:"subnet4"`
		Subnet6 []subnet `json:"subnet6"`
	}
	if json.Unmarshal(b, &root) != nil {
		return out
	}
	for _, v := range root {
		for _, s := range append(v.Subnet4, v.Subnet6...) {
			if s.UserContext != nil && s.UserContext.VRX.Server != "" && s.UserContext.VRX.Subnet != "" && s.ID != 0 {
				out[s.UserContext.VRX.Server+"/"+s.UserContext.VRX.Subnet] = s.ID
			}
		}
	}
	return out
}

// loadIDs refreshes the id assignment from the files on disk.
func (r *Renderer) loadIDs() {
	ids := map[int]map[string]uint32{4: readIDs(r.paths.Dhcp4Conf()), 6: readIDs(r.paths.Dhcp6Conf())}
	r.mu.Lock()
	r.ids = ids
	r.mu.Unlock()
}

func (r *Renderer) prevIDs(family int) map[string]uint32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ids[family]
}
