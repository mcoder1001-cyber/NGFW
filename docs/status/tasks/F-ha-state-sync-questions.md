# F-ha-state-sync open product questions

Contracts published on this task branch; additive endpoint fields and observed
RPC are documented in F-ha-state-sync-contract.md. Native IPsec engine decision
supersedes the task's historical strongSwan language.

Owner choice remains: steer session-preserving HA users toward NAT44-EI or accept
NAT44-ED session loss. This implementation accepts ED configuration with a warning
and an explicit unsupported badge; it does not silently change plugin modes.
Proposed V-item: VPP native IPsec SA sequence/replay-window state sync is missing;
`ipsec_sad_entry_update` does not carry sequence/replay state. Native IKEv2 rekey
is the current fallback. Manager owns V-item allocation and vpp-code-track edits.

Lab-only acceptance deferred: two distinct VPP nodes, observed UDP delivery,
VRRP convergence and EI session continuity, ED loss behavior and native IKEv2
rekey. No host/global mutation was performed by slot18. Review/quick gate remain
mandatory before merge; deferred lab acceptance does not waive code failures.
