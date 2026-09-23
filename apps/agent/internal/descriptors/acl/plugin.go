package acl

import (
	"context"
	"fmt"

	"ngfw/agent/binapi/acl"
	"ngfw/agent/internal/vpp"
)

// PluginInfo is the acl plugin's health/limits snapshot (acl_plugin_get_version,
// acl_plugin_get_conn_table_max_entries). The connection-table size is a startup.conf knob
// (acl-plugin { connection count max }) and out of scope here; it is read and logged only.
type PluginInfo struct {
	Major               uint32
	Minor               uint32
	ConnTableMaxEntries uint64
}

// String implements fmt.Stringer.
func (p PluginInfo) String() string {
	return fmt.Sprintf("acl plugin v%d.%d, conn table max %d entries", p.Major, p.Minor, p.ConnTableMaxEntries)
}

// GetPluginInfo reads the plugin version and connection-table limit.
func GetPluginInfo(ctx context.Context, client vpp.Client) (PluginInfo, error) {
	svc := acl.NewServiceClient(client)
	ver, err := svc.ACLPluginGetVersion(ctx, &acl.ACLPluginGetVersion{})
	if err != nil {
		return PluginInfo{}, fmt.Errorf("acl_plugin_get_version: %w", err)
	}
	ct, err := svc.ACLPluginGetConnTableMaxEntries(ctx, &acl.ACLPluginGetConnTableMaxEntries{})
	if err != nil {
		return PluginInfo{}, fmt.Errorf("acl_plugin_get_conn_table_max_entries: %w", err)
	}
	return PluginInfo{Major: ver.Major, Minor: ver.Minor, ConnTableMaxEntries: ct.ConnTableMaxEntries}, nil
}
