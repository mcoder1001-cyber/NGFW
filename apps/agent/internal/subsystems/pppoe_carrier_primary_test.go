package subsystems

import (
	"context"
	"errors"
	interfaces "ngfw/agent/binapi/interface"
	"reflect"
	"testing"
)

func TestCarrierPrimaryOrderAndRepeatedPollPreservesAddresses(t *testing.T) {
	f := newForwardingProductFixture(t)
	transit := f.session.Carrier.VPP4()
	f.addresses = []string{transit, f.mirror.LocalIPv4}
	f.prepare(t)
	if !reflect.DeepEqual(f.addresses, []string{f.mirror.LocalIPv4, transit}) {
		t.Fatal(f.addresses)
	}
	if len(f.addressWrites) != 4 {
		t.Fatal("wrong old-order repair", f.addressWrites)
	}
	f.addressWrites = nil
	f.prepare(t)
	if len(f.addressWrites) != 0 {
		t.Fatal("repeated poll churned interface NAT addresses")
	}
}

func TestCarrierPrimaryRecoversEveryInterruptedSubset(t *testing.T) {
	for _, subset := range []string{"empty", "transit", "negotiated", "both"} {
		t.Run(subset, func(t *testing.T) {
			f := newForwardingProductFixture(t)
			transit := f.session.Carrier.VPP4()
			switch subset {
			case "empty":
				f.addresses = nil
			case "negotiated":
				f.addresses = []string{f.mirror.LocalIPv4}
			case "both":
				f.addresses = []string{transit, f.mirror.LocalIPv4}
			}
			f.prepare(t)
			if !reflect.DeepEqual(f.addresses, []string{f.mirror.LocalIPv4, transit}) {
				t.Fatal(f.addresses)
			}
		})
	}
}

func TestCarrierPrimaryForeignAddressOrIdentityNeverDeletes(t *testing.T) {
	for _, fault := range []string{"foreign-address", "wrong-prefix", "duplicate", "native-identity", "owner-tag"} {
		t.Run(fault, func(t *testing.T) {
			f := newForwardingProductFixture(t)
			switch fault {
			case "foreign-address":
				f.addresses = append(f.addresses, "192.0.2.99/32")
			case "wrong-prefix":
				f.addresses = append(f.addresses, "192.0.2.10/24")
			case "duplicate":
				f.addresses = append(f.addresses, f.addresses[0])
			case "native-identity":
				f.nativeFault = true
			case "owner-tag":
				f.rows[2].Tag = "foreign:pppwan"
			}
			if _, err := f.rt.prepareCarrierForwarding(t.Context(), f.session, f.mirror); err == nil {
				t.Fatal("foreign state accepted")
			}
			if len(f.addressWrites) != 0 || len(f.rt.carrierReady) != 0 {
				t.Fatal("foreign state mutated or made ready")
			}
		})
	}
}

func TestCarrierPrimaryEachAmbiguousFailureAndCancellationRecovers(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		for failedWrite := 1; failedWrite <= 4; failedWrite++ {
			f := newForwardingProductFixture(t)
			transit := f.session.Carrier.VPP4()
			f.addresses = []string{transit, f.mirror.LocalIPv4}
			ctx, cancel := context.WithCancel(t.Context())
			f.addressFault = func(*interfaces.SwInterfaceAddDelAddress) error {
				if len(f.addressWrites) == failedWrite {
					if canceled {
						cancel()
						return nil
					}
					return errors.New("reply lost after mutation")
				}
				return nil
			}
			_, err := f.rt.prepareCarrierForwarding(ctx, f.session, f.mirror)
			cancel()
			if err == nil || len(f.rt.carrierReady) != 0 {
				t.Fatal("partial repair reported ready")
			}
			if _, tracked := f.rt.mirrored[f.session.Iface]; !tracked {
				t.Fatal("partial mirror cannot be withdrawn")
			}
			f.addressFault = nil
			f.prepare(t)
			if !reflect.DeepEqual(f.addresses, []string{f.mirror.LocalIPv4, transit}) {
				t.Fatal("failed recovery", f.addresses)
			}
		}
	}
}
