# Validate an offline Debian delivery set

The read-only checker rejects a delivery set missing the management packages,
FRR, any runtime package in the existing appliance installer, the seven shipping
VPP packages, or a declared dependency. It produces hashes and a relative-file
installation plan. It does not install packages or start services.

Prepare a directory containing:

- `vpp/`: the complete real output from `deploy/vpp/build.sh`, including its
  manifest and checksum files. The existing VPP verifier must accept it with
  `--require-files` and `--install-gate`.
- Product archives: `ngfw-agent`, `ngfw-api`, `ngfw-web`, and `ngfw-meta`, all at the
  same version, built from the repository packaging recipe.
- `ngfw-strongswan`, the separately built P11 product package with the VPP plugin;
  an upstream strongSwan package cannot replace it. This complete-runtime profile
  requires it even while `ngfw-meta` only recommends it.
- Runtime and transitive dependency archives for Ubuntu 26.04 amd64. Include
  FRR and `frr-pythontools`, Node.js 22 and PostgreSQL 18. Existing package
  dependency version constraints must be satisfied. The checker does not assume
  dependencies are already installed on the destination machine.

Run from a source checkout with Python 3, Bash and the Debian package tools:

```sh
python3 deploy/debian/bundle/verify.py /path/to/delivery > bundle-manifest.json
python3 deploy/debian/bundle/verify.py /path/to/delivery --manifest bundle-manifest.json
```

Keep the expected manifest outside the delivery directory and obtain it through
a trusted channel. A matching SHA-256 detects changes relative to that manifest;
it does not authenticate the publisher. Existing signed APT publication remains
the release trust mechanism. Never treat a manifest supplied alongside unknown
packages as approval to install them.

Only `install_files` belong to the runtime plan. Development/debug packages in
the full verified VPP build output are excluded. Paths in the plan are relative
to the delivery root; no APT command is emitted or executed. Duplicate package
names, foreign architectures, symlinked archive paths, unresolved versioned
dependencies, declared conflicts/breaks and unsupported relationship syntax
fail closed. Versioned virtual `Provides` and dependency alternatives are
supported. Architecture/profile-restricted relationships and non-amd64 package
sets require a separate supported implementation. In `Depends` and `Pre-Depends`,
a direct real-package `:any` dependency is supported only when the named package
in this amd64/all bundle declares exactly `Multi-Arch: allowed` and satisfies
its version constraint. `foreign`, `same`, `no`, absent or unknown Multi-Arch
values cannot authorize `:any`. This follows [Debian Policy §5.6.34.4](https://www.debian.org/doc/debian-policy/ch-controlfields.html#multi-arch-allowed).

Qualified virtual dependencies remain unsupported and are rejected even when
another alternative could satisfy the relationship. Unqualified versioned
virtual dependencies retain their existing behavior. `:any` in `Provides`,
`Conflicts` or `Breaks`, explicit foreign/native architecture qualifiers,
`:native`, architecture restrictions and build profiles are rejected. No
cross-architecture installation or general Multi-Arch solver is claimed.
Inspection uses a private archive snapshot with bounded output, archive count,
sizes and relationship count. Source replacement during inspection is rejected.
Files can change afterwards: the installer below takes its own private snapshot
and verifies that snapshot against the trusted manifest before use.

The plan proves package metadata closure, not that package payloads, maintainer
scripts, application migrations or services work. It does not perform an APT
solver simulation, a clean-machine install or hardware tests. No real complete
delivery set has been validated in this change. Synthetic `dpkg-deb` fixtures
exercise the checker; their VPP boundary stub is not provenance evidence.

For release, build the real product/VPP archives, obtain runtime dependencies
from authenticated repositories, publish the signed APT repository, and run the
existing clean Ubuntu install/remove/reinstall and appliance boot acceptance.
The existing runtime installer and firstboot safeguards still apply. This
checker is a bounded preparation step; the explicit installer below still needs
real release artifacts and clean-target acceptance.

## Preflight and explicitly install a trusted delivery set

The installer requires the expected manifest outside the delivery directory.
It copies `.deb` archives and the VPP manifest/checksum files into a private,
bounded snapshot using no-follow file descriptors, then runs the existing full
bundle and VPP verification against that snapshot. Symlinks and special files
are rejected. The source directory can be on removable storage; installation
uses the verified private copies. Temporary copies under `/var/tmp` are removed on success or
failure. Every verification helper also runs with a fixed system PATH and
minimal environment; caller loader, shell startup, proxy and temporary-directory
settings are removed before preflight. Allow space for a second copy of the archives (up to 16 GiB).

The default only prints the verified plan and never runs APT or starts services:

```sh
python3 deploy/debian/bundle/install.py /path/to/delivery --manifest /trusted/bundle-manifest.json
```

Only on an explicitly authorized fresh Ubuntu 26.04 amd64 target, request the
mutating operation:

```sh
sudo python3 deploy/debian/bundle/install.py /path/to/delivery --manifest /trusted/bundle-manifest.json --install
```

This requires root, checks the OS/release and dpkg architecture, and first runs
an APT simulation. If simulation fails, installation does not run. Both commands
use the exact verified local archive paths, `--no-download`, `--no-remove`, an
empty private repository list, lists/cache directories, isolated APT
configuration and a minimal environment with no inherited proxy settings.
It does not run `apt update`, permit downgrades, add repositories or install
recommended/suggested packages outside the complete supplied runtime profile.
Existing configuration files are retained (`--force-confold`). APT still reads
the target's installed-package status and uses its normal dpkg database/lock.
Archives remain root-private, so APT reads them as root rather than weakening
the snapshot permissions for its `_apt` helper.

**Installation is a privileged mutation.** Trusted package maintainer scripts
can write configuration, migrate data and start services. The installer does
not sandbox those scripts or prohibit their own network activity. The isolated
APT acquisition path does not prove that every package script is offline.
Installation is not transactional: a failure may leave partially configured
packages; retain the APT/dpkg output and inspect the target before retrying.
Concurrent administrative package changes must be avoided. Use this operation
for fresh targets; it is not a validated upgrade or rollback mechanism.

Synthetic tests build real small `dpkg-deb` archives, stub only the VPP provenance
boundary and capture APT commands without executing them. They prove command
construction and rejection paths, not real artifact provenance or installation.
A genuine complete bundle, signed-release trust, actual clean Ubuntu
installation/remove/reinstall, firstboot and hardware validation remain required.
The bounded verifier's unsupported relationship/Multi-Arch syntax limitations
above still apply and can reject real distribution package sets.


## Export an already verified set for transport

From a source checkout with Python 3, Bash and Debian package tools, export a
complete directory using the separately obtained trusted runtime manifest:

```sh
python3 deploy/debian/bundle/export.py /path/to/delivery --manifest /trusted/bundle-manifest.json --output /owned/output/ngfw-delivery.tar
```

This does not build or fetch missing packages. The existing full bundle and VPP
verification must pass before an archive is produced. It reuses the installer's
private bounded snapshot and clean helper environment. The output directory
must already exist, be owned by the caller, and have no group/other write
permission. Every destination parent component must be a real directory, not a
symlink. Output inside the source tree, a path overlapping the trusted manifest,
unsafe member names, and any existing output file or symlink are rejected.

Export writes a mode-0600 temporary sibling, checks hashes of the bytes actually
streamed, rechecks the whole snapshot and pinned destination directory, and
publishes a new file atomically without replacing an existing name. A concurrent
creator of that output name wins; its file is preserved. Temporary output is
removed on failure. Keep the caller-owned output directory and its ancestry
stable during export; the implementation checks directory identity before and
after publication. Filesystems must support hard links and directory fsync.

The uncompressed tar uses deterministic sorted relative paths and regular file
members. It includes the runtime archives plus the complete verified VPP archive
set, `vpp/manifest.json` and `vpp/SHA256SUMS` needed to rerun the existing VPP
verifier. Nonshipping VPP development/debug archives are retained for that
verification and **never** enter `install_files`. The external expected runtime
manifest is not placed in the tar; retain it separately through the trusted
channel. That expected manifest covers runtime artifacts, not the retained
nonshipping VPP payloads. Export does not add publisher authentication or prove
that those excluded payloads form an authenticated release.

The JSON export report includes `sha256` and `archive_bytes` for the complete
saved tar, including tar headers, padding and the retained nonshipping VPP
archives. `bytes` remains the sum of member payload sizes. Preserve this report
through the same trusted channel used for the expected manifest; a report copied
with an unknown tar cannot authenticate its publisher. Before extraction, compare
`sha256sum /path/to/ngfw-delivery.tar` and `stat -c %s /path/to/ngfw-delivery.tar`
against the trusted report. Reject a mismatch before opening the archive.

برای انتقال، مقدار `sha256` و اندازهٔ `archive_bytes` در گزارش خروجی را از مسیر
مطمئن نگه دارید. پیش از استخراج، هش و اندازهٔ فایل منتقل‌شده را با گزارش مقایسه
کنید؛ در صورت اختلاف، فایل را استخراج یا نصب نکنید. گزارش همراه فایل ناشناس،
اصالت ناشر را ثابت نمی‌کند.

Transport the tar and keep the trusted expected manifest separately. Restore
only an export you trust into a new empty directory with a standard tar tool,
then rerun `verify.py --manifest` or the default `install.py` preflight against
that directory. The exporter supplies no extraction helper or automatic
installation. Allow disk space for the private snapshot plus the uncompressed
archive. The package archive does not embed an installer or create a bootable
image. The separate authenticated helper delivery below removes the recipient's
source-checkout requirement; package archives never authorize helper execution.

Export fixtures build real small Debian archives, exercise deterministic member
content/roundtrip and failure paths, and stub the VPP provenance boundary. They
are not a real complete artifact delivery, signed release, clean-machine install
or hardware acceptance result. Those P10 release gates remain required.

## Deliver trusted helpers to a recipient without a checkout

On the publishing machine, use a trusted checkout at a committed revision.
Export the helper archive into an existing caller-owned output directory outside
the checkout (no group/other write permission):

```sh
python3 deploy/debian/bundle/helpers.py --output /owned/output/ngfw-helpers.tar > /owned/output/helper-report.json
cp deploy/debian/bundle/recipient.py /owned/output/recipient.py
sha256sum /owned/output/helper-report.json /owned/output/recipient.py
```

The helper exporter refuses source files that differ from their committed bytes.
It exports the canonical installer and verifier, the runtime installer's literal
package lists, and the complete VPP verification dependency closure: VERSION,
lib/build/verify scripts, pydeps lock, patch series and patch files (including
optional patches), and `tests/run.sh`. These are byte-identical canonical files
at their existing paths, not a maintained vendored implementation. The report
records the source revision, exact file inventory, sizes, hashes and private
modes, the complete helper tar hash/size and the separate launcher hash/size.
Export is deterministic and refuses to replace an existing output. It neither
builds packages nor signs artifacts.

**Authenticate the launcher before executing it.** Send the expected launcher
SHA256 and helper-report SHA256 through a separately authenticated release
channel, together with the trusted runtime manifest and package transport report.
Copying these hashes alongside unknown files does not establish trust. The
recipient must compare the received `recipient.py` hash with that externally
trusted value before running any Python code. The launcher's own hash check is
only an accidental-mismatch check; executable code cannot authenticate itself.
No public-key signing/bootstrap distribution or ownership policy is supplied by
this change. Existing signed APT publication remains the release trust mechanism.

On a recipient, only the delivered `recipient.py`, `ngfw-helpers.tar`, helper report,
external runtime manifest and restored package directory are needed. No repository
checkout, Python packages, Go/Node toolchain or source build is required. The
unchanged VPP gate still needs normal Debian/Ubuntu system tools: Python **3.12
or newer** at `/usr/bin/python3`, Bash, dpkg/dpkg-deb, APT, Git, patch and coreutils
(plus ordinary awk/grep/sed/find utilities); shellcheck is used if installed.
Git is needed for VPP's scratch unit tests, not to fetch or access a checkout.
Do not omit dependencies or skip the VPP tests to make a target appear supported.

After authenticating the launcher and checking the package tar against its
trusted transport report before extraction, run the read-only preflight from
any directory:

```sh
env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin LC_ALL=C /usr/bin/python3 -I /received/recipient.py /restored/delivery \
  --manifest /trusted/bundle-manifest.json \
  --helpers /received/ngfw-helpers.tar \
  --helper-report /received/helper-report.json \
  --helper-report-sha256 EXPECTED_SHA256_FROM_AUTHENTICATED_CHANNEL
```

The bootstrap checks the report against that external digest before parsing it,
then checks the complete helper archive's size/hash before parsing the tar. Every
member must match the independently authenticated inventory in name, bytes, size
and mode; omitted/extra/duplicate members, links, special files and unsafe paths
are refused. Helpers are bounded to 128 files, 1 MiB each, 8 MiB expanded and a
10 MiB tar. Extraction uses a private temporary tree under `/var/tmp`, with
0600 files/0700 executables and directories. Only after all members pass does it
run the canonical installer with isolated Python, fixed system PATH and a minimal
environment. The installer's VPP verification retains its full tests and install
gate. A fixed `/nonexistent` HOME satisfies those path-guard tests without using
the caller's home or Git startup configuration. Temporary helper files are removed
on success or failure. The runtime manifest and helper report must be outside the
delivery directory. An unknown helper in the package directory is never executed.

For an explicitly authorized fresh Ubuntu 26.04 amd64 target, the same authenticated
launcher accepts `--install`; run the command with `sudo env -i ...` and add that
flag. All privileged mutation, APT isolation, simulation, failure and release
limitations described above still apply. No target installation has been validated
by this helper-delivery change.

The outside-checkout tests build actual small synthetic Debian archives and run
the **unchanged real full VPP verifier and its 66 tests** from the delivered helper
tree, without stubbing that gate. They cover successful read-only preflight and
refusal of modified helper archives/reports, changed runtime manifests, missing
dependencies, unsafe members and oversized inputs. Synthetic manifests agreeing
with pinned source metadata do not prove that their empty payloads were built
from that upstream source. Real release build provenance, signatures, clean Ubuntu
install/remove/reinstall, firstboot and hardware acceptance remain outstanding;
this closes the bounded recipient checkout gap, not all of P10.

برای دریافت‌کنندهٔ بدون مخزن کد، **پیش از اجرای `recipient.py`** هش آن را با
مقدار مورد انتظارِ دریافت‌شده از یک کانال مستقل و احرازشده مقایسه کنید. هش یا
گزارشِ همراه فایل ناشناس، اصالت آن را ثابت نمی‌کند. مقدار
`--helper-report-sha256` نیز باید از همان مسیر مطمئن به دست آید؛ راه‌انداز پیش
از تجزیهٔ گزارش، این هش را بررسی می‌کند و پیش از اجرای کمک‌ابزارها، هش آرشیو و
همهٔ فایل‌های آن را می‌سنجد. مانیفست زمان‌اجرا (`--manifest`) و گزارش کمک‌ابزار
(`--helper-report`) باید **بیرون از دایرکتوری تحویل بسته‌ها** باشند. پیش از
استخراج بستهٔ منتقل‌شده با ابزار استاندارد `tar`، هش و اندازهٔ آن را با گزارش
انتقالِ دریافت‌شده از کانال مطمئن تطبیق دهید.

پیش‌نیازها همان ابزارهای سیستمِ ذکرشده در بالا هستند: Python **۳٫۱۲ یا جدیدتر**
در `/usr/bin/python3`، Bash، `dpkg`/`dpkg-deb`، APT، Git، `patch`، `coreutils` و
ابزارهای متنی معمول؛ `shellcheck` در صورت نصب‌شدن استفاده می‌شود. مخزن کد،
زنجیرهٔ ساخت Go/Node یا بسته‌های اضافی Python لازم نیست؛ وابستگی‌ها یا آزمون‌های
VPP را برای عبور ظاهری از بررسی حذف نکنید.

فرمان استاندارد بالا در حالت پیش‌فرض فقط بررسیِ پیش از نصب و چاپ طرح را انجام
می‌دهد و **هیچ بسته‌ای نصب نمی‌کند**. نصب واقعی فقط با مجوز صریح، دسترسی root و
گزینهٔ `--install` روی هدف تازهٔ Ubuntu 26.04 amd64 درخواست می‌شود؛ همان فرمان و
محدودیت‌های بخش نصب را رعایت کنید. سیاست امضا و توزیع راه‌انداز، اثبات منشأ
بسته‌های واقعی و پذیرش نصب روی هدف پاک و سخت‌افزار همچنان باز هستند؛ این تغییر
به معنای تکمیل همهٔ P10 نیست.
