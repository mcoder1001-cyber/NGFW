package strongswan

// KernelVppPlugins is the charon plugin load list for the VPP data-plane path (P11): the stock
// DefaultPlugins with kernel-netlink replaced by the out-of-tree kernel-vpp plugin
// (libstrongswan-kernel-vpp.so from extras/strongswan/vpp_sswan), which programs SAs and policies
// through VPP's IPsec binary API. socket-default stays: IKE reaches charon through the linux-cp
// pair in the root netns (D-060), not through VPP's ikev2 UDP sockets. vici is the control channel
// (mandatory). Pass it via WithDaemonConfig(DaemonConfig{Plugins: KernelVppPlugins(), …}).
//
// The plugin name registered by libstrongswan-kernel-vpp.so is "kernel-vpp". charon loads it from
// the plugin directory of the built strongSwan tree; there is no --enable-kernel-vpp switch (it is
// compiled out of tree against a VPP staging sysroot — see deploy/strongswan).
func KernelVppPlugins() []string {
	return []string{
		"random", "nonce", "openssl", "pem", "pkcs1", "pkcs8", "x509", "pubkey",
		"revocation", "constraints", "kernel-vpp", "socket-default", "vici",
	}
}

// KernelVppPluginName is the name charon loads the out-of-tree VPP data-plane plugin under.
const KernelVppPluginName = "kernel-vpp"
