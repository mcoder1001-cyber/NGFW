package ravpn

import (
	"context"
	"errors"
	"os"
	"testing"

	"go.fd.io/govpp/api"
	"ngfw/agent/binapi/memclnt"
	tapapi "ngfw/agent/binapi/tapv2"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/vpp/bootid"
	"ngfw/agent/internal/vpp/fake"
)

func TestGuardedTAPRetiresOnlyDeadAbsentBootGeneration(t *testing.T) {
	for _, scenario := range []string{"safe", "alive", "collision", "boot-race", "namespace-race", "no-probe"} {
		t.Run(scenario, func(t *testing.T) {
			guard, tap, plan, boot, endpoint := guardedFixture(t)
			meta, e := guard.Create(context.Background(), endpoint)
			if e != nil {
				t.Fatal(e)
			}
			old := meta.(TAPReceipt)
			tap.rows = nil
			boot.StartTime++
			guard.Retired = func(identity bootid.Identity) bool { return identity.Equal(old.Boot) && scenario != "alive" }
			guard.Absent = func(_ context.Context, value *tapv2.Tap, index uint32) error {
				if index != old.Index || value.Id != endpoint.Id {
					t.Fatal("foreign proof")
				}
				if scenario == "collision" {
					return ErrBoundary
				}
				if scenario == "boot-race" {
					boot.StartTime++
				}
				if scenario == "namespace-race" {
					plan.NamespaceInode++
				}
				return nil
			}
			if scenario == "no-probe" {
				guard.Absent = nil
			}
			_, e = guard.Create(context.Background(), endpoint)
			if scenario == "safe" {
				if e != nil {
					t.Fatal(e)
				}
				receipt, e := guard.Store.Load(endpoint.Name)
				if e != nil || !receipt.Boot.Equal(*boot) || receipt.Pending {
					t.Fatal("new receipt not verified")
				}
			} else {
				if e == nil {
					t.Fatal("unsafe receipt retired")
				}
				receipt, e := guard.Store.Load(endpoint.Name)
				if e != nil || !receipt.Boot.Equal(old.Boot) || len(tap.rows) != 0 {
					t.Fatal("refused recovery changed ownership/runtime")
				}
			}
			if tap.deleted != 0 {
				t.Fatal("recovery deleted VPP state")
			}
		})
	}
}
func TestTAPAbsenceIncludesEveryOwnersNumericIDsAndRecycledIndices(t *testing.T) {
	plan := networkFixture()
	endpoint, _, e := TransitTAPs(plan, 2432, 2433)
	if e != nil {
		t.Fatal(e)
	}
	for _, scenario := range []string{"absent", "foreign-id", "recycled-index", "same-ns-link", "dump-error"} {
		t.Run(scenario, func(t *testing.T) {
			client := fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{}))
			client.On("sw_interface_tap_v2_dump", func(api.Message) ([]api.Message, error) {
				if scenario == "dump-error" {
					return nil, errors.New("bounded fixture failure")
				}
				if scenario == "absent" {
					return nil, nil
				}
				row := &tapapi.SwInterfaceTapV2Details{ID: 77, SwIfIndex: 88, HostNamespace: "foreign", HostIfName: "foreign"}
				if scenario == "foreign-id" {
					row.ID = endpoint.Id
				}
				if scenario == "recycled-index" {
					row.SwIfIndex = 19001
				}
				if scenario == "same-ns-link" {
					row.HostNamespace = endpoint.HostNamespace
					row.HostIfName = endpoint.HostIfName
				}
				return []api.Message{row}, nil
			})
			e := VerifyTAPAbsent(context.Background(), client, endpoint, 19001)
			if (e == nil) != (scenario == "absent") {
				t.Fatal("all-owner absence proof mismatch")
			}
		})
	}
}
func TestRetiredVPPBootRefusesLiveProcessAndUnknownIdentity(t *testing.T) {
	current := (bootid.Reader{}).ForPID(os.Getpid())
	if !current.Complete() {
		t.Fatal("current process identity unavailable")
	}
	if RetiredVPPBoot(current) || RetiredVPPBoot(bootid.Identity{}) {
		t.Fatal("live/unknown process retired")
	}
	current.StartTime++
	if !RetiredVPPBoot(current) {
		t.Fatal("different process generation not retired")
	}
}
