# P11 read-only build input intake

Existing historical packaging work is preserved in commits `e4d62ea44dcbfe5e89877af7addc076ff67d98a3`
and `3e22b76d`. Its source was strongSwan 5.9.6 from
`https://download.strongswan.org/strongswan-5.9.6.tar.bz2`, with recorded SHA-256
`91d0978ac448912759b85452d8ff0d578aafd4507aaf4f1c1719f9d0c7318ab7`.
This is recovered historical metadata, not a new authenticity or security assessment.
The historical review marked the plugin/build unusable pending ABI, identifier
allocation, security and packaging corrections. The unsafe legacy builder and C
sources are not restored by this bounded intake change.

`verify_inputs.py` prepares a read-only report for a future staging build:

```sh
python3 deploy/strongswan/verify_inputs.py \
  --vpp-output /owned/verified-product-vpp-output \
  --source /owned/strongswan-5.9.6.tar.bz2 \
  --source-version 5.9.6 \
  --source-sha256 EXPECTED_SHA256_FROM_SEPARATELY_TRUSTED_CHANNEL
```

The supplied expected digest is required; calculating a digest from an unknown
archive and supplying that same digest cannot authenticate the publisher. Obtain
the expected digest through an authenticated release/signature or another
separately trusted channel. The
reported `expected_origin` is the historical expected upstream URL; the checker never
fetches it and cannot prove where the local archive came from. A successful
report always has `release_approved: false`. No release is approved here.

The checker takes bounded private snapshots under `/var/tmp`, using the existing
no-follow bundle copier and helper environment. It runs the unchanged complete
VPP `verify.sh --require-files --install-gate`, with its tests enabled. Then it
checks metadata/hash/size of `vpp-dev`, `libvppinfra-dev`, `vpp` and `libvppinfra`
against that same verified manifest. The host's unsuffixed VPP development
packages cannot substitute for the verified product build. These four packages
are build inputs only; dev archives must not enter the runtime delivery plan.

The source copy must match the separately trusted digest. Bzip2 tar member
inspection requires a single `strongswan-5.9.6` root, pregenerated executable
`configure` and nonempty `src/libstrongswan/settings/settings_parser.c`. Duplicate
members, traversal, absolute paths, symlinks, hardlinks, special files and
oversized source/member inventories are rejected. Nothing is extracted, executed,
downloaded, installed or compiled. Temporary snapshots are deleted on success
and failure. This conservative profile can refuse archives with harmless links;
a separate reviewed implementation is needed to support those archives.

This does not validate compiler/linker compatibility, runtime dependency closure,
source security, package payloads or plugin ABI/ID allocation. A real product
VPP build and authenticated source input have not been demonstrated here. Actual
`.deb` building, security/licensing resolution, plugin integration, clean target
installation, IKE/ESP traffic and reboot acceptance remain unfinished/NOT RUN.
Synthetic fixtures use real small Debian/bzip2 archives but stub VPP provenance;
a separate negative fixture runs the real gate and rejects those fake inputs.

برای آماده‌سازی ورودی ساخت، هش مورد انتظار سورس را از مسیر مستقل و مطمئن تهیه
کنید. ابزار فقط ورودی‌ها را بررسی می‌کند؛ هیچ فایل سورسی استخراج یا اجرا نمی‌شود
و هیچ بسته‌ای روی میزبان نصب نمی‌شود. خروجی موفق، تأیید انتشار یا امنیت محصول
نیست. ساخت بستهٔ واقعی، رفع مشکلات پلاگین و آزمون سخت‌افزار هنوز انجام نشده است.

## Materialised prerequisites (builder remains unfinished)

`prepare_stage.py` consumes the same complete intake checks and their private
snapshots, including the unchanged `--require-files --install-gate` VPP verifier
with tests enabled. `verify()` remains read-only; its scoped `verified_snapshot()`
API keeps the checked bytes alive until the consumer finishes. No original input
path is reopened for extraction.

```sh
python3 deploy/strongswan/prepare_stage.py \
  --vpp-output /owned/verified-product-vpp-output \
  --source /owned/strongswan-5.9.6.tar.bz2 \
  --source-version 5.9.6 \
  --source-sha256 EXPECTED_SHA256_FROM_SEPARATELY_TRUSTED_CHANNEL \
  --output /private-owned-parent/new-stage
```

The Python `prepare(vpp_output, source, version, digest, output)` API also requires
64 lowercase hexadecimal digest characters before creating output or invoking
intake. The caller must obtain that digest separately through a trusted channel;
this tool cannot establish that trust for the caller.

The parent must already exist, belong to the effective user, and prohibit group
and other writes. The output must be new. Linux `renameat2(RENAME_NOREPLACE)`
publishes the complete private tree atomically and refuses even an output created
concurrently. Every parent component is opened without following symlinks; operations use a
pinned directory descriptor. Destination identity is checked before and after
publication, with rollback of the owned output on a changed parent. Destinations
inside the VPP input tree or at the source path are rejected. Snapshots and
partial trees are removed on errors; publication follows successful snapshot
cleanup. Private directories are mode 0700 and files 0600 (0700 for source/archive
files carrying executable bits). Archived owners, setuid bits and permissions
are never restored. This protects against other users; processes running as the
same user/root remain within the trusted host boundary.

If preparation fails normally, temporary snapshots and partial trees are cleaned up; fix the reported input or space problem and retry with a new output name. Abrupt termination, reboot or power loss can leave private temporary directories. There is no automatic crash recovery, and atomic publication does not promise power-loss durability: staged files and directories are not fsynced. After such an interruption, stop any preparation process you started, inspect leftovers, and remove only your own abandoned `.p11-stage-*` directory and intake snapshots. Do not remove original input archives, another process's directories, or an existing output merely to bypass the no-replace check. Treat a stage surviving power loss as incomplete; discard only that owned stage after inspection and regenerate it from the original inputs and separately trusted source digest under a new output name. No resumable build or release acceptance is implied.

Output contains actual source bytes under `source/strongswan-5.9.6`, decoded
headers and other development payloads under separate `vpp-dev/vpp-dev` and
`vpp-dev/libvppinfra-dev` roots, and `intake.json`. Runtime archives are verified
as part of the quartet but are not extracted. The separate roots avoid silently
overwriting shared payload paths. `dpkg-deb --fsys-tarfile` only decodes archive
data; no maintainer script, source script, configure, compiler or installer runs.

Extraction rejects traversal, absolute paths, empty/dot components (except the
standard Debian `./` prefix/root), duplicates, links, devices and other special
files. It refuses file overwrite and file/directory collisions. Limits are
128 MiB per file, 50,000 members per archive, 1 GiB decoded tar per archive and
1 GiB aggregate decoded tar and regular-file payload across source and both dev
packages. The existing stricter intake source bounds also apply. Decoder output
is streamed to bounded temporary files with a 120-second deadline. Tar metadata
is bounded before parsing. Conservative rejection can refuse upstream/package
archives containing ordinary symlinks; support requires a separately reviewed
change.

Focused fixtures: `python3 deploy/strongswan/test_verify_inputs.py` and
`python3 deploy/strongswan/test_prepare_stage.py`. They build real small Debian
archives and bzip2 source archives; positive VPP provenance is explicitly stubbed.
Negative fixtures call the real complete gate and reject synthetic builds.
Materialisation is a useful prerequisite, **not** a working builder: actual build,
compiler/ABI compatibility, identifier allocation, plugin/agent wiring,
security/licensing and old-source release approval remain unfinished. Reports
retain `release_approved: false` and add `builder_ready: false`.
