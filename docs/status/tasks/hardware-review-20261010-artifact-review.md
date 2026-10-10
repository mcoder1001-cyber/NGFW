# Independent native Debian artifact review

Input build: package version `0.1.0~dev+cc80be66edcf`, four native `.deb` archives in `/dev/shm`, prepared from the manager's documentation-only source checkpoint. An independent `git diff d2d55984d74fa1d06c32e8271886f11f16375407 HEAD -- apps packages deploy tools .github pnpm-lock.yaml package.json` in `/root/ngfw-wt/hardware-manager-20261010` returned no product-code difference before this artifact inspection.

All inspection was read-only, except extraction of the API control metadata to the reviewer's private `/tmp/ngfw-hardware-review-api-control-20261010` scratch directory. No package was installed and no product/host code was edited. Both target root filesystems remain corrupt and block installation, activation and reboot independently of the package finding below.

## MAJOR: native API shared-library dependencies are dropped from final control

Source location: `deploy/debian/ngfw/debian/control`, the `Package: ngfw-api` `Depends` line. Failure: that line lacks `${shlibs:Depends}` even though the API ships a native GNU/Linux Argon2 Node addon. The generated shlibs result is consequently discarded by `dh_gencontrol`.

Independently executed:

```text
dpkg-deb -f /dev/shm/ngfw-api_0.1.0~dev+cc80be66edcf_amd64.deb Package Version Architecture Depends
Package: ngfw-api
Version: 0.1.0~dev+cc80be66edcf
Architecture: amd64
Depends: nodejs (>= 22), nodejs (<< 23), adduser, python3

cat /dev/shm/ngfw-hardware-package-20261010/debian/ngfw-api.substvars
shlibs:Depends=libc6 (>= 2.34), libgcc-s1 (>= 4.2)
misc:Depends=
misc:Pre-Depends=
```

The actual API archive contains:

```text
usr/lib/ngfw/api/node_modules/.pnpm/@node-rs+argon2-linux-x64-gnu@2.2.1/node_modules/@node-rs/argon2-linux-x64-gnu/argon2.linux-x64-gnu.node
```

Independent `readelf -d` on that staged native object returned `DT_NEEDED` for `libdl.so.2`, `libgcc_s.so.1`, `librt.so.1`, `libpthread.so.0`, `libc.so.6`, and `ld-linux-x86-64.so.2`. Generated package dependency computation succeeded, but final API metadata does not carry its output. The observed native version requirements include GCC_4.2.0 and GLIBC symbol versions through GLIBC_2.14; the generated Debian minimum libc constraint is `>=2.34`. This receipt does not claim that GLIBC_2.34 was found as a symbol in the addon.

The final build log explicitly confirms:

```text
dpkg-gencontrol: warning: package ngfw-api:
    substitution variable ${shlibs:Depends} unused, but is defined
```

Corrective action: add the existing computed `${shlibs:Depends}` to the API Depends field, rebuild and independently check the resulting `.deb` final control includes `libc6 (>= 2.34)` and `libgcc-s1 (>= 4.2)` alongside the existing Node constraints. Future focused regression coverage tying native payload dependency substitution to final package metadata is recommended; this review requires the concrete rebuilt metadata check. Do not hide the warning or manually hardcode only today's library names. This is a real packaging correction, not lab-only deferred acceptance.

Severity is MAJOR and blocks declaring these artifacts ready. A runtime failure on the two targets is not proven: Ubuntu resolute's installed/base or transitive Node libraries may already satisfy these dependencies. That does not make discarded native dependency metadata correct. No product merge or release approval is asserted.

## Other warnings assessed

- `ngfw-agent` reports `${shlibs:Depends}` undefined: independently inspected staged `ngfw-agent` and `ngfw-startupgen` are statically linked x86-64 ELF binaries from CGO-disabled builds. No missing dynamic dependency was demonstrated for the agent; an empty substitution is not itself a runtime failure.
- `dpkg-shlibdeps` reports the libc6 loader diversion under usr-merge. It still computed the API dependency result above and reported no missing library. The diversion warning alone is not independently graded a blocker; it must not be used to dismiss the separate discarded-variable finding.

## Independently inspected artifact bytes

| Artifact | SHA256 |
| --- | --- |
| ngfw-agent_0.1.0~dev+cc80be66edcf_amd64.deb | 09598a3c81f6ea288bf5f263812cad68fda412d3e77a8a43c95e2d5b5339f006 |
| ngfw-api_0.1.0~dev+cc80be66edcf_amd64.deb | 53ae499eae2c25a79e4462e5503aa2690189288ce94dc21c7954e1daf1328779 |
| ngfw-web_0.1.0~dev+cc80be66edcf_all.deb | e4601b2b32d312b0fc956817447bc56cba5f8f03f10e633cbd906fb7ad0b73cf |
| ngfw-meta_0.1.0~dev+cc80be66edcf_all.deb | 01ce9a9835c451a89a964c7e82a6f1bf99b4673dee2387972c5bcbf9e3cfbc8d |

A Python inspection streamed `dpkg-deb --fsys-tarfile` for all four archives without executing payloads. It checked archive paths have no absolute or parent-traversal entries, and critical non-node_modules regular files under `/usr/lib/ngfw`, systemd units and `/usr/sbin/ngfw-agent` are owned root:root and not group/world writable. It verified the three helper payloads exactly match their installed byte receipts and have mode 0755:

```text
ngfw-ra-daemon: 9a41b8b873b008f1a364b88937e0ee1e52b12db6c488215b197875991f7be44f
ngfw-ra-namespace-broker: 7511796cf27805b5dca2dbc270cf592b9ca74898ce7fcee899017f35f47e4d61
ngfw-wan-probe: 00cec4f44c75e63a0155a440d49ff18b0bc9ac512587ad9a12857df85e907deb
All three payload-to-.sha256 comparisons PASS.
```

Byte comparisons against the reviewed source passed for packaged agent/API/firstboot/firewall-bootstrap units and firstboot/firewall-bootstrap/base-policy-renderer scripts. Final agent/API/meta `postinst` metadata was inspected and contained no `deb-systemd-invoke`, `invoke-rc.d`, `systemctl start` or `systemctl restart`; the previously reviewed meta future-boot enablement remains. Final meta dependencies exactly pin all three management packages to the build version and `vpp (= 26.06-release+ngfw3)`. These targeted inspection commands exited zero; no broad test suite was rerun.

The manager reported 41 staged packaging fixture tests passing. This reviewer did not rerun them and does not count them as independent live installation or acceptance.

**Verdict: BLOCK artifact readiness for the current native API dependency metadata omission; APPROVE the inspected helper receipts and preserved activation safeguards. Hardware install/reboot remains separately BLOCKED until clean offline filesystem recovery and network verification.**

## Corrected source verification

The manager published `2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c`, `fix(packaging): declare native API library dependencies`. Independently ran `git show` for that exact source commit and compared packaging rules/tests, `tools/ci.sh` and the hosted workflow against the reviewed base. The sole product change is:

```diff
-Depends: ${misc:Depends}, nodejs (>= 22), nodejs (<< 23), adduser, python3
+Depends: ${misc:Depends}, ${shlibs:Depends}, nodejs (>= 22), nodejs (<< 23), adduser, python3
```

Rules, tests and both CI implementations are unchanged; the manager checkout was clean. This uses the already generated shlibs bounds without hardcoding library names or changing Node constraints, runtime privilege boundaries or activation behavior.

**R8 source-only verdict: APPROVE exact correction `2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c`.** The old `cc80be66edcf` archives retain their artifact-readiness BLOCK. The corrected final build has now received the independent inspection below. Source approval is not a claim that hosted quick, target filesystem recovery or hardware acceptance have passed.

## Corrected final archive inspection

Input: `/root/Documents/Codex/2026-10-10/hardware/runtime-fixed/`, version `0.1.0~dev+2045ab8b3d2f`. The manager reported `dpkg-buildpackage` exit zero and 41 unchanged staged fixtures passing. Independently compared product directories/CI/lock files between source `2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c` and D112 integration `bde83bc87ae817a61bbc70e4029f76109ae77c35`: no differences. Actual remote branch readback confirmed integration `bde83bc87ae817a61bbc70e4029f76109ae77c35` and its preserved integration archive ref. The differing package version is provenance from the reviewed source checkpoint, not a different product tree.

| Corrected artifact | Independently computed SHA256 |
| --- | --- |
| ngfw-agent_0.1.0~dev+2045ab8b3d2f_amd64.deb | 7e505cf9c6415a58c45a39366b512667d4ad79e2f6db340f7ef0d639ca6431bf |
| ngfw-api_0.1.0~dev+2045ab8b3d2f_amd64.deb | 47683ec5b2194ea360c656648718f48943cb04480717cf59804e5ff6ec6309a0 |
| ngfw-web_0.1.0~dev+2045ab8b3d2f_all.deb | f124a576f14435903dfe62f69fd6853a75c2e609528a5c6e0733334a93d48d56 |
| ngfw-meta_0.1.0~dev+2045ab8b3d2f_all.deb | 4a16c2e6b04da696876c2978a36af9feca238ab55ec120801ffdfb63c89c777d |

All seven manifest/archive/buildinfo/changes hashes matched `SHA256SUMS`. Actual final control metadata for all four archives agreed with the manifest and expected versions/architectures. API now carries both generated native bounds and unchanged Node constraints. Meta exactly pins the three management packages and VPP. Independent streamed payload inspection repeated safe path and critical root ownership/non-writable-mode assertions, all three helper byte-to-receipt comparisons with 0755 executable modes, all seven reviewed unit/script byte comparisons and final maintainer-script no-start/restart inspection. Native Argon2 was extracted only into private temporary scratch, read with `readelf`, then removed; its DT_NEEDED set remained the six libraries recorded above. No binary was executed, full API tree extracted, package installed or target mutated.

Command/output appendix (each exited zero):

```text
cd /root/Documents/Codex/2026-10-10/hardware/runtime-fixed
sha256sum --check SHA256SUMS
manifest.json: OK
ngfw-agent_0.1.0~dev+2045ab8b3d2f_amd64.deb: OK
ngfw-api_0.1.0~dev+2045ab8b3d2f_amd64.deb: OK
ngfw-meta_0.1.0~dev+2045ab8b3d2f_all.deb: OK
ngfw-web_0.1.0~dev+2045ab8b3d2f_all.deb: OK
ngfw_0.1.0~dev+2045ab8b3d2f_amd64.buildinfo: OK
ngfw_0.1.0~dev+2045ab8b3d2f_amd64.changes: OK

dpkg-deb -f ngfw-api_0.1.0~dev+2045ab8b3d2f_amd64.deb Package Version Architecture Depends
Package: ngfw-api
Version: 0.1.0~dev+2045ab8b3d2f
Architecture: amd64
Depends: libc6 (>= 2.34), libgcc-s1 (>= 4.2), nodejs (>= 22), nodejs (<< 23), adduser, python3
```

The independent Python inspection used `subprocess.Popen(['dpkg-deb','--fsys-tarfile',archive])` and `tarfile.open(fileobj=proc.stdout,mode='r|')` for payloads, and `--ctrl-tarfile` for maintainer scripts; it asserted child exit zero, compared bytes with `hashlib.sha256`, and never invoked maintainer scripts. Actual selected output from that exited-zero inspection:

```text
SHA256SUMS: 7/7 PASS
ngfw-agent: metadata/payload/activation checks PASS
ngfw-api: metadata/payload/activation checks PASS
ngfw-meta: metadata/payload/activation checks PASS
ngfw-web: metadata/payload/activation checks PASS
ngfw-ra-daemon: 9a41b8b873b008f1a364b88937e0ee1e52b12db6c488215b197875991f7be44f receipt/mode PASS
ngfw-ra-namespace-broker: 7511796cf27805b5dca2dbc270cf592b9ca74898ce7fcee899017f35f47e4d61 receipt/mode PASS
ngfw-wan-probe: 00cec4f44c75e63a0155a440d49ff18b0bc9ac512587ad9a12857df85e907deb receipt/mode PASS
Source unit/script bytes: 7/7 PASS
API final Depends: libc6 (>= 2.34), libgcc-s1 (>= 4.2), nodejs (>= 22), nodejs (<< 23), adduser, python3
Meta exact management/VPP pins PASS
Native DT_NEEDED: libdl.so.2,libgcc_s.so.1,librt.so.1,libpthread.so.0,libc.so.6,ld-linux-x86-64.so.2
Corrected final archive inspection PASS (no installation/execution)
```

**R8 verdict: APPROVE corrected final archive integrity, concrete dependency correction and reviewed activation safeguards for the matching product tree.** Old archives remain blocked. This closes the native metadata finding; it does not authorize deploying to corrupt targets or establish hosted CI, licensing/release, reboot persistence or packet-forwarding acceptance. Hardware installation/activation/reboot remains BLOCKED pending clean offline recovery and network prerequisites. The manifest's review/CI status fields were recorded before this review and are not independent evidence of readiness; use the exact receipt and applicable hosted result.
