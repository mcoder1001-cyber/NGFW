# P10 packaging checkpoint

Branch: `task/P10-packaging-finish-20261002`; base: `53a43ce5`.
First published remote SHA: `c401f4efde321505cdd6a358cf93d044b1d215b4`. Local first checkpoint `ff6223db`; later code publication pending manager upload.
Owned: deploy/debian/vrx/**, deploy/debian/README.md, deploy/apt/**,
scripts/publish-apt.sh, scripts/10-install-runtime.sh, docs/09-os-packages.md,
docs/install/**, deploy/systemd/vrx-* (not manager unit or P11).

Implemented: single-source four-package control/install metadata; product VPP exact-version substitution;
no-start/no-enable package install; idempotent group/user creation and data-preserving upgrade handling;
base hardened API/agent units with explicit AF_NETLINK and approved capability boundary.
The build deliberately refuses missing staged binary/API/web/VPP inputs.

Implemented additionally: prepare.sh verifies product manifest/install gate before staging, builds static Go tools, pnpm deploy production API and web payload, copies units and generates SHA version. Driver has not yet built actual packages.

Verification: four Python host-independent checks PASS; bash -n prepare.sh PASS; git diff --check PASS; tools/ci.sh check PASS with missing-gitleaks warning (not full quick). systemd-analyze verify could not resolve absent vpp.service/product binaries; no unit syntax error reported, acceptance NOT RUN.

Next checkpoint: HTTPS-only nginx config with WS, RESTCONF and host-meta; overwrites forwarded client identity at the trusted local proxy. Real OpenSSL TLS bootstrap generates private 0600 key once, validates existing matching pair and preserves it on retry, refuses partial pairs. Six host-independent checks PASS (including actual TLS generation/retry/refusal). nginx syntax on target and integration NOT RUN.

Remaining: package copyright/license metadata (repository has no LICENSE; do not invent terms), firstboot, nginx enablement, base policy,
APT signing/publication, runtime installer correction, user install/upgrade docs.
This is a scaffold; task is RUNNING, not done. No package install, host config change,
daemon startup, VPP restart or laboratory acceptance has been performed.

Next command: python3 deploy/debian/vrx/tests/test_packaging.py

Independent review fix: explicitly writable/provisioned capture and rsyslog TLS directories; regression checks actual renderer defaults. Correct TLS runtime driver to rsyslog-openssl (renderer uses ossl, not gnutls). Six checks PASS again.

Independent review fix 2: helper binaries install to /usr/lib/vrx/bin, matching apply-startup.sh product defaults; regression compares installed mapping and actual script default. Seven checks PASS. pnpm12 deploy independently verified by reviewer (198 packages); full package build still NOT RUN. Remote checkpoint second SHA: 0d124f3e0440f78a5c1bb5804be2cf1eaa9a12b0.

Firstboot checkpoint: fixed-name PostgreSQL role/database creation, application-owned
Drizzle migrations + existing AuthService seed without opening HTTP, durable expected-admin
password verification; stable generated JWT/32-byte secret key; TLS/bootstrap startupgen,
nginx validation and durable completion marker before deleting bootstrap.env. Production
shell has no test-root bypass. Fixture execution redirects only a test-local copy and uses
fake commands: DB/TLS/startup/nginx failures retain credentials/no completion; retry
preserves generated keys and completes once. Two firstboot control-flow checks PASS,
seven earlier packaging checks PASS; node syntax and bash syntax PASS. Actual PostgreSQL,
API deployment bootstrap, VPP rendering and appliance boot NOT RUN. Firstboot ordering,
service activation/base nftables and signed APT remain pending; task still RUNNING.

Firstboot fresh-review MAJOR fixed: no marker-negative unit condition; optional environment file allows scheduled cleanup after marker publication/crash. Production script still fails closed if first initialization lacks credentials. Three fixture-flow tests PASS incl actual unit condition/marker cleanup boundary.

Firstboot JWT review fix: random generation separately checked before printf; existing api.env must contain exactly one 64-hex stable JWT key; the same persisted key is exported to bootstrap. Four fixture tests PASS including missing/empty/duplicate JWT and failed random command retaining bootstrap credentials with no completion marker.

JWT re-review fix: independently count assignment lines before extracting key, because Bash strips trailing newlines and valid-plus-empty otherwise bypassed extraction regex. Tests now cover valid+empty and empty+valid orderings, both refuse without credential deletion. Four fixture tests PASS.

Signed APT checkpoint: preflight verified VPP install-gate, four same-version product
Debians, exact meta/VPP pin and seven ship:true runtimes; reprepro Release/InRelease
signing with protected local GPG home, public trust anchor export and signature gate
before readiness. Actual dpkg fixture wrong-VPP-pin rejection PASS before output/key
creation. Actual isolated GPG preflight fails because gpg-agent cannot start/connect
in this environment; signing test explicitly SKIPPED/NOT RUN, not PASS. Reprepro
actual generation and release-builder acceptance NOT RUN. Bash syntax PASS.

Activation checkpoint: product-only VPP unit drop-in requires successful firstboot, not just ordering; API/agent also require firstboot. Meta postinst enables future boot units via deb-systemd-helper without starting/restarting VPP. Eight packaging fixture checks PASS, four firstboot fixture tests PASS. Target service boot/ordering acceptance NOT RUN.

Fresh arbiter input parity fix: DATABASE_URL and SECRET_KEY_FILE assignment counts must each be exactly one before exact canonical-value checks. Bootstrap exports the parsed validated persisted values. Five firstboot fixture tests PASS, including eight duplicate alternate/empty orderings across both critical fields; failures retain credentials/no marker.

Added installation/upgrade instructions with current implementation and explicit release prerequisites; no WIP device install recommendation. Questions document missing license and exact approved capability/rsyslog-owner mismatch without broadening privileges.

Fresh arbiter key precedence fix: initial api.env accepts only the three generated canonical variables; unsupported overrides such as JWT_KEY_FILE are refused, as is an inherited nonempty bootstrap JWT key-file override. Operators configure optional runtime settings after completed provisioning. Six firstboot fixture tests PASS including valid secret plus nonexistent key-file override preserving credentials/no marker.

Static firewall checkpoint (manager reserved D173): explicit management iface plus
optional unique ≤64 punt iface list; initially empty punt set for no-data-NIC default.
Packaging owns only inet vrx_base, atomic add/delete/recreate same named table; no
flush or writes to renderer-owned inet vrx. Early generation is independent of DB/auth,
preserves distro nftables early-boot ordering/enable target, and nftables requires this
early helper. Main firstboot requires NFT success, then PG/auth. No host load performed;
pure base-policy tests 3 PASS, packaging 8 PASS, firstboot fixtures 6 PASS. Actual nft -c
and fresh full offline systemd graph validation still NOT RUN. Runtime LCP/punt-set sync
is REAL UNBUILT functionality: static DROP can otherwise precede renderer accepts.
P10 remains RUNNING; do not label this limitation a laboratory-only acceptance test.

APT independent security MAJOR fixed: canonical signing home computed before any mutation and repository equal/ancestor/descendant overlap refused before key generation or chmod. Actual fixture overlap variants all rejected without signing directory/output creation; 2 APT validation tests PASS, real GPG/reprepro signing check explicitly SKIPPED in current environment.

APT independent exact-pin MAJOR fixed: parse Debian dependency groups/names; require exactly one standalone vpp (= verified-version), refusing alternatives, lookalikes, duplicate VPP groups and non-equality. Actual dpkg fixtures all rejected before output/key creation. 3 APT validation tests PASS; signing still SKIPPED/NOT RUN due isolated gpg-agent failure.

Unchanged local full quick on 8b761430: pnpm install/generation/contract/pattern/gitleaks
stages PASS; Turbo 34/35 tasks succeeded (12 cached), API unit task FAILED_ENV.
Representative failures: listen EPERM on temporary Unix fake-agent sockets; chown
EINVAL for deliberately foreign-UID JWT fixtures; teardown close errors cascade from
failed socket setup. No assertion/gate changes. Full Go steps not reached. Logs:
/tmp/vrx-ci/NGFW-packaging-finish-20261002-165449-5/08-turbo.log and .scratch/p10-quick.log.
This is NOT CI GATE PASSED. Hosted unchanged complete quick remains mandatory.
Debian source build now directly declares Python3/OpenSSL used by its regression checks.

Runtime profile correction: explicit fresh-appliance authorization flag and verified
VPP artifact directory are required BEFORE APT; original VPP install gate must pass,
only seven ship:true local .debs consumed, no public FD.io fallback. Removed
kea-ctrl-agent and added rsyslog-openssl; corrected obsolete strongSwan configure
claims. Actual negative preflight fixture PASS and proves apt-get was never called.
Runtime installation still NOT RUN.

Fixture portability correction: redirected firstboot control-flow copy models root/unit identity and file metadata, so Debian Rules-Requires-Root:no tests run under nonroot builders without product root bypass. Production EUID/stat guards unchanged; six fixture tests PASS.

Runtime installer fresh-review MAJOR fixed: serialized root-owned install lock,
protected exact existing policy-rc.d backup (including symlink/mode/ownership), temporary
no-start guard installed BEFORE every APT call, EXIT/error restoration, refusal to
overwrite a concurrently changed policy. VPP is explicitly disabled after dependency
installation until product units/firstboot are provisioned. Fixture APT actively attempts
service activation and gets 101; missing/file/symlink originals restored on both success
and APT failure (six variants). Two runtime profile tests PASS; no real APT/host service
operation. An uncatchable kill may leave the no-start guard; original backup is retained
for operator recovery. Remaining final source review/hosted full quick still required.

Removed legacy package purges/autoremove and irqbalance stop from runtime profile: NetworkManager/cloud-init and host hardening are outside P10 and can disrupt preconfigured management. Runtime unit registration uses disable without --now; no stop/restart operation in installer. Two runtime fixture tests PASS again. Central manager-authored P10 deferred campaign added to the existing single DEFERRED-ACCEPTANCE.md, preserving real unbuilt/runtime security/license distinctions.

Base-unit offline score acceptance (systemd 255.4): API before5.4 -> after3.0;
agent before6.2 -> after5.0. API gets narrow unprivileged restrictions; no V8
MemoryDenyWriteExecute. Agent retains hostname/clock/device access and net namespaces;
only cgroup namespace denied. Native syscall ABI/LockPersonality/SUID/RT/controlgroup,
kernel-module and tunable restrictions audited against actual production renderers,
ALLOWLIST and shipped apply-startup.sh/lib.sh. Go production reads boot_id/sysfs;
/proc/sys writes observed only integration fixtures. Startup apply/driverctl/sysfs
binding are gated separate manager/operator systemd-run contexts, not invoked by the
agent (rpc_dataplane_startup confirms manager step). No blanket privilege expansion.
Eight packaging regression checks PASS. Actual local unit verify reports missing
VPP/Valkey and product binaries; no ordering cycle emitted, but complete target graph
acceptance is NOT RUN. Fresh independent R4/R8 delta review still required. Global
sysident atomic /etc parent write gap is recorded in parent-owned privilege PENDING;
CAP_CHOWN alone cannot fix it and /etc is not made broadly writable.

## Recovery checkpoint, 2026-10-02

Recovered published `aa76368a` on branch `codex/packaging-resume-20261002` in
`NGFW-packaging`; `git rebase origin/main` confirms current base `53a43ce5` is
already included. Original remote history and independent rulings are preserved.
New owned delta: API postinst provisions `/data/{backups,updates,support}` at
0750 `vrx:vrx`, documents ownership, and updates obsolete preparation warning.
A redirected-script test runs configure twice using only temporary paths and the
builder UID/GID; existing backup bytes and metadata are preserved. Host account
commands are removed from that fixture copy; production account setup unchanged.

Actual checks: `python3 -m unittest discover -s deploy/debian/vrx/tests` ran 24
checks: 23 PASS, 1 signing SKIP because isolated gpg-agent cannot run. Packaging
suite after fixture hardening: 9 tests PASS. `bash -n prepare.sh`, `git diff --check`
PASS. `tools/ci.sh check --base origin/main` PASS with explicit missing-gitleaks
warning; this is not full CI GATE PASSED. Debian `dh` absent; no package installation
or host service operation was attempted. No actual .deb build/installed boot PASS.

Full unchanged hosted quick and independent R4/R8 review of recovered base-unit
hardening plus this storage delta remain required. Dynamic punt admission, missing
license/copyright, CAP_CHOWN/daemon ownership and atomic `/etc` writes remain real
unresolved product constraints, not lab waivers. Actual signing and clean appliance
acceptance remain NOT RUN. Publication paused by manager pending central destination
authorization review; no outbound push attempted by this worker.

Next command: `python3 -m unittest discover -s deploy/debian/vrx/tests` (after any
review corrections); manager publishes this checkpoint and schedules exact-head CI.

Follow-up validation on unchanged product checkpoint `db721ff49a747f8e7453ad1b4f4e11d085237d95`:
automatic review allowed local escalated fixture execution. All 24 packaging tests
PASS (23.814s), including actual isolated GPG generation/signature verification.
The signing fixture still mocks reprepro and VPP verification; this proves signing
control flow and key isolation, not actual signed product repository publication.
No host package/service operation occurred. Unchanged full quick is running with
private writable caches at `/tmp/p10-ci`; install, generation/output gate and
real gitleaks PASS (132.10 KB scanned, no leaks). Turbo and Go results pending.
