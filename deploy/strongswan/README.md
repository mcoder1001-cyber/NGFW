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
