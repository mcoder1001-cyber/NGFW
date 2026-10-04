package igmp

import "ngfw/agent/internal/descriptors/dfkit"

// CheckPersistent requires durable interface ownership claims.
func (d *InterfaceDescriptor) CheckPersistent() error {
	return dfkit.CheckClaims(NameInterface, d.Owner)
}

// CheckPersistent requires durable interface ownership claims.
func (d *ListenDescriptor) CheckPersistent() error { return dfkit.CheckClaims(NameListen, d.Owner) }

// CheckPersistent requires durable interface ownership claims.
func (d *ProxyDeviceDescriptor) CheckPersistent() error {
	return dfkit.CheckClaims(NameProxyDevice, d.Owner)
}

// CheckPersistent requires durable interface ownership claims.
func (d *DownstreamDescriptor) CheckPersistent() error {
	return dfkit.CheckClaims(NameDownstream, d.Owner)
}

// RecordsNoOwnership declares global ranges are write-only and guarded by the explicit globals-owner registration role.
func (*GroupPrefixDescriptor) RecordsNoOwnership() {}
