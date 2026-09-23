package df6

import (
	"context"
	"errors"
	"fmt"

	"ngfw/agent/binapi/ip"
	"ngfw/agent/internal/vpp"
)

// ErrNoSuchTable: a referenced IP table does not exist in VPP.
var ErrNoSuchTable = errors.New("no such ip table")

// RequireTable checks that IP table id (IPv6 when ip6) exists; table 0 always exists. Several
// VPP 26.06 handlers (SR policy / steering / localsid, …) index the FIB with fib_table_find()'s
// ~0 "not found" result and crash VPP instead of failing, so descriptors check first rather
// than relying on the scheduler's vrf/<id> dependency alone (it does not know the family).
func RequireTable(ctx context.Context, c vpp.Client, id uint32, ip6 bool) error {
	if id == 0 {
		return nil
	}
	stream, err := ip.NewServiceClient(c).IPTableDump(ctx, &ip.IPTableDump{})
	if err != nil {
		return fmt.Errorf("ip_table_dump: %w", err)
	}
	tables, err := Collect(stream.Recv)
	if err != nil {
		return fmt.Errorf("ip_table_dump: %w", err)
	}
	for _, t := range tables {
		if t.Table.TableID == id && t.Table.IsIP6 == ip6 {
			return nil
		}
	}
	fam := "ipv4"
	if ip6 {
		fam = "ipv6"
	}
	return fmt.Errorf("%w: %s table %d", ErrNoSuchTable, fam, id)
}
