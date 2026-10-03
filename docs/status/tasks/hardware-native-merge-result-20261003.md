# Reviewed native/host integration result — 2026-10-03

PR [135](https://github.com/mcoder1001-cyber/NGFW/pull/135) merged as `a0922c7e9ca25c50dd9e7b18aab62e8385697443` from one integration commit `9e230fadb799b99c44c0c29a13bb321f62e8fba9`, tree `5b0c7b0f236c7d5f9013fd58a89638db90489b64`.
The unchanged complete [mandatory hosted quick gate](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37139608578) passed. Three independent reviewers approved this exact source tree. No CI or linter configuration was weakened; actual bounds and fixture failures were fixed.

Scoped board closures: F-isis-rip-host, F-system-identity-host, F-tunnels-host, S-capture-retention-stop, S-cli-ipsec, S-vrrp-product-fixes, TD-lcp-leftover-local-path. Board counts: {"merged": 154, "running": 13, "todo": 15, "ready": 13, "parked": 8, "review": 8}. All other rows retain their existing state and contents, apart from the explicit native milestone note.

Native route based VPP IKEv2 PSK, owned IPIP/routes, secure socket secret delivery and observed state/actions are integrated. Native certificate provisioning and SA event publishing remain source work, so F-ikev2-native stays running. Existing certificate references do not imply native certificate support. VRRP F3 daemon-spawn permission is outside the implemented fix and remains a separate ownership decision.

Historical private hardware evidence is preserved with its original pre-brand provenance. This merge does not prove a fresh installed appliance, VPP patch deployment, new physical packet run or throughput. Shared VPP was not restarted. Identity/tunnel T4 browser screenshots remain deferred. Reviewed history is preserved on codex/hardware-native-reviewed-archive-20261003 and original recovery bundle on codex/hardware-native-recovery-20261003.

Actual main push CI result must be recorded separately after observation; pre-merge branch success is not main CI success.
