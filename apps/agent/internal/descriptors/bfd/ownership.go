package bfd

import (
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/dfkit"
)

// RecordsNoOwnership declares that keys are selected by the explicit numeric ownership range, not a mutable claim record.
func (*AuthKeyDescriptor) RecordsNoOwnership() {}

// CheckPersistent requires durable claims and successful-add recovery records.
func (d *SessionDescriptor) CheckPersistent() error {
	if err := dfkit.CheckClaims(NameSession, d.Owner); err != nil {
		return err
	}
	if err := dfkit.CheckBoot(NameSession, df7.BootStoreFor(d.Owner)); err != nil {
		return err
	}
	return checkMultihopPersistent(d.Owner)
}

// CheckPersistent requires durable echo-source interface claims.
func (d *EchoSourceDescriptor) CheckPersistent() error {
	return dfkit.CheckClaims(NameEchoSource, d.Owner)
}
