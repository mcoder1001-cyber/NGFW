# Independent resumed plugin/seed evidence review

Same existing hardware task; branch codex/hardware-evidence-review-20261010.
Owned documentation only; no product or target writes.

Actual native runtime371137 B/0600 receipt98b516a17a02cc82a8b6e26b3ac81d3e1ea8b3cf28fb73b1c0a68bd41b9376fe remains failed: all four services/TLS login work, but22 config/event polls stay revision0/empty. Diagnostic49788 B/0600 receipt41dcecbe3e9f47e96a8ab9d9ecd5c0e5ca2b5e0668cd1e112f728fa6e932f383 reports downstream agent validation failure. Files present is not loaded capability proof. No manual revision/database bypass or early binding.

Canonical renderer preserves current plugin switches when dataplane.plugins is absent; stock firstboot previously supplied {dataplane:{}}. Primary VPP26.06 registration marks each of linux_cp, linux_nl and npt66 default_disabled. Thus an explicit canonical configuration is supported; root has also implemented a shipped default bootstrap asset to repair the actual native bootstrap gap. Source/tests/final hosted gate for that product correction remain separately under review.

Primary sources: [LCP registration](https://raw.githubusercontent.com/FDio/vpp/v26.06/src/plugins/linux-cp/lcp_api.c), [netlink registration](https://raw.githubusercontent.com/FDio/vpp/v26.06/src/plugins/linux-cp/lcp_nl.c), [NPT66 registration](https://raw.githubusercontent.com/FDio/vpp/v26.06/src/plugins/npt66/npt66_api.c).

Actual independent private-receipt check ran from this reviewer worktree with Python JSON/hash parsing and exact safe selectors, never printing configuration/secret payload. Selected output:

```text
file=noPCI-plugin-preflight-20261010T123700Z.json
size=274716 mode=0600
sha256=35131df9f0853f2865b197251f067db7762ec664a3dd5cdcd3ca509e234678c6
render_exit=0 dryrun_exit=0
render_stderr_empty=false dryrun_stderr_empty=false
network_equal=true ioerr_before=0x6 ioerr_after=0x6 new_storage_errors=[]
no_pci=true management_blacklist=true physical_dev_rows=0 required_plugins_enabled=3
dryrun_no_apply_flag=true
will use: tcp:172.30.126.195:22 (passes now)
gate: product mode — appliance approval of rendering 367ead293aefd84d4b3f85f882d3dac33834bda9129467c9a1578f7223a39184 (dead-man mandatory)
dry run: nothing changed
document size=3170 mode=0600 sha256=3c1ba66789d713ac8c2a8b8fb55b021ba8279c153d5b68a57794f04289419d4b
```

APPROVE exact readonly preview/source4a63dcc56bbd7343dd218c5773c2257920a026af427bd2d5175ca553c45b9856 and root-only noPCI apply applicability: finite sealed document/source/live/render approval hashes, unchanged protected management, mandatory product dead-man and rollback before native postapply/real seed verification. Original live608B c892394e36bfc45950407128a81c849fdaa1c66a1fbe64ce9154fa271f215b5e; rendered735B367ead293aefd84d4b3f85f882d3dac33834bda9129467c9a1578f7223a39184 reported by operator, exported originals/render independently readback pending. No binding, actual correction/seed PASS or .37 provisioning approval is claimed.

Product checkpointb1f5bea6bb8a7964165b9ba314d09ecbd238750a has been read: fixed bootstrap asset enables these three switches without devices; firstboot installs0600 and invokes canonical generator; meta manifest includes asset; real CLI regression tests noPCI/protected management/missing-LCP refusal. Final focused tests/evidence review underway, not a merge approval.
