# بسته‌های موردنیاز سیستم‌عامل

سیستم پایه: **Ubuntu 26.04.x LTS (resolute)**، کرنل **7.0**، معماری x86_64.

سه پروفایل نصب داریم و هیچ‌وقت نباید با هم قاطی شوند:

| پروفایل | کجا | اسکریپت |
|---|---|---|
| **Runtime** — روی خود دستگاه (appliance) | ایمیج محصول | [`scripts/10-install-runtime.sh`](../scripts/10-install-runtime.sh) |
| **Build & Dev** — ماشین توسعه و سرور بیلد | CI و لپ‌تاپ مهندس | [`scripts/20-install-build.sh`](../scripts/20-install-build.sh) |
| **Lab & Test** — ماشین تست و تولید ترافیک | آزمایشگاه | [`scripts/40-install-lab.sh`](../scripts/40-install-lab.sh) |

روی دستگاه محصول **فقط پروفایل Runtime** نصب می‌شود. کامپایلر، هدر، پایتون توسعه و ابزار
دیباگ روی appliance یعنی سطح حملهٔ اضافه و ردشدن در ممیزی امنیتی.

---

## ۱. مخازن شخص ثالث

| مخزن | آدرس | چه می‌دهد | وضعیت |
|---|---|---|---|
| **FD.io** | `packagecloud.io/fdio/release` | vpp و پلاگین‌ها | تأییدشده — مخزن هر انتشار هم جداست (مثلاً `fdio/2606`) |
| **FRRouting** | `deb.frrouting.org/frr` با `$(lsb_release -s -c)` و `frr-stable` | frr، frr-pythontools | resolute خیلی تازه است (فروردین ۱۴۰۵)؛ اگر مخزن هنوز سوییت `resolute` را منتشر نکرده باشد، بستهٔ `frr` خود آرشیو اوبونتو را جایگزین کنید |
| **NodeSource** | `deb.nodesource.com/node_22.x` | Node.js 22 LTS | resolute خودش Node.js 22.x را دارد (کافی است)؛ NodeSource فقط برای پین دقیق نسخه و بروزرسانی مستقل از چرخهٔ اوبونتو نگه داشته می‌شود |
| **PostgreSQL PGDG** | `apt.postgresql.org` | postgresql-16/17/18 | اختیاری؛ resolute خودش postgresql-18 دارد |
| **ISC Kea** | Cloudsmith (`dl.cloudsmith.io/public/isc/kea-<ver>`) | kea 2.6/3.x | resolute خودش kea 3.0.x دارد؛ نیازی به این مخزن نیست |
| **Valkey** | مخزن رسمی Valkey | valkey-server | resolute به‌صورت پیش‌فرض valkey-server دارد؛ نیازی به این مخزن نیست |

> **نکتهٔ مجوز:** در resolute بستهٔ `redis-server` صرفاً یک متاپکیج انتقالی است که
> `valkey-server` (مجوز BSD-3-Clause) را نصب می‌کند. برای محصول تجاری مستقیماً
> `valkey-server` را در لیست پکیج‌ها بیاورید تا وابستگی انتقالی غیرضروری نداشته باشید.

---

## ۲. پروفایل Runtime (روی دستگاه)

### ۲.۱ Data plane

| بسته | نقش |
|---|---|
| `vpp` | خود VPP |
| `vpp-plugin-core` | پلاگین‌های اصلی: acl، nat، ikev2، wireguard، linux-cp، gtpu، vxlan، srv6، lb، prom، sflow، … |
| `vpp-plugin-dpdk` | درایورهای DPDK |
| `libvppinfra` `vpp-lib` | کتابخانه‌های مشترک (وابستگی خودکار) |
| _(جزو `linux-modules-$(uname -r)-generic` است)_ | `vfio-pci`، `uio_pci_generic` — در resolute دیگر پکیج جدای `linux-modules-extra` برای flavor عمومی وجود ندارد، این ماژول‌ها از قبل داخل خود کرنل هستند |
| `rdma-core` `libibverbs1` `ibverbs-providers` | مسیر درایور Mellanox ConnectX (پلاگین rdma در VPP) |
| `pciutils` `ethtool` `driverctl` | شناسایی و bind کارت شبکه |
| `numactl` `hwloc` | پین‌کردن worker روی NUMA |
| `libnuma1` `libssl3` `libcrypto++` | وابستگی‌های رمزنگاری/حافظه |

بستهٔ `vpp-dbg` را فقط روی ایمیج دیباگ نصب کنید، نه روی محصول.

### ۲.۲ مسیریابی

| بسته | نقش |
|---|---|
| `frr` | bgpd، ospfd، ospf6d، isisd، ripd، bfdd، pimd، staticd، zebra |
| `frr-pythontools` | `frr-reload.py` — برای اعمال امن پیکربندی بدون restart **الزامی است** |

### ۲.۳ VPN و رمزنگاری

| بسته | نقش |
|---|---|
| **strongSwan (از سورس)** | نسخهٔ مخزن resolute پلاگین `kernel-vpp` و `socket-vpp` را **ندارد**. باید با `--enable-kernel-vpp --enable-socket-vpp` کامپایل و به‌صورت `.deb` اختصاصی بسته‌بندی شود |
| `libstrongswan-extra-plugins` `libcharon-extra-plugins` | در صورت استفاده از strongSwan مخزن (مسیر بدون VPP) |
| `wireguard-tools` | فقط ابزار `wg`؛ مسیر داده در پلاگین VPP است، نه ماژول کرنل |
| `openssl` `libssl3` | PKI |
| `tpm2-tools` `libtss2-esys-3.0.2-0` `libtss2-tctildr0` | نگهداری کلید ریشه در TPM |
| `opensc` `libengine-pkcs11-openssl` | HSM/PKCS#11 (اختیاری، فاز D6.4) |

### ۲.۴ سرویس‌های شبکه

| بسته | نقش |
|---|---|
| `kea-dhcp4-server` `kea-dhcp6-server` `kea-ctrl-agent` | DHCP + API کنترلی |
| `unbound` `dns-root-data` | resolver/forwarder + DNSSEC |
| `chrony` | NTP |
| `snmpd` `snmp` `libsnmp-base` | SNMP v2c/v3 |
| `keepalived` | VRRP (مسیر جایگزین پلاگین بومی VPP) |
| `nftables` | فایروال صفحهٔ مدیریت میزبان |
| `lldpd` | فقط اگر پیاده‌سازی LLDP خود VPP کافی نبود |

### ۲.۵ صفحهٔ کنترل

| بسته | نقش |
|---|---|
| `nodejs` (22.x از NodeSource) | `vrx-api` |
| `postgresql-18` `postgresql-client-18` | مخزن پیکربندی، revision، audit |
| `valkey-server` | کش، pub/sub، صف (پیش‌فرض توزیع resolute؛ `redis-server` فقط متاپکیج انتقالی است) |
| `nginx` | TLS، سرو SPA، پراکسی `/api` |
| `ca-certificates` `openssl` | زنجیرهٔ اعتماد |

`vrx-agent` یک باینری استاتیک Go است و بستهٔ سیستمی نمی‌خواهد.

### ۲.۶ رصد، لاگ و ابزار عملیاتی

| بسته | نقش |
|---|---|
| `prometheus-node-exporter` | متریک میزبان (متریک VPP از پلاگین `prom` می‌آید) |
| `rsyslog` `logrotate` | لاگ محلی و صدور syslog |
| `tcpdump` | ضبط روی اینترفیس‌های مدیریتی (مسیر داده با pcap خود VPP) |
| `iproute2` `iputils-ping` `traceroute` `mtr-tiny` | ابزار پایه |
| `jq` `curl` `gnupg` `unzip` `rsync` | ابزار اسکریپت و ارتقا |
| `smartmontools` `lm-sensors` `ipmitool` | سلامت سخت‌افزار برای داشبورد |
| `dmidecode` | شناسایی مدل دستگاه برای مجوز |

---

## ۳. آنچه نباید روی دستگاه باشد

| بسته/سرویس | چرا |
|---|---|
| `NetworkManager` | با اینترفیس‌های مدیریت‌شده تداخل می‌کند — `apt purge` |
| `systemd-networkd` روی NIC های دیتاپلین | کارت باید به `vfio-pci` بایند شود، نه به کرنل |
| `irqbalance` | وقفه‌ها را از روی هستهٔ ایزوله جابه‌جا می‌کند و jitter می‌سازد — `systemctl disable --now` |
| `ufw` | ما `nftables` را مستقیم مدیریت می‌کنیم |
| `unattended-upgrades` | ارتقا فقط از طریق مکانیزم A/B خودمان — `apt purge` |
| `cloud-init` | فقط روی ایمیج‌های ابری بماند، نه ایمیج bare-metal |
| `snapd` | حجم و سطح حمله بی‌دلیل — `apt purge` |
| `gcc` `make` `*-dev` | روی محصول نهایی هیچ کامپایلری نباید باشد |

---

## ۴. تنظیمات کرنل و بوت

خط فرمان GRUB (مثال برای سرور ۱۶ هسته‌ای که ۸ هسته به VPP می‌دهد):

```
default_hugepagesz=1G hugepagesz=1G hugepages=16 \
intel_iommu=on iommu=pt \
isolcpus=2-9 nohz_full=2-9 rcu_nocbs=2-9 \
intel_pstate=disable processor.max_cstate=1 intel_idle.max_cstate=0 \
pcie_aspm=off mce=ignore_ce
```

> این مقادیر باید توسط **مولد `startup.conf`** (قلم کار D0.6) از روی موجودی سخت‌افزار
> تولید شود، نه دستی. عدد hugepage و بازهٔ isolcpus به تعداد worker و NUMA بستگی دارد.

`sysctl`:
```
vm.nr_hugepages            (توسط hugepagesz بوت تعیین می‌شود)
vm.max_map_count = 3096000
vm.overcommit_memory = 0
kernel.shmmax = 68719476736
net.core.rmem_max = 67108864
fs.file-max = 2097152
```

ماژول‌ها: `vfio-pci` (ترجیحی)، `vfio_iommu_type1`، `uio_pci_generic` (پشتیبان برای
ماشین مجازی بدون IOMMU). محدودیت‌ها: `memlock unlimited` برای کاربر vpp.

---

## ۵. پروفایل Build & Dev

| گروه | بسته‌ها |
|---|---|
| زنجیرهٔ بیلد VPP | `build-essential` `cmake` `ninja-build` `clang` `lld` `ccache` `python3-dev` `python3-venv` `python3-pip` `libnuma-dev` `libssl-dev` `libelf-dev` `libpcap-dev` `libmnl-dev` `uuid-dev` `nasm` `git` `chrpath` — ولی روش درست: `git clone` سورس VPP و `make install-dep` که خودش همه را نصب می‌کند |
| بیلد strongSwan | `libgmp-dev` `libsystemd-dev` `pkg-config` `gperf` `bison` `flex` `autoconf` `automake` `libtool` |
| Go | `golang-1.23` یا tarball رسمی + `protoc` (`protobuf-compiler`) + `protoc-gen-go` `protoc-gen-go-grpc` |
| Node | `nodejs` 22 + `corepack enable` برای pnpm |
| بسته‌بندی | `devscripts` `debhelper` `dh-make` `dpkg-dev` `fakeroot` `reprepro` `gnupg` `dput` |
| ایمیج‌سازی | `xorriso` `isolinux` `squashfs-tools` `cloud-image-utils` `qemu-utils` `debootstrap` |
| کانتینر | `docker.io` + `docker-compose-v2` (یا `podman`) |
| کیفیت | `shellcheck` `jq` `yq` |

## ۶. پروفایل Lab & Test

| گروه | بسته‌ها |
|---|---|
| مجازی‌سازی | `qemu-kvm` `libvirt-daemon-system` `virtinst` `bridge-utils` `ovmf` |
| توپولوژی | `containerlab` (باینری رسمی) یا `docker.io` + `iproute2` |
| ترافیک | **TRex** (تاربال رسمی سیسکو، بستهٔ apt ندارد) · `iperf3` · `netperf` · `pktgen-dpdk` (از سورس) |
| تحلیل | `tshark` `tcpdump` `wireshark-common` |
| اتوماسیون | `python3-venv` + در venv: `robotframework`، `robotframework-sshlibrary`، `scapy`، `pytest` |
| همتای مسیریابی | `frr` روی کانتینر برای تست BGP/OSPF |

---

## ۷. برآورد دیسک و پارتیشن‌بندی

| بخش | اندازه | توضیح |
|---|---|---|
| EFI | 1 GB | |
| root A | 20 GB | ایمیج فعال |
| root B | 20 GB | ایمیج ارتقا (مکانیزم A/B، قلم D0.5) |
| `/var/log` | 20 GB+ | لاگ و pcap |
| `/var/lib/postgresql` | 20 GB | revision و audit |
| `/data` | باقی دیسک | پشتیبان، بستهٔ به‌روزرسانی، core dump |

حداقل حافظه: **۱۶ GB** (۸ GB برای hugepage + ۸ GB برای سیستم و صفحهٔ کنترل).
برای جدول کامل BGP و یک میلیون session NAT: **۳۲ GB به بالا**.

---

## ۸. نکتهٔ حقوقی

`frr`، `strongswan`، `chrony`، `keepalived` و `nftables` همگی GPL هستند و به‌صورت
**فرآیند جداگانه** اجرا و با فایل پیکربندی/CLI/JSON مدیریت می‌شوند. این «تجمیع صرف» است و
کد صفحهٔ کنترل شما را آلوده نمی‌کند. اگر strongSwan را **تغییر دادید** (که برای پلاگین
`kernel-vpp` محتمل است)، آن تغییرات GPL می‌شوند و باید در اختیار مشتری قرار گیرند —
پچ‌ها را در یک مخزن عمومی جدا نگه دارید و ترجیحاً upstream بفرستید.
