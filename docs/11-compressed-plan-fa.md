# طرح فشردهٔ ۲۱ روزه — همهٔ قابلیت‌های نرم‌افزاری، روی VM، بدون آزمایشگاه

این سند جای [`10-30-day-agent-plan-fa.md`](10-30-day-agent-plan-fa.md) را می‌گیرد و
سه تغییر اصلی شما را اعمال می‌کند:

| تغییر | قبل | حالا |
|---|---|---|
| محیط | Docker | **VM (QEMU/KVM + libvirt)** با DPDK روی virtio — همان مسیر سخت‌افزار |
| اعتبارسنجی | آزمایشگاه، TRex، soak، گواهی‌نامه | **فقط تأیید پیکربندی روی VPP** (`Retrieve` = desired و `vppctl show`)؛ گذردهی بعد از هفتهٔ ۳ و فقط اگر کد VPP تغییر کند |
| تست | ۹ شرط DoD برای هر قابلیت | unit فقط برای هستهٔ تراکنشی؛ بقیه یک چک پیکربندی؛ E2E فقط مسیر ورود→commit |
| دامنه | Tier-1 + بخشی از T2 | **هر قابلیتی که با پیکربندی VPP/FRR/strongSwan/Kea قابل انجام است** — T1+T2+T3 |
| کد VPP | مخلوط | **ترک جدا** (بخش ۴) — طرح اصلی هیچ خط C در VPP ندارد |

---

## ۱. اعداد

| | |
|---|---|
| مدت | **۲۱ روز تقویمی** (+ هفتهٔ ۴ اختیاری برای گذردهی) |
| ایجنت موازی | ۱۲ تا ۱۸ در اوج (روز ۳–۱۵) |
| انسان | ۲ نفر: یکپارچه‌ساز/معمار + مهندس شبکه برای راستی‌آزمایی روی VPP |
| اقلام WBS پوشش‌داده‌شده | **۸۷ از ۱۰۲** کامل، **۶** جزئی، **۹** خارج (جدول بخش ۳) |
| کد C در VPP | **صفر** در این ۲۱ روز — ۶ قلم به ترک جدا |

محدودکنندهٔ واقعی زمان دیگر کدنویسی نیست؛ **توان بازبینی و merge دو انسان** است.
اگر یک نفر هستید، ۲۸ روز بگذارید؛ ۱۸ ایجنت با یک بازبین یعنی merge نخوانده.

---

## ۲. زمان‌بندی روزانه

### فاز ۱ — قرارداد و هسته (روز ۱–۶)

| روز | لِین‌های موازی | خروجی روز |
|---|---|---|
| **۱** | P01 اسکلت · P04 محیط VM · P02 schema ×۳ ایجنت (تقسیم دامنه: زیرساخت/L2/L3 — NAT/ACL/اشیا — VPN/سرویس/مسیریابی/عملیات/سیستم) · P03 proto ×۲ · P09 CI-lite | VM ها بالا، VPP 26.06 با DPDK روی virtio، پیش‌نویس کامل schema برای **همهٔ** دامنه‌ها |
| **۲** | ادامهٔ P02/P03 · بازبینی انسانی قرارداد · P05/P06/P07 شروع روی پیش‌نویس | **قفل قرارداد پایان روز ۲** · تصمیم چنداجاره‌ای (D10.1) همین روز |
| **۳–۵** | P05 هستهٔ agent · P06 هستهٔ API · P07 پوستهٔ UI · **کارخانهٔ descriptor** ×۸ ایجنت (هر ایجنت چند پلاگین VPP: interface/l2/bond · ip/ip_neighbor/urpf/abf · nat44_ed/ei/64/66/det44/map/cnat · acl/macip · ipsec/ikev2/wireguard · gre/ipip/vxlan/gpe/gtpu/l2tp/pppoe/sr/lisp · policer/qos/lb/span/lldp/bfd/vrrp/igmp/mpls · dhcp/dns/flowprobe/sflow/prom/pcap) · **کارخانهٔ renderer** ×۴ (frr · strongSwan · kea+unbound+chrony · snmpd+keepalived+rsyslog) | descriptor ها با `Retrieve` علیه binapi نوشته شده‌اند ولی هنوز به هم وصل نیستند |
| **۶** | P08 اسلایس عمودی · یکپارچه‌ساز | پینگ از VPP، شمارندهٔ زنده، commit/rollback، `kill -9 vpp` → بازسازی |

### فاز ۲ — بلیتز قابلیت‌ها (روز ۷–۱۵) — هر موج ۳ روز، ۱۲ لِین

| موج | روز | لِین‌ها (هر لِین = یک `FEATURE-TEMPLATE`) |
|---|---|---|
| **A** | ۷–۹ | vlan-qinq · bonding · bridge/L2XC/L3XC · loopback-bvi-gso-lldp-span · vrf-static-ecmp · neighbors-ra · rpf-adl-pbr · object-model · acl · host-acl-nftables · nat44-ed+sessions · nat44-ei/64/66/nptv6 |
| **B** | ۱۰–۱۲ | det44-map-dslite-cnat · strongswan-build+ipsec-s2s · ikev2-native · wireguard · tunnels(gre/ipip/vxlan/gpe/gtpu/l2tp/pppoe) · pki · frr-linuxcp+bgp · ospfv2/v3 · isis-rip · bfd-redistribution · kea-dhcp+relay · unbound+chrony+syslog |
| **C** | ۱۳–۱۵ | mpls-static+sr-mpls · igmp-mfib-static · srv6+service-chaining · lisp · host-stack-exposure · lb · qos-flat(policer/mark) · snmp · ipfix-sflow · dashboard+prometheus+alarms · capture-trace · vrrp+config-sync |

### فاز ۳ — سیستم، بسته‌بندی، تثبیت (روز ۱۶–۲۱)

| روز | لِین‌ها |
|---|---|
| **۱۶–۱۷** | cli · restconf-yang · terraform-ansible-sdk · aaa-external · tenants(اگر روز ۲ تصویب شد) · backup-restore+revisions-ui · ra-vpn · session-browser-scale |
| **۱۸** | deb-packaging · iso-installer · ab-upgrade · vm/cloud-images · hardening-lite(nftables/systemd/signing) · licensing-simple |
| **۱۹–۲۰** | **freeze — فقط رفع باگ** · E2E روی توپولوژی VM (۳ روتر + همتای FRR + همتای strongSwan) · بازبین امنیتی روی کل کد |
| **۲۱** | دمو · گزارش «داریم / نداریم» تولیدشده توسط ایجنت از وضعیت واقعی CI · تصمیم برای هفتهٔ ۴ |

### هفتهٔ ۴ (اختیاری، فقط اگر لازم شد)
گذردهی روی یک VM با DPDK و — اگر سخت‌افزار رسید — روی NIC واقعی. **فقط sanity**، نه بنچمارک.
شرط اجرا: یا کد VPP تغییر کرده (ترک بخش ۴) یا startup.conf روی سخت‌افزار واقعی تولید شده.

---

## ۳. وضعیت دقیق هر ۱۰۲ قلم WBS

نشانه‌ها: ✅ در طرح · 🟡 جزئی (بخش پیکربندی داخل، بخش دیگر خارج) · 🔧 نیاز به کد VPP (ترک جدا) · ⏳ اعتبارسنجی به تعویق (کد هست، صحت روی سخت‌افزار تأیید نشده) · ❌ خارج از طرح

| قلم | وضعیت | روز | توضیح |
|---|---|---|---|
| D0.1 monorepo/CI/codegen | ✅ | ۱ | |
| D0.2 VPP پین‌شده | ✅ | ۱ | از مخزن `fdio/2606`، **بدون فورک**؛ فورک فقط اگر ترک 🔧 شروع شود |
| D0.3 ایمیج OS/ISO | ✅ | ۱۸ | autoinstall |
| D0.4 deb/APT | ✅ | ۱۸ | |
| D0.5 ارتقای A/B | ✅ | ۱۸ | روی VM تست می‌شود؛ روی سخت‌افزار ⏳ |
| D0.6 مولد startup.conf | ⏳ | ۳ | کد کامل؛ مقادیر NUMA/RSS/hugepage روی سخت‌افزار واقعی تأیید نشده |
| D0.7 هستهٔ agent | ✅ | ۳–۵ | |
| D0.8 هستهٔ API | ✅ | ۳–۵ | |
| D0.9 auth/RBAC/audit/TPM | 🟡 | ۳–۵ | همه ✅ جز TPM (روی VM با TPM مجازی swtpm تست می‌شود، سخت‌افزار ⏳) |
| D0.10 پوستهٔ UI | ✅ | ۳–۵ | |
| D0.11 CLI | ✅ | ۱۶ | |
| D0.12 تلمتری | ✅ | ۵ | |
| D0.13 زیرساخت تست | 🟡 | ۱، ۱۹ | توپولوژی VM + smoke ✅ · Robot/TRex/Playwright کامل ❌ |
| D1.1 موجودی NIC و bind درایور | 🟡 | ۷ | نرم‌افزار ✅ · ماتریس درایور واقعی (E810/CX6/mGig) ⏳ سخت‌افزار |
| D1.2 پایهٔ اینترفیس | ✅ | ۶ | |
| D1.3 آدرس‌دهی/unnumbered | ✅ | ۶ | |
| D1.4 VLAN/QinQ | ✅ | ۷ | |
| D1.5 bonding/LACP | ✅ | ۷ | LACP روی virtio تست می‌شود؛ رفتار با سوییچ واقعی ⏳ |
| D1.6 L2 | ✅ | ۷ | |
| D1.7 LLDP | ✅ | ۷ | |
| D1.8 GSO/offload | ⏳ | ۷ | پیکربندی ✅؛ اثر واقعی offload فقط روی NIC فیزیکی |
| D1.9 nsim | ✅ | ۷ | |
| D1.10 SPAN/ERSPAN | ✅ | ۷ | |
| D1.11 loopback/BVI | ✅ | ۷ | |
| D2.1 VRF | ✅ | ۷ | |
| D2.2 static/ECMP | ✅ | ۷ | |
| D2.3 ARP/ND/RA/DAD | ✅ | ۷ | |
| D2.4 RPF/ADL | ✅ | ۷ | |
| D2.5 مرورگر FIB | ✅ | ۷ | با داده‌های مصنوعی ۱M تست می‌شود |
| D2.6 ابزارها | ✅ | ۶ | |
| D2.7 classifier/PBR | ✅ | ۷ | پلاگین `abf` |
| D2.8 MPLS/SR-MPLS | 🟡 | ۱۳ | MPLS ایستا و SR-MPLS ✅ · **LDP دینامیک 🔧 V5** (linux-nl مسیر MPLS کرنل را همگام نمی‌کند) |
| D2.9 Multicast/BIER | 🟡 | ۱۳ | IGMP، mfib ایستا، BIER ✅ · **PIM دینامیک 🔧 V5** |
| D3.1 linux-cp | 🟡 | ۱۰ | پیکربندی و چرخهٔ عمر ✅ · **پچ‌های لبه‌ای 🔧 V1** · جدول کامل ⏳ |
| D3.2 چارچوب FRR | ✅ | ۱۰ | |
| D3.3 BGP | ✅ | ۱۰ | همهٔ گزینه‌ها؛ همگرایی ۱M مسیر ⏳ |
| D3.4 OSPFv2 | ✅ | ۱۰ | |
| D3.5 OSPFv3 | ✅ | ۱۰ | |
| D3.6 IS-IS | ✅ | ۱۰ | |
| D3.7 RIP | ✅ | ۱۰ | |
| D3.8 BFD | ✅ | ۱۰ | |
| D3.9 PBR در API/UI | ✅ | ۷ | |
| D3.10 redistribution | ✅ | ۱۰ | |
| D4.1 NAT44-ED | 🟡 | ۷ | همه ✅ · **ALG های اضافه (SIP/FTP) 🔧 V4** |
| D4.2 NAT44-EI | ✅ | ۷ | |
| D4.3 NAT64/66/NPTv6 | ✅ | ۷ | |
| D4.4 DET44 | ✅ | ۱۰ | |
| D4.5 DS-Lite/MAP/LW4o6/464XLAT | ✅ | ۱۰ | |
| D4.6 CNAT | ✅ | ۱۰ | |
| D4.7 مرورگر session | ✅ | ۷، ۱۶ | مقیاس ۱M با دادهٔ مصنوعی |
| D5.1 مدل اشیا | ✅ | ۷ | FQDN و schedule در agent حل می‌شود، نه VPP |
| D5.2 ACL | ✅ | ۷ | |
| D5.3 host ACL/nftables | ✅ | ۷ | |
| D5.4 ویرایشگر ۱۰۰k | ✅ | ۷ | |
| D5.5 ADL/Auto-SDL | ✅ | ۷ | |
| D5.6 state-sync سیاست | 🔧 | — | **V2** — VPP پلاگین ندارد؛ بخش کنترلی ✅ روز ۱۳ |
| D6.1 هستهٔ IPsec | 🟡 | ۱۰ | همه ✅ · QAT ⏳ سخت‌افزار |
| D6.2 strongSwan+VPP | ✅ | ۱۰ | بیلد `vpp_sswan` (کد C علیه VPP، نه در VPP) · پچ احتمالی 🔧 V6 |
| D6.3 IKEv2 بومی | ✅ | ۱۰ | |
| D6.4 PKI | 🟡 | ۱۰ | همه ✅ · HSM/PKCS#11 ⏳ سخت‌افزار |
| D6.5 WireGuard | ✅ | ۱۰ | |
| D6.6 تونل‌ها | ✅ | ۱۰ | همهٔ انواع |
| D6.7 SRv6 | ✅ | ۱۳ | |
| D6.8 LISP | ✅ | ۱۳ | |
| D6.9 RA-VPN | ✅ | ۱۶ | |
| D6.10 داشبورد تونل | ✅ | ۱۰ | |
| D7.1 Kea | ✅ | ۱۰ | |
| D7.2 DHCP relay/client | ✅ | ۱۰ | |
| D7.3 DNS | ✅ | ۱۰ | |
| D7.4 NTP | ✅ | ۱۰ | |
| D7.5 SNMP | ✅ | ۱۳ | |
| D7.6 IPFIX/sFlow | ✅ | ۱۳ | |
| D7.7 syslog | ✅ | ۱۰ | |
| D7.8 QoS | 🟡 | ۱۳ | policer، marking، DSCP/dot1p ✅ · **HQoS سلسله‌مراتبی 🔧 V3** |
| D7.9 Load Balancer | ✅ | ۱۳ | |
| D7.10 Host Stack | ✅ | ۱۳ | فقط عرضهٔ پیکربندی قابلیت‌های موجود |
| D7.11 PG/pcap | ✅ | ۱۳ | |
| D8.1 داشبورد | ✅ | ۱۳ | |
| D8.2 ضبط/trace | ✅ | ۱۳ | |
| D8.3 Prometheus | ✅ | ۱۳ | |
| D8.4 هشدار | ✅ | ۱۳ | |
| D8.5 backup/restore | ✅ | ۱۶ | |
| D8.6 support bundle | ✅ | ۱۸ | |
| D8.7 UI ارتقا | ✅ | ۱۸ | |
| D8.8 RESTCONF/YANG | ✅ | ۱۶ | YANG از schema تولید می‌شود |
| D8.9 Terraform/Ansible/SDK | ✅ | ۱۶ | |
| D9.1 VRRP | ✅ | ۱۳ | |
| D9.2 config sync | ✅ | ۱۳ | |
| D9.3 state sync | 🔧 | — | **V2** |
| D9.4 تست failover | 🟡 | ۱۹ | کارکردی روی VM ✅ · زمان‌سنجی < ۱s ⏳ |
| D9.5 UI خوشه | ✅ | ۱۳ | |
| D10.1 چنداجاره‌ای | ✅/❌ | ۱۶ | **تصمیم روز ۲** — مدل داده را عوض می‌کند؛ بعداً اضافه‌کردنش ۳× گران‌تر است |
| D10.2 AAA خارجی | ✅ | ۱۶ | |
| D11.1 ایمیج ابری | 🟡 | ۱۸ | ساخت ایمیج ✅ · انتشار در marketplace ❌ |
| D11.2 ایمیج VM | ✅ | ۱۸ | همان محیط توسعه |
| D12.1 سخت‌سازی | 🟡 | ۱۸ | nftables، systemd hardening، امضای بسته ✅ · CIS کامل و secure boot ❌ |
| D12.2 مجوز | ✅ | ۱۸ | ساده |
| D12.3 تست نفوذ | ❌ | — | |
| D12.4 مستندات | 🟡 | ۲۱ | تولیدشده توسط ایجنت از کد ✅ · ۴۰۰ صفحهٔ ویراسته ❌ |
| D12.5 QA/interop | ❌ | — | فقط smoke و توپولوژی VM |
| D12.6 soak/chaos | ❌ | — | `kill -9 vpp` در CI ✅، ۷۲ ساعت ❌ |
| D12.7 گواهی‌نامه | ❌ | — | |

**خلاصه:** ۸۷ ✅ · ۶ 🟡 با بخش 🔧 · ۹ ❌/⏳ خالص (D12.3، D12.5، D12.6، D12.7، بخش سخت‌افزاری D1.1/D1.8/D0.6، marketplace، CIS/secure-boot).

---

## ۴. ترک جدای کد VPP (C) — خارج از ۲۱ روز

هیچ‌کدام برای دمو یا PoC لازم نیست. هر یک با یک مهندس VPP و ایجنت کمکی، **موازی** با طرح اصلی.

| # | قلم | چرا کد لازم است | حجم | راه‌حل بدون کد (فعلاً) |
|---|---|---|---|---|
| **V1** | پچ‌های `linux-cp`/`linux-nl` | لبه‌های شناخته‌شده: IPv6 RA روی جفت، bond sub-if، نگاشت VRF چندگانه، churn جدول کامل | ۲–۴ هفته، نامعلوم | با پیکربندی پیش‌فرض کار می‌کند؛ لبه‌ها را مستند می‌کنیم |
| **V2** | state-sync برای HA (NAT44-ED session، ACL reflexive) | VPP برای ED پلاگین HA ندارد (فقط EI دارد `nat44_ei_ha`) | ۴–۶ هفته | VRRP بدون حفظ session؛ یا NAT44-EI با HA بومی |
| **V3** | HQoS سلسله‌مراتبی | scheduler سلسله‌مراتبی DPDK از VPP حذف شده | ۴–۸ هفته | policer + marking + صف‌بندی تخت |
| **V4** | ALG های NAT (SIP، FTP فعال، PPTP) | nat44-ed فقط ALG محدود دارد | ۲–۳ هفته هر ALG | مستند «پشتیبانی نمی‌شود» |
| **V5** | همگام‌سازی مسیر MPLS و mroute کرنل→VPP | linux-nl فقط unicast v4/v6 را همگام می‌کند | ۳–۵ هفته | agent مسیرهای LDP/PIM را از FRR (JSON) می‌خواند و خودش با API در VPP می‌نشاند — **کنترل‌پلین، بدون کد VPP** — این را روز ۱۳ به‌جای کد VPP می‌سازیم |
| **V6** | پچ `vpp_sswan` (kernel-vpp) | اگر با 26.06 و strongSwan 6.x ناسازگاری داشت | ۰–۲ هفته | نسخهٔ سازگار را پین می‌کنیم |

قاعدهٔ طرح: **اگر ایجنتی به جایی رسید که فکر کرد باید در VPP کد بزند، متوقف می‌شود و قلم را
به این جدول اضافه می‌کند.** هیچ C در `apps/` نوشته نمی‌شود.

---

## ۵. سیاست تست فشرده

| لایه | چه تستی | چه تستی نه |
|---|---|---|
| `packages/schema` | unit کامل روی primitives و اعتبارسنج‌های معنایی | — |
| scheduler/reconciler | unit: ترتیب، rollback، idempotency | — |
| موتور commit | e2e روی Postgres واقعی: commit/rollback/confirmed | — |
| **هر قابلیت** | **یک چک:** بعد از commit، `Retrieve()` = desired **و** خروجی `vppctl show <x>` شامل پیکربندی است؛ بعد از rollback هیچ | تست سطح بسته (جز ۵ مورد: اسلایس عمودی، NAT، IPsec، BGP→FIB، VRRP) |
| UI | Playwright فقط ورود → ویرایش → commit → rollback | تست هر صفحه |
| `kill -9 vpp` | در CI شبانه روی VM | — |
| گذردهی | ❌ تا هفتهٔ ۴ | |

این کافی است برای «پیکربندی روی VPP درست نشسته». کافی نیست برای «تحت بار درست کار می‌کند» —
و شما همین را خواسته‌اید.

---

## ۶. محیط VM

| جزء | انتخاب |
|---|---|
| هایپروایزر | QEMU/KVM + libvirt روی یک سرور توسعه (۳۲+ هسته، ۶۴+ GB) — هر ایجنت یک VM اختصاصی برای smoke و یک توپولوژی مشترک شبانه |
| ایمیج پایه | Ubuntu 24.04 cloud image + cloud-init nocloud؛ VPP 26.06 از `fdio/2606`؛ همان بسته‌های `scripts/10-install-runtime.sh` |
| دیتاپلین در VM | **DPDK روی virtio-pci با `uio_pci_generic`** (یا vfio-noiommu) + ۲ hugepage یک‌گیگی — همان مسیر کد سخت‌افزار؛ روز انتقال فقط PCI whitelist و درایور عوض می‌شود |
| توپولوژی | شبکه‌های ایزولهٔ libvirt = سگمنت L2: `mgmt`، `lan`، `wan`، `dmz`، `p2p-ab`، `p2p-bc` |
| VM ها | `vrx-a` `vrx-b` `vrx-c` (روتر) · `host-lan` `host-wan` (scapy/iperf3) · `peer-frr` · `peer-sswan` |
| ابزار | `tools/lab` (bash/Go): `up`, `down`, `snapshot`, `restore`, `restart-vpp <vm>`, `ssh <vm>`, `vppctl <vm> …`, `topology <name>` |
| CI | همان `tools/lab` روی runner با KVM تودرتو یا سرور فیزیکی CI |

انتقال به سخت‌افزار = همان ایمیج ISO روی سرور فیزیکی + مولد startup.conf با PCI واقعی. هیچ
مسیر کدی «مخصوص VM» وجود ندارد.

---

## ۷. فرض‌ها و ریسک‌های این فشردگی

1. **دو انسان تمام‌وقت** برای بازبینی و merge؛ کمتر از این = زمان بیشتر، نه ایجنت بیشتر.
2. **قرارداد در روز ۲ قفل می‌شود** حتی اگر ناقص باشد؛ کمبودها با PR برچسب `contract` و تأیید انسانی.
3. **کیفیت = «پیکربندی درست روی VPP»**، نه پایداری تحت بار. باگ‌های همزمانی و نشتی در روز ۲۱ کشف نشده‌اند.
4. **سرور توسعهٔ قوی از روز ۱** (۳۲ هسته، ۶۴ GB، KVM) — بدون آن کارخانهٔ descriptor موازی نمی‌شود.
5. هزینهٔ توکن ایجنت‌ها در این ۲۱ روز قابل‌توجه است؛ برای ۱۵ لِین × ۱۵ روز بودجه بگذارید.
6. پس از روز ۲۱، هرچه ⏳ است روی سخت‌افزار خودش را نشان می‌دهد — به‌خصوص D0.6، D1.1، D3.1.
