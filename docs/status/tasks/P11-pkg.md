# P11-pkg — superseded packaging scope

The product owner's route-based-only decision in
`docs/decisions/DEC-ipsec-route-based.md` explicitly excludes the old P11-pkg
strongSwan/kernel-vpp/socket-vpp build. This task therefore closes as
**superseded**, not as an implemented or tested package.

The supported implementation is native VPP IKEv2 with a protected route-based
tunnel interface. Native runtime packaging is tracked by P10/F-vpp-debs and
native host acceptance by P11-host. Existing historical packaging remains for
provenance; no host package was installed or shared VPP changed by this closeout.

Verification: read the owner's decision, reconcile the board dependencies and
retain the clean-appliance installation case in deferred acceptance. Distribution
of the obsolete plugin is neither required nor certified.
