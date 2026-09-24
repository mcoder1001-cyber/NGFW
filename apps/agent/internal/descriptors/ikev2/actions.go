package ikev2

import (
	"context"
	"fmt"

	"ngfw/agent/binapi/ikev2"
	"ngfw/agent/internal/vpp"
)

// Action helpers for the VPN F-* tasks (initiator flows, rekey orchestration). They are not
// descriptors: they change runtime state, not configuration, and nothing retrieves them. Profile
// names are the desired names; the owner prefix is added here.

// InitiateSAInit starts IKE_SA_INIT for the owned profile (ikev2_initiate_sa_init).
func InitiateSAInit(ctx context.Context, c vpp.Client, owner, profile string) error {
	name := owner + "-" + profile
	if _, err := ikev2.NewServiceClient(c).Ikev2InitiateSaInit(ctx, &ikev2.Ikev2InitiateSaInit{Name: name}); err != nil {
		return fmt.Errorf("ikev2_initiate_sa_init (%s): %w", name, err)
	}
	return nil
}

// DeleteIKESA deletes the IKE SA with initiator SPI ispi (ikev2_initiate_del_ike_sa).
func DeleteIKESA(ctx context.Context, c vpp.Client, ispi uint64) error {
	if _, err := ikev2.NewServiceClient(c).Ikev2InitiateDelIkeSa(ctx, &ikev2.Ikev2InitiateDelIkeSa{Ispi: ispi}); err != nil {
		return fmt.Errorf("ikev2_initiate_del_ike_sa (%#x): %w", ispi, err)
	}
	return nil
}

// DeleteChildSA deletes the child SA with SPI ispi (ikev2_initiate_del_child_sa).
func DeleteChildSA(ctx context.Context, c vpp.Client, ispi uint32) error {
	if _, err := ikev2.NewServiceClient(c).Ikev2InitiateDelChildSa(ctx, &ikev2.Ikev2InitiateDelChildSa{Ispi: ispi}); err != nil {
		return fmt.Errorf("ikev2_initiate_del_child_sa (%#x): %w", ispi, err)
	}
	return nil
}

// RekeyChildSA rekeys the child SA with SPI ispi (ikev2_initiate_rekey_child_sa).
func RekeyChildSA(ctx context.Context, c vpp.Client, ispi uint32) error {
	if _, err := ikev2.NewServiceClient(c).Ikev2InitiateRekeyChildSa(ctx, &ikev2.Ikev2InitiateRekeyChildSa{Ispi: ispi}); err != nil {
		return fmt.Errorf("ikev2_initiate_rekey_child_sa (%#x): %w", ispi, err)
	}
	return nil
}
