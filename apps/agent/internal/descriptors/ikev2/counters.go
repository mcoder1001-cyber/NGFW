package ikev2

import (
	"context"
	"fmt"
	"go.fd.io/govpp/adapter"
	"go.fd.io/govpp/adapter/statsclient"
	"io"
	"math"
	ipsecapi "ngfw/agent/binapi/ipsec"
	"ngfw/agent/binapi/ipsec_types"
	"ngfw/agent/internal/descriptors/vpn"
	"ngfw/agent/internal/vpp"
)

// SACounter contains only forwarding measurements, never dumped key material.
type SACounter struct {
	Packets, Bytes int64
	Encap          bool
}

// SACounters joins current owned CHILD SPIs to the IPsec combined-counter index.
// VPP IKE allocates SAD IDs in the high half of the ID space. Foreign entries
// are discarded, and all returned key buffers are wiped immediately on receipt.
func SACounters(ctx context.Context, c vpp.Client, path string, owned []SAState) (map[uint32]SACounter, error) {
	want := map[uint32]map[uint32]bool{}
	for _, sa := range owned {
		for _, ch := range sa.Children {
			if sa.Index > 0xfff || ch.Index > 0x7ff {
				return nil, fmt.Errorf("native SA counter index exceeds capability layout")
			}
			id := (sa.Index << 12) | ch.Index
			for _, spi := range []uint32{ch.ISPI, ch.RSPI} {
				if want[spi] == nil {
					want[spi] = map[uint32]bool{}
				}
				want[spi][id] = true
			}
		}
	}
	out := map[uint32]SACounter{}
	if len(want) == 0 {
		return out, nil
	}
	sc := statsclient.NewStatsClient(path)
	if err := sc.Connect(); err != nil {
		return nil, fmt.Errorf("IPsec stats segment unavailable: %w", err)
	}
	defer func() { _ = sc.Disconnect() }()
	entries, err := sc.DumpStats("^/net/ipsec/sa$")
	if err != nil {
		return nil, fmt.Errorf("IPsec counter snapshot: %w", err)
	}
	var counters adapter.CombinedCounterStat
	for _, entry := range entries {
		if string(entry.Name) == "/net/ipsec/sa" {
			counters, _ = entry.Data.(adapter.CombinedCounterStat)
		}
	}
	if counters == nil {
		return nil, fmt.Errorf("IPsec SA combined counters unavailable")
	}
	stream, err := ipsecapi.NewServiceClient(c).IpsecSaV3Dump(ctx, &ipsecapi.IpsecSaV3Dump{SaID: math.MaxUint32})
	if err != nil {
		return nil, err
	}
	for {
		d, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		vpn.Zero(d.Entry.CryptoKey.Data[:])
		vpn.Zero(d.Entry.IntegrityKey.Data[:])
		if !ownedCounterEntry(want, d.Entry.Spi, d.Entry.SadID) {
			continue
		}
		counter := SACounter{Encap: d.Entry.Flags&ipsec_types.IPSEC_API_SAD_FLAG_UDP_ENCAP != 0}
		for _, worker := range counters {
			if int(d.StatIndex) >= len(worker) {
				continue
			}
			v := worker[d.StatIndex]
			counter.Packets = saturatingCounter(counter.Packets, v.Packets())
			counter.Bytes = saturatingCounter(counter.Bytes, v.Bytes())
		}
		out[d.Entry.Spi] = counter
	}
	return out, nil
}
func saturatingCounter(previous int64, value uint64) int64 {
	if previous < 0 {
		previous = 0
	}
	if value > math.MaxInt64 {
		return math.MaxInt64
	}
	if previous > math.MaxInt64-int64(value) {
		return math.MaxInt64
	}
	return previous + int64(value)
}

// The pinned capability allocates native SAD IDs from SA/CHILD pool indices;
// matching SPI alone would allow a colliding foreign profile to supply counters.
func ownedCounterEntry(want map[uint32]map[uint32]bool, spi, sadID uint32) bool {
	if sadID&0x80000000 == 0 {
		return false
	}
	id := sadID & 0x00ffffff
	if want[spi][id] {
		return true
	}
	return sadID&0xc0000000 == 0xc0000000 && want[spi][id&^uint32(0x800)]
}
