# Host .37 installation continuation after offline recovery

Owner `/root/host_37`, branch `codex/hardware-37-20261010`. Current phase: .37 logical filesystem repair, clean acceptance and protected normal reboot are complete; original SSH22/management/routing/DNS are preserved with explicitly classified unused-data kernel linklocal reboot delta. Temporary exact101 start policy and inactive persistent VPP mask/trusted UTC are prepared with private original-state backup. **Package installation, firstboot, firewall activation, NIC binding and physical acceptance remain NOT RUN**. The following historical dependency/staging observations retain their own time; latest contracts at the end govern actual next steps. RAM rescue ended on normal hardware reboot.

## Actual payload and dependency evidence

Inspected all four corrected product packages in `/root/Documents/Codex/2026-10-10/hardware/runtime-fixed` and all seven shipping VPP packages in sibling `vpp-runtime`. All11 actual SHA256 values match their manifests. Product version0.1.0~dev+2045ab8b3d2f, API amd64 corrected shlibs dependencies libc6>=2.34/libgcc-s1>=4.2 plus Node>=22,<<23; VPP version26.06-release+ngfw3 with exact inter-package pins. Product source2045ab8 is product-tree-identical to reviewed PR217/final main4908716b as established in R1/T1 receipts. Historical fields in runtime manifest still say old integrationbde/quick pending; those fields do not supersede actual final source/main completed CI review receipts and no manager manifest was edited by this worker.

Read-only strict-known-host SSH ran `apt-cache -o Dir::Cache::pkgcache= -o Dir::Cache::srcpkgcache= policy <package>` and `dpkg-query -W` for41 unique external direct dependencies. No apt update/install or cache writes. Local `dpkg --compare-versions` checks show all72 direct Depends terms satisfy supplied/cached versions; zero direct constraint gaps. This is **not a complete transitive/offline dependency closure or actual installation resolver result**. Third-party .debs are not part of these11 local artifacts, so no complete portable/offline bundle is claimed.

| Required runtime | Actual cached candidate | Result |
|---|---|---|
| Node>=22,<<23 |22.22.1+dfsg+~cs22.19.15-1ubuntu1 | Both bounds pass; no NodeSource repo addition is needed for this cached direct requirement |
| libc6>=2.42 (VPP/core plugin) |2.43-2ubuntu2 | Pass |
| libnl3/route>=3.11 |3.12.0-2 | Pass |
| libssl3t64>=3 |3.5.5-1ubuntu3.2 | Pass |
| PostgreSQL>=16 |18+290ubuntu1 | Pass |
| FRR/frr-pythontools |10.5.1-1ubuntu4.1 | Candidate exists; differs from historical reference10.7.1, actual reload/routes still need acceptance |
| Valkey |9.0.4-0ubuntu0.1 | Candidate exists |
| nftables |1.1.6-1 | Candidate exists; not currently installed/captured natively |

Protected controller receipts in private0700 host37 directory, all0600:

| File | Bytes | SHA256 |
|---|---:|---|
| local-package-controls-20261010.json |5050 |13912927f68e8d2733894e803ab10204b83d75b44b762ff126c08f31fc048b35 |
| readonly-runtime-dependency-candidates-20261010.json |17521 |8df2334cbd10109af0fbad07c9b731af0dec64b7472fe0a4d366560c8f45c4b3 |
| readonly-runtime-constraint-comparison-20261010.json |13372 |a008c60c6d29b178e2b46d2717334fc12adf544b2e51a52ec0e069ffa05885ac |
| local-package-maintainer-scripts-20261010.json |20166 |86d8b24aab8fa8f1eb3db3132debe6b25985114f96ac45a392b4bbab46747cbb |
| readonly-install-host-prerequisites-20261010.json |306 |f2dbf123d5552cde9c09fb23f8535a76cd34db077574fa6092afcb83d598a0f5 |

18 actual archive maintainer scripts inspected as text, never executed. Product packaging uses `dh_installsystemd --no-enable --no-start`; meta manually enables future firstboot/agent/API/socket units. Actual VPP postinst contains `deb-systemd-invoke $_dh_action vpp.service` and `invoke-rc.d vpp $_dh_action`, so vendor VPP automatic activation must be blocked during installation. Product source comments alone are not sufficient.

## Concrete prerequisites and management boundary

Fresh readonly host facts: online CPUs0-43, one NUMA node, HugePages_Total/Free/Rsvd/Surp all0, Hugepagesize2048kB; product API not installed. **Hugepages reservation is a concrete firstboot gap:** canonical firstboot invokes ngfw-startupgen, which refuses host reservation0 even for the initial no-PCI document. After clean recovery, choose/reserve an adequate reviewed hugepage budget using actual VPP buffer/heap requirements and persist it; never fake host facts or reserve pages now while recovery is held.

Before packages, reconfirm healthy offline check/result, original22/authentication, root/media status, management enp12s0/address172.30.126.37/24/gateway172.30.126.1/PCI0000:0c:00.0/igc/group58, all addresses/routes/rules and original netplan contents/hashes. Do not apply stale00-installer-config or edit management/networkd/sshd. Repeat hardware inventory after reboot; clock currently wrong and trusted synchronization must precede certificate/browser acceptance.

After healthy storage and phase release, install an atomic policy-rc.d exit101 guard with exact original backup/restoration/recovery marker semantics from scripts/10-install-runtime.sh, and a reviewed temporary persistent VPP mask before any vendor VPP package installation. Preserve any existing mask/start-policy state; do not use a broad wildcard service stop. The persistent mask prevents an interrupted installation/reboot from starting vendor defaults before product firstboot. Use package resolver simulation first; inspect removals/upgrades and ensure no management service/config replacement. Install only verified package hashes/versions; do not force unresolved dependencies or weaken signatures. No broad OS upgrade is part of this continuation. Restore start policy only after complete package/configuration readiness, with the original recovery evidence retained if restoration is ambiguous.

The inspected runtime installer itself runs apt update/install and is **not authorized to run in the present corrupt/held phase**. Its start-policy mechanism is a template for the later target procedure, not a claim that the11 artifacts form its required signed/offline repository. Manager must supply the concrete reviewed payload/install entry point before execution.

## Firstboot/firewall/seed ordering

1. Preserve full native firewall ruleset before activation using newly available nft tooling after dependency installation under suppressed service starts. Existing compatibility exports were empty but do not establish native absence. Check original nftables config/unit state; do not load distro defaults or flush ruleset. Arm a management rollback that removes/restores only packaging-owned inet ngfw_base, leaving all existing tables/routes intact; verify fresh SSH22 from an independent session.
2. Prepare explicit protected bootstrap fields for management enp12s0 and private generated administrator credentials in root-owned0600 bootstrap.env. Never emit credentials in argv/logs/Git. Run canonical provisioning rather than ad hoc SQL/secrets. Original SSH/password settings remain untouched. Initial api.env accepts only database/secret/JWT fields; do not add seed/runtime overrides before successful canonical firstboot.
3. Product firstboot requires PostgreSQL, Valkey and nftables, before VPP/agent/API/nginx. Early firewall-bootstrap validates the explicit interface and uses nft -c before publishing its owned file. The base renderer replaces **only inet ngfw_base**, has only an input hook (no forward chain), accepts loopback/established traffic and new managementTCP22/443 plus relevant IPv6 ND; it does not admit new rescue2222. Before normal base-policy activation, confirm restored originalSSH22 and retire only our RAM rescue2222 through the manager-controlled return procedure; do not retire it while it remains the recovery access path. Review actual full ruleset and preserve management flows instead of blindly loading the baseline. ExecStop override is empty, preventing default whole-ruleset flush on stop.
4. With reviewed hugepages/host facts and service suppression, canonical firstboot initializes local DB/migrations/admin, generates private keys/TLS, renders initial dataplane{} with dpdk no-pci plus detected management blacklist, verifies nginx and publishes completion marker before deleting bootstrap credentials. Require actual success and stable secrets/permissions. Remove the temporary VPP mask only after its initial startup.conf is verified no-PCI/management-blacklisted and all prerequisites are ready.
5. **After canonical firstboot succeeds, before the first API start or any configuration revision/data binding, explicitly enable NGFW_SEED_DEFAULT_NICS=1 through a persistent product API unit override.** The inspected product API unit currently does not set it despite the user docs' appliance expectation; default is safely0. Preserve canonical api.env fields. Explicit agent NGFW_MGMT_IF=enp12s0 and NGFW_MGMT_PCI=0000:0c:00.0 may reinforce the existing default-route/SSH inventory. Start initial no-PCI VPP then agent then API, verify HostNics identifies management, and wait for the single system seed revision. Do not create/import a candidate/revision first, because seeding refuses an already-touched configuration. Seeding creates configuration only and does not bind hardware.

## Exact required persistent physical rows

Management0000:0c:00.0 must be in dataplane.managementPci and excluded from pciWhitelist/devices/physical data rows. Exact seven seeded interfaces must be enabled with physical `{pci, owner:"dataplane", builtIn:true}` and matching devices.<pci>.name:

| Interface | PCI | Original driver | Entire IOMMU group |
|---|---|---|---|
| enp10s0 |0000:0a:00.0 |igc |56, singleton |
| enp11s0 |0000:0b:00.0 |igc |57, singleton |
| enp13s0 |0000:0d:00.0 |igc |59, singleton |
| enp14s0 |0000:0e:00.0 |igc |60, singleton |
| enp15s0 |0000:0f:00.0 |igc |61, singleton |
| enp16s0 |0000:10:00.0 |igc |62, singleton |
| enp17s0 |0000:11:00.0 |igc |63, singleton |

Verify exact rows/whitelist/devices through persisted running product API state and system.seed-defaults audit before binding or manually revising them. Empty br0..br5/br7..br9 have no PCI and are not additional physical data rows. Do not count transient vppctl interfaces as persisted state.

Then manager-reviewed startup dry-run/apply must use the seeded running document, protected management/SSH route checks, actual verified Intel DPDK driver/plugin, real VFIO IOMMU and all-group isolation, exact startup checksum and rollback. Revalidate groups before binding. Managementigc/group58 remains untouched. Physical af_packet is prohibited by product D105; do not bypass that guard or enable no-IOMMU. Data ownership changes require this explicit later phase, not the present recovery preparation.

## Actual acceptance still required

After installation: services/API/HTTPS/login/Unix-socket privileges/private-secret metadata; full firewall/management routes and new SSH22; seven persisted rows and actual VPP mapping; canonical firstboot rerun/idempotent package reconfiguration preserving secrets/data; real routing/NAT/ACL and FRR reload/route tests; reviewed reboot with restored management/configuration. All data ports lacked carrier at preflight, so physical wire forwarding requires connected test links and cannot be claimed from no-PCI smoke or CI. Required dependency/boot/runtime failures must be fixed; only actual unavailable lab traffic may be deferred per owner policy.

Next exact readonly action now: retain held RAMPTY and originalSSH, await root's .37 offline phase release after .211. After confirmed repaired storage and restored management, first package action is a reviewed resolver simulation with verified local artifacts and safe cache/log settings; no install/activation command is authorized by this preparation receipt itself.


## Guarded archive input and solver source

Owned hardware-37-20261010-package-input.py SHA256d5294674a9eede245900d54240c58ce8c88ed8e96282b39e19ab0532c1c21def has controller/PREFLIGHT/SIMULATE AST3 PASS. Actual --check-local exit0 validates all11 manifest archive hashes, expected product source/version, VPPversion, package names, filename safety, sizes and amd64/all controls without contacting the target. Private install-input-local-20261010T103306Z.json4502B/0600 SHA256bbe9b51dd90da1a28cfcd6579995beb780710ad1c3701c43113b8016fb68c3e3; target_contacted=false. Exact controller package set and private input evidence remain outside Git.

Future --upload-simulate-after-recovery mode is NOT RUN and requires a manager-reviewed healthy normal return: originalroot8:2, rescue/nextroot absent, clean filesystem superblock (supplementing completed offline acceptance, not replacing it), trusted target/controller clock within60seconds, unchanged management enp12s0/source172.30.126.37/PCI0c/igc, and /run tmpfs. It requires pre-existing exact root-owned policy-rc.d101 suppression and persistent VPP /dev/null mask/inactive service; it never creates or restores those safeguards. Existing policy/mask preservation and temporary ownership remain a separate reviewed installation step.

Only after these guards, the source creates a new private0700 RAM upload directory, transfers11 verified archives, rechecks sizes/hashes/names plus policy/mask, and runs apt-get -s --no-remove --no-install-recommends with pkgcache/srcpkgcache disabled. Complete actual solver output is private, planned Inst/Remv and critical systemd/SSH/network/libc/bootloader/kernel/firewall lines are counted and retained for manager review. This is an exact local-archive solver input, not a complete signed offline repository. It does not install, apt-update, activate services, bind interfaces, or alter management/firewall/SSH. No target-mode execution is authorized on the current corrupt mounted root.

Next permitted command is local --check-local only; target-mode requires the later post-repair phase release and independent applicability review. Actual package installation remains NOT RUN.


Own start-guards.py0da1fe78a90fa84086aab56e213021fd07263bcf2ebc6f779386b4fc698fa25b AST2PASS/R7source-onlyAPPROVE; exact reviewedpeer2530bc53 deltaonlyownhost37/IP/enp12s0/PCI0c/group58/task/private/tempnames. inspect is readonly and captures actual37originalpolicy/mask/network/clock privately; prepare uses matchingroot-owned0600privatebaseline, durableoriginalroot700/600recoverymarker, atomic101policy/persistentVPPmask and UTCwallclock only. restore removes only our guards and restores originaltype/metadata/content/link; marker partialrollback states checked. No packageinstall/serviceactivation, no NTP/RTC/network/firewall/SSH/devicechange. Targetmodes dormant until verifiedhealthy37normalreturn and parentinstallationphaserelease. Neverborrow211originals. Exactinspectentrypoint afternormalreturn/release: python3 -B docs/status/tasks/hardware-37-20261010-start-guards.py inspect.

Two actual211native-install findings guide conditional37preparation only: transaction VPP_INSTALL_SKIP_SYSCTL=1 prevents VPPmaintainerscript sysctl --system; before nativeagentconfigure inspect actual37localtime identity and ifsame-file relativezoneinfo link triggers guard, canonicalize narrowly to the same verifiedabsolutezoneinfo file with ownprivateoriginalidentitybackup. No cloneassertion, effectiveTZ/hostname/clockchange or relaxedproductguard; exactactual37source/snapshot needs review. Canonicalfirstboot/hugepagebudget/noPCI/NGFW_SEED_DEFAULT_NICS1 ordering and originalsevenphysicaldataNIC acceptance remain required.


Current actual normal acceptance and start suppression are complete: protected management/auth/boot/DNS/storage PASS with explicit data-only kernelLL reboot delta in recovery.md; current L3 baseline preserved by prepare. Exact policy101+persistent inactive VPP mask and durable original-state marker remain. Own package-input.py4cd015b2 includes actual missing jq/pciutils/driverctl/curl/nftables in named apt simulation along with immutable11archives; no broad upgrade/update or service activation. Native actual transaction must set VPP_INSTALL_SKIP_SYSCTL=1 and compare management sysctls. Actual37 localtime is the same verified relative Tehran link finding as211; preserve own exactlink/zone/identity metadata privately and canonicalize only that same verified absolute zone path if required for native guard, never relax guard or change effective timezone. Concrete solver source/plan and native transaction source reviewed before install.


Pre-install native identity compatibility contract: root explicitly authorizes normalizing own observed `/etc/localtime` relative link to the SAME verified absolute `/usr/share/zoneinfo/Asia/Tehran` before APT so the unchanged agent identity guard accepts it. Own identity-native.py38343860d344e47654c7d212f75e7d9af97733fa03dd46fa1a48a9f8c6cda528 (AST2PASS) adapts reviewed211 trusted-directory/openat/nofollow/rootowner/mode/atomic-replace branch, pins actual37 original localtime inode66/root8:2 and zoneinode22527/hash2dbd/1248bytes plus own marker e275. `inspect` first stores full original metadata/hash/currentL3/sysctls/identity/zone/clock in private exclusive0600 fsynced controller file; `canonicalize --baseline EXACT_PRIVATE_PATH` verifies exact original state then atomically replaces only link, fsyncs/etc, checks zonebytes/effectiveTZ/hostname/otheridentity/UTCmonotonic/currentL3/sysctls/guards/suppressedservices unchanged. No package configure/install/firstboot/RTC/NTP/restart/binding operation exists in this helper. Exact source publication/readback and R7 applicability precede inspect/normalization; native APT transaction later executes unchanged product postinst/identity migration with VPP_INSTALL_SKIP_SYSCTL=1. No bypass/unknown timezone path adoption.


Installed-state read-only firstboot evidence contract: own firstboot-inspect.py SHA256e75e7c517845797936389e74aed1b176bf1f0a98b13f728a3d4c778cae11ce59 AST2PASS copies reviewed peer666cb91c selector code exactly except own task/private/IP/enp12 names. Run only after native11-package installation/configuration completes under101/VPPmask; capture full nativeNFT before activation, firstboot/firewall/render/startup scripts metadata/hash and effective units, PostgreSQL/Valkey/other service states, currentfullL3/DNS/protectedPCI, allsysctlconflicts/memory/NUMA/HugePages/currentkernelhealth and absence of firstboot secret artifacts. Output remains private0600/file+directoryfsynced; no package configure/install, service start, sysctlwrite/network/firewall/bind/reboot. This actual snapshot supplies host-specific firstboot source inputs; never borrow211 expected values.

Changed solver source4cd015 local check ACTUALexit0/target_contacted=false: exact11 full archive SHA256/control identities match unchanged4502B private receipt SHA256bbe9b51dd90da1a28cfcd6579995beb780710ad1c3701c43113b8016fb68c3e3 (fresh filename install-input-local-20261010T121101Z.json/exclusive0600/fsync). Additional named packages jq/pciutils/driverctl/curl/nftables are simulation inputs, not claimed installed. Actual target solver and native install remain pending.
