package ipsec_test

import (
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ipsec_types"
)

// short names for the binapi types the tests build literals of
type ipsecSpdEntryV2Alias = ipsec_types.IpsecSpdEntryV2

func interfaceIndex(i uint32) interface_types.InterfaceIndex {
	return interface_types.InterfaceIndex(i)
}
