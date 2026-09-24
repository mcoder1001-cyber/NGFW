# زمان‌بندی جامع تولید محصول VRX
## پوشش کامل قابلیت‌های TNSR + تمام قابلیت‌های VPP 26.06

سند مرجع: VPP نسخه **26.06** (انتشار ۲۴ ژوئن ۲۰۲۶، ۲۸ قابلیت جدید، ۶۳۲ کامیت) و شاخه در حال توسعه **26.10-rc0**.
فهرست قابلیت‌های TNSR از مستندات رسمی Netgate استخراج شده است.

---

## ۱. خلاصه مدیریتی

| شاخص | مقدار |
|---|---|
| حجم کار خالص مهندسی | **۵٬۱۸۵ نفر-روز** (≈ ۲۴۷ نفر-ماه ≈ ۲۰.۶ نفر-سال) |
| با سرباره ۲۵٪ (مدیریت، یکپارچه‌سازی، دوباره‌کاری) | **۶٬۴۸۰ نفر-روز** (≈ ۳۰۹ نفر-ماه) |
| سناریوی پیشنهادی | **۳۶ ماه با ۱۴ نفر** (۱۲ مهندس + QA + DevOps) |
| اولین نسخه قابل فروش (MVP) | **ماه ۱۰** |
| اولین GA تجاری (روتر لبه) | **ماه ۱۶** |
| پوشش کامل TNSR | **ماه ۲۷** (با ۲ نفر اضافه در Q7–Q8 یا حذف دامنه: ماه ۲۴) |
| پوشش کامل سطح قابلیت VPP | **ماه ۳۰** |
| GA نهایی با سخت‌سازی و گواهی‌نامه | **ماه ۳۶** |

> **یافتهٔ ظرفیت (پس از متعادل‌سازی ۱۰۲ قلم کار با ظرفیت سه‌ماهه):** هشت قلم T2
> (VPN دسترسی راه دور، AAA، Terraform/Ansible/SDK، IKEv2 بومی، RPF/ADL، syslog و
> مرورگر لاگ، بستهٔ پشتیبانی، ایمیج‌های مجازی) از Q8 عبور می‌کنند و همتایی کامل TNSR
> عملاً پایان Q9 یعنی **ماه ۲۷** تمام می‌شود، نه ماه ۲۴. سه گزینه: (الف) پذیرش ماه ۲۷؛
> (ب) افزودن ۲ نفر در Q7–Q8 (حدود ۱۲۰ هزار دلار) و حفظ ماه ۲۴؛ (ج) انتقال آن هشت قلم
> از GA-2 به R2.0. این تصمیم را همین حالا آگاهانه بگیرید، نه در ماه ۲۳.
>
> بارگذاری سه‌ماهه: ۷۵٪ → ۸۷٪ → ۹۵٪ → ۹۶٪ → ۹۹٪ → ۸۹٪ → ۹۸٪ → **۱۰۲٪** → ۸۰٪ → ۹۹٪ →
> ۶۵٪ → ۲۸٪. تنها Q8 بیش‌تعهد دارد. ذخیرهٔ احتیاطی کل برنامه **۱۵.۳٪** است
> (۶٬۴۸۱ نفر-روز بار در برابر ۷٬۶۴۸ نفر-روز ظرفیت) — تا ماه ۳۰ آن را خرج دامنهٔ جدید نکنید.

> **فایل کاری:** جزئیات تک‌تک ۱۰۲ قلم کار با فرمول‌های زنده در
> [`wbs/VRX-WBS.xlsx`](../wbs/VRX-WBS.xlsx) است (۱۰ شیت، ۶۲۰ فرمول). خروجی‌های
> [Jira](../wbs/VRX-WBS-jira.csv) و [MS Project](../wbs/VRX-WBS-msproject.csv) هم آماده‌اند.
> نسخهٔ انگلیسی این سند: [`08-master-schedule-en.md`](08-master-schedule-en.md).

> **نکته مهم و صریح:** TNSR خودش تمام قابلیت‌های VPP را در رابط مدیریتی عرضه نمی‌کند.
> «همه TNSR + همه VPP» یعنی دامنه‌ای **بزرگ‌تر از خود TNSR**. بخش‌هایی از VPP
> (مثل LISP، BIER، quicly، netmap، OSI، SRv6-mobile) ارزش تجاری تقریباً صفر دارند
> ولی در این برآورد گنجانده شده‌اند. در بخش ۱۱ فهرست دقیق «چه چیزی را حذف کنیم»
> آمده که ۹ ماه از زمان‌بندی را آزاد می‌کند.

---

## ۲. مبانی برآورد

**واحد:** نفر-روز (PD) کار یک مهندس ارشد/میان‌رده. سال کاری مؤثر = ۱۹۰ PD
(پس از کسر تعطیلات، مرخصی، جلسات، پشتیبانی).

**تعریف «انجام‌شده» برای هر قابلیت (۹ شرط):**
۱. Data plane واقعاً کار می‌کند (اثبات با تست سطح بسته، نه سطح API)
۲. وضعیت پس از `systemctl restart vpp` و ریبوت کامل بازسازی می‌شود
۳. Endpoint REST + OpenAPI + کلاینت تولیدشده
۴. چرخه candidate/commit/rollback با مسیرهای خطا
۵. صفحهٔ UI با فرم schema-driven + نمای فهرست + وضعیت زنده
۶. رشته‌های i18n فارسی و انگلیسی
۷. تست unit + integration + E2E
۸. صفحهٔ مستندات کاربر + معادل CLI
۹. ثبت در audit log

**هر برآورد شامل همهٔ ۹ مورد بالا است** — یعنی PD خالص کدنویسی حدود ۴۵٪ عدد است.

---

## ۳. دامنهٔ کامل محصول — فهرست قابلیت با برآورد

سطح‌بندی: **T1** = ضروری برای فروش · **T2** = لازم برای رقابت با TNSR · **T3** = پوشش کامل VPP، ارزش تجاری کم

### D0 — زیرساخت و پلتفرم — ۶۸۵ PD

| # | قابلیت | Tier | PD |
|---|---|---|---|
| D0.1 | Monorepo، CI/CD، خط تولید codegen (Zod→OpenAPI→client→proto→binapi) | T1 | 25 |
| D0.2 | فورک پین‌شدهٔ VPP + مدیریت پچ + بیلد بسته‌های VPP | T1 | 30 |
| D0.3 | ایمیج OS (Ubuntu 24.04 autoinstall) + ISO نصاب | T1 | 45 |
| D0.4 | بسته‌بندی `.deb` + مخزن APT + امضا | T1 | 25 |
| D0.5 | ارتقای A/B image با rollback خودکار | T1 | 40 |
| D0.6 | مولد `startup.conf`: hugepages، workers، RSS، NUMA، isolcpus، PCI whitelist | T1 | 30 |
| D0.7 | هستهٔ `vrx-agent`: اتصال govpp، stats، reconciler/KVScheduler، گراف وابستگی | T1 | 70 |
| D0.8 | هستهٔ `vrx-api`: NestJS، datastore candidate/running، موتور commit، confirmed-commit، revisions، rollback | T1 | 75 |
| D0.9 | Auth/RBAC/audit/API-key/TLS/مدیریت اسرار با TPM | T1 | 55 |
| D0.10 | پوستهٔ UI: تم، چیدمان، i18n fa/en + RTL، SchemaForm، DataGrid، کلاینت WS، نوار تغییرات معلق + diff | T1 | 80 |
| D0.11 | CLI تعاملی شبیه clixon (config mode، completion، `show` ها) | T1 | 90 |
| D0.12 | خط لولهٔ تلمتری: stats → gRPC stream → WS fan-out → Prometheus → تاریخچه | T1 | 50 |
| D0.13 | زیرساخت تست: containerlab/QEMU، Robot Framework، مهار TRex، Playwright | T1 | 70 |

### D1 — اینترفیس‌ها و لایهٔ ۲ — ۳۰۰ PD

| # | قابلیت | Tier | PD |
|---|---|---|---|
| D1.1 | موجودی NIC و bind درایور: `dpdk`، `rdma`(ConnectX)، `af_xdp`، `af_packet`، `vmxnet3`، `virtio`، `vhost-user`، `tap-v2`، `netmap`، `memif`، `pipe`، درایور Marvell mGig (جدید در 26.06) | T1 | 70 |
| D1.2 | پایهٔ اینترفیس: admin state، MTU، MAC، promisc، rx-mode، rx/tx queues، placement روی worker | T1 | 35 |
| D1.3 | آدرس‌دهی IPv4/IPv6 + IP unnumbered | T1 | 20 |
| D1.4 | زیراینترفیس 802.1q + QinQ/802.1ad + stacking | T1 | 25 |
| D1.5 | Bonding: LACP، XOR، round-robin، active-backup | T1 | 30 |
| D1.6 | L2: bridge domain، cross-connect، L2XC، L3XC، split-horizon، MAC aging، فیلتر MAC زمان‌بندی‌شده | T1 | 45 |
| D1.7 | LLDP | T2 | 12 |
| D1.8 | VNET GSO، checksum offload، jumbo frame | T1 | 20 |
| D1.9 | Network Delay Simulator (nsim) | T3 | 8 |
| D1.10 | SPAN / ERSPAN mirroring | T2 | 25 |
| D1.11 | Loopback / BVI | T1 | 10 |

### D2 — لایهٔ ۳، مسیریابی پایه، MPLS، Multicast — ۳۷۵ PD

| # | قابلیت | Tier | PD |
|---|---|---|---|
| D2.1 | مدیریت VRF/table + Source VRF Select | T1 | 30 |
| D2.2 | مسیر ثابت v4/v6، ECMP، مسیر وزن‌دار، نمای DPO/adjacency | T1 | 35 |
| D2.3 | پایگاه همسایه: ARP، ND، Proxy-ND، IPv6 RA، IPv6 DAD (جدید 26.06) | T1 | 30 |
| D2.4 | RPF strict/loose + ADL (allow/deny list) + Auto-SDL | T2 | 30 |
| D2.5 | مرورگر FIB با صفحه‌بندی سمت سرور (۱M+ مسیر) | T1 | 25 |
| D2.6 | ابزارها: ping، traceroute، ARP flush | T1 | 20 |
| D2.7 | Classifier/classify table + IP session redirect + ACL-Based Forwarding (PBR) | T2 | 45 |
| D2.8 | MPLS: عملیات برچسب، LSP، MPLS-over-Ethernet، L3VPN پایه، SR-MPLS | T2 | 90 |
| D2.9 | Multicast: IGMPv3، PIM (از FRR)، mfib، BIER | T3 | 70 |

### D3 — مسیریابی دینامیک (FRR + linux-cp) — ۵۴۰ PD

| # | قابلیت | Tier | PD |
|---|---|---|---|
| D3.1 | **سخت‌سازی linux-cp/linux-nl** — همگام‌سازی جفت اینترفیس، state/MTU، VRF، IPv6، کارایی در جدول کامل | T1 | 90 |
| D3.2 | چرخهٔ عمر FRR: چارچوب renderer، خوانندهٔ وضعیت JSON، reload ایمن، dry-run | T1 | 60 |
| D3.3 | BGP: هسته، AFI/SAFI v4/v6، community/ext/large، route-map، prefix-list، AS-path filter، peer-group، route-reflector، confederation، graceful restart، BGP Roles (RFC 9234)، add-path، VPNv4/VPNv6 | T1 | 140 |
| D3.4 | OSPFv2 | T1 | 55 |
| D3.5 | OSPFv3 | T2 | 40 |
| D3.6 | IS-IS | T2 | 45 |
| D3.7 | RIPv2 / RIPng | T2 | 25 |
| D3.8 | BFD (پلاگین بومی VPP + یکپارچگی با FRR) | T1 | 35 |
| D3.9 | Policy-Based Routing در API/UI | T2 | 20 |
| D3.10 | ماتریس redistribution + تجربهٔ کاربری route-policy | T1 | 30 |

### D4 — NAT و CGNAT — ۳۶۵ PD

| # | قابلیت | Tier | PD |
|---|---|---|---|
| D4.1 | NAT44-ED: session، pool، twice-NAT، port-forward، 1:1، interface-NAT، hairpinning، session helper/ALG | T1 | 90 |
| D4.2 | NAT44-EI | T2 | 25 |
| D4.3 | NAT64، NAT66، NPTv6 | T2 | 50 |
| D4.4 | DET44 (CGNAT قطعی) + لاگ‌برداری و IPFIX مخصوص CGNAT | T2 | 45 |
| D4.5 | DS-Lite، MAP-E، MAP-T، LW4o6 BR، 464XLAT | T2 | 75 |
| D4.6 | CNAT: policy 1:1، SNAT/DNAT policy (جدید 26.06)، ترجمهٔ متعادل‌شده | T2 | 45 |
| D4.7 | مرورگر Session در مقیاس ۱M+ با فیلتر سمت سرور و kill session | T1 | 35 |

### D5 — فایروال، ACL و سیاست — ۲۷۰ PD

| # | قابلیت | Tier | PD |
|---|---|---|---|
| D5.1 | مدل اشیا: address، group، FQDN، service، schedule، zone، tag | T1 | 50 |
| D5.2 | پلاگین ACL: L2 MACIP، L3/L4 بدون حالت و باحالت، ورودی/خروجی هر اینترفیس، شمارندهٔ برخورد | T1 | 70 |
| D5.3 | Host/local-in ACL برای حفاظت صفحهٔ مدیریت + فایروال میزبان با nftables | T1 | 30 |
| D5.4 | ویرایشگر قواعد در UI برای ۱۰۰٬۰۰۰ قاعده (virtualized، CSV bulk، reorder، جستجو) | T1 | 60 |
| D5.5 | یکپارچگی ADL / Auto-SDL | T3 | 15 |
| D5.6 | همگام‌سازی حالت stateful برای HA (معادل VPF state sync در TNSR) | T2 | 45 |

### D6 — VPN و تونل‌ها — ۶۵۰ PD

| # | قابلیت | Tier | PD |
|---|---|---|---|
| D6.1 | هستهٔ IPsec: SA/SPD، حالت tunnel و transport، ESP/AH، crypto غیرهمگام، موتورهای native/ipsecmb/openssl، offload به QAT | T1 | 80 |
| D6.2 | strongSwan + پلاگین‌های `kernel-vpp`/`socket-vpp`، renderer `swanctl.conf`، IKEv1/IKEv2، تمام cipher ها و DH group 1–24 و 31، NAT-T، DPD، MOBIKE | T1 | 100 |
| D6.3 | مسیر جایگزین: پلاگین بومی IKEv2 در VPP + بهبودهای رمزنگاری 26.06 | T2 | 40 |
| D6.4 | PKI: CA، CSR، import/export، CRL/OCSP، ACME، هشدار انقضا، PKCS#11/HSM | T1 | 60 |
| D6.5 | WireGuard | T1 | 35 |
| D6.6 | تونل‌ها: GRE (L2/L3)، IPIP، VXLAN، VXLAN-GPE، GTP-U، L2TPv3، PPPoE، Tunnel Infra، Packet Vector Tunnel | T1/T3 | 90 |
| D6.7 | SRv6: هسته، network programming، service chaining (static/dynamic/masquerading proxy)، SRv6-mobile (توابع GTP) | T3 | 110 |
| D6.8 | LISP و LISP-GPE | T3 | 40 |
| D6.9 | VPN دسترسی راه دور: IKEv2 + EAP + استخر کلاینت (و اختیاری OpenVPN/SSL-VPN) | T2 | 60 |
| D6.10 | داشبورد تونل‌ها، بازرسی SA/SPI، تاریخچهٔ rekey، تلمتری هر تونل | T1 | 35 |

### D7 — سرویس‌های شبکه، QoS و Host Stack — ۵۷۵ PD

| # | قابلیت | Tier | PD |
|---|---|---|---|
| D7.1 | سرور DHCPv4/v6 (Kea) + reservation + option set + پایگاه و UI اجاره‌ها | T1 | 60 |
| D7.2 | DHCP relay/proxy (پلاگین VPP) + DHCP client | T1 | 30 |
| D7.3 | DNS: Unbound (resolver/forwarder/DNSSEC/view) + پلاگین caching DNS در VPP | T1 | 50 |
| D7.4 | NTP با chrony (client/server) + PTP اختیاری | T1 | 25 |
| D7.5 | SNMP v2c/v3 با net-snmp + MIB اختصاصی | T2 | 45 |
| D7.6 | IPFIX/NetFlow (flowprobe) + sFlow | T2 | 50 |
| D7.7 | صدور syslog + مرورگر لاگ | T1 | 35 |
| D7.8 | QoS: policer، shaper، QoS record/map/mark، DSCP/dot1p، HQoS سلسله‌مراتبی، صف‌بندی هر اینترفیس | T2 | 85 |
| D7.9 | پلاگین Load Balancer (GRE/NAT/L3DSR/maglev) | T3 | 40 |
| D7.10 | **Host Stack:** session layer، تنظیم TCP/UDP، VCL، موتورهای TLS، QUIC/quicly، سرور HTTP(S) داخلی، HTTP/3 CONNECT و UDP proxying (جدید 26.06)، proxy app، SRTP، HSI | T3 | 130 |
| D7.11 | مولد بستهٔ PG + ابزار pcap در UI | T2 | 25 |

### D8 — رصدپذیری، عیب‌یابی و عملیات — ۴۲۵ PD

| # | قابلیت | Tier | PD |
|---|---|---|---|
| D8.1 | داشبورد + نمودارها + CPU هر worker، آمار صف و بافر، buffers monitoring، buffer metadata tracker | T1 | 60 |
| D8.2 | ضبط بسته: pcap rx/tx/drop، BPF Trace Filter، trace node، **Trace Path plugin** (جدید 26.06)، خروجی G2 | T1 | 55 |
| D8.3 | Prometheus exporter (پلاگین `prom` + متریک‌های خودمان) + داشبورد Grafana | T1 | 35 |
| D8.4 | موتور رویداد و هشدار + آستانه + ایمیل/webhook/SNMP trap | T1 | 45 |
| D8.5 | پشتیبان‌گیری/بازیابی، خروجی زمان‌بندی‌شده، قالب پیکربندی و تأمین انبوه | T1 | 40 |
| D8.6 | بستهٔ پشتیبانی / sysdump | T1 | 20 |
| D8.7 | UI ارتقا + انتشار مرحله‌ای | T1 | 25 |
| D8.8 | **لایهٔ سازگاری RESTCONF/NETCONF + مدل‌های YANG** (برای همتایی کامل با TNSR) | T2 | 90 |
| D8.9 | Terraform provider + Ansible collection + Python SDK | T2 | 55 |

### D9 — دسترس‌پذیری بالا و خوشه — ۲۱۰ PD

| # | قابلیت | Tier | PD |
|---|---|---|---|
| D9.1 | VRRPv3 (پلاگین بومی VPP) + مسیر keepalived | T1 | 45 |
| D9.2 | همگام‌سازی پیکربندی بین همتاها + عضویت خوشه | T1 | 50 |
| D9.3 | همگام‌سازی حالت: session های NAT/ACL و SA های IPsec | T2 | 60 |
| D9.4 | خودکارسازی تست failover و تنظیم زمان همگرایی | T1 | 30 |
| D9.5 | نمای خوشه در UI | T2 | 25 |

### D10 — چنداجاره‌ای و AAA — ۱۲۵ PD

| # | قابلیت | Tier | PD |
|---|---|---|---|
| D10.1 | مدل VDOM/tenant + RBAC و محدودسازی API به تفکیک tenant | T2 | 70 |
| D10.2 | AAA: RADIUS، TACACS+، LDAP، SAML، OIDC + MFA | T2 | 55 |

### D11 — ابر و مجازی‌سازی — ۱۳۰ PD

| # | قابلیت | Tier | PD |
|---|---|---|---|
| D11.1 | ایمیج ابری AWS/Azure/GCP + marketplace + cloud-init + درایور ENA/Hyper-V + یکپارچگی مسیریابی ابری | T2 | 90 |
| D11.2 | ایمیج KVM/VMware/Hyper-V/Proxmox + SR-IOV و PCI passthrough | T1 | 40 |

### D12 — سخت‌سازی، تضمین کیفیت، مستندات و انتشار — ۵۳۵ PD

| # | قابلیت | Tier | PD |
|---|---|---|---|
| D12.1 | سخت‌سازی CIS، secure boot، امضای بسته و بستهٔ به‌روزرسانی آفلاین (air-gapped) | T1 | 60 |
| D12.2 | مدل مجوز و اجرای entitlement | T1 | 40 |
| D12.3 | تست نفوذ مستقل + رفع یافته‌ها | T1 | 45 |
| D12.4 | مستندات کامل کاربر (~۴۰۰ صفحه) + مرجع CLI + مرجع API | T1 | 120 |
| D12.5 | QA: رشد مجموعهٔ رگرسیون، تست شبانهٔ توپولوژی، perf CI، ماتریس interop (Cisco، Juniper، FortiGate، strongSwan، AWS VGW) | T1 | 160 |
| D12.6 | Soak و chaos: `kill -9`، link flap، ۷۲ ساعت در نرخ خط | T1 | 50 |
| D12.7 | آماده‌سازی گواهی‌نامه (CC / استانداردهای داخلی) | T2 | 60 |

### جمع کل

| دامنه | PD | سهم |
|---|---|---|
| D0 زیرساخت | 685 | ۱۳٪ |
| D1 اینترفیس/L2 | 300 | ۶٪ |
| D2 L3/MPLS/Multicast | 375 | ۷٪ |
| D3 مسیریابی دینامیک | 540 | ۱۰٪ |
| D4 NAT/CGNAT | 365 | ۷٪ |
| D5 فایروال/ACL | 270 | ۵٪ |
| D6 VPN/تونل | 650 | ۱۳٪ |
| D7 سرویس/QoS/Host-Stack | 575 | ۱۱٪ |
| D8 رصد/عملیات | 425 | ۸٪ |
| D9 HA | 210 | ۴٪ |
| D10 چنداجاره‌ای/AAA | 125 | ۲٪ |
| D11 ابر | 130 | ۳٪ |
| D12 کیفیت/انتشار | 535 | ۱۰٪ |
| **خالص** | **۵٬۱۸۵** | ۱۰۰٪ |
| **با سرباره ۲۵٪** | **۶٬۴۸۰** | |

---

## ۴. ساختار تیم و اسکوادها

| اسکواد | نفر | مسئولیت | مهارت کلیدی |
|---|---|---|---|
| **S1 — Dataplane & Agent** | ۳ | پلاگین‌های VPP، govpp، reconciler، renderer ها، stats | C، Go، DPDK، VPP internals |
| **S2 — Routing & L3** | ۲ | linux-cp، FRR، BGP/OSPF/IS-IS، MPLS، multicast، FIB | Go، مهندسی شبکه (CCIE/JNCIE) |
| **S3 — Security & VPN** | ۲ | IPsec، strongSwan، PKI، WireGuard، NAT، ACL | Go، رمزنگاری، IKE |
| **S4 — Control Plane** | ۲ | NestJS، موتور commit، RBAC، تلمتری، RESTCONF، SDK ها | Node.js/TypeScript |
| **S5 — Frontend** | ۲ | React/MUI، SchemaForm، داشبورد، i18n/RTL | React، TypeScript، UX |
| **S6 — Platform & Release** | ۱ | بیلد VPP، بسته‌بندی، ایمیج، APT، ارتقا، CI | DevOps، Debian packaging |
| **QA / Lab** | ۲ | Robot، TRex، interop، شبانه، perf CI | اتوماسیون تست شبکه |
| **معمار / TPM** | ۱ | معماری، بازبینی، برنامه، ۵۰٪ تحویل | — |
| **نویسندهٔ فنی** | ۰.۵ (از ماه ۱۲) | مستندات کاربر، مرجع CLI/API | — |
| **مجموع سرشماری** | **۱۵–۱۶** | ظرفیت تحویل مؤثر ≈ ۱۴ FTE | |

**محاسبهٔ ظرفیت:** ۱۴ FTE × ۱۹۰ نفر-روز مؤثر در سال = **۲٬۶۶۰ نفر-روز در سال**.
۶٬۴۸۰ ÷ ۲٬۶۶۰ = ۲.۴۴ سال تحویل خالص، به‌اضافهٔ یک سه‌ماههٔ راه‌اندازی و جذب نیرو
(ظرفیت ۵۰٪) و سه ماه گیت‌های تثبیت انتشار که موازی‌سازی نمی‌شوند → **۳۶ ماه**.

### برنامهٔ جذب نیرو

| ماه | استخدام |
|---|---|
| M0 (پیش از شروع) | معمار/TPM + **یک مهندس ارشد VPP/DPDK** ← گلوگاه اصلی جذب، از امروز شروع کنید |
| M1 | ۲ × Go، ۲ × Node.js، ۱ × React، ۱ × DevOps |
| M3 | ۱ × React، ۱ × QA automation |
| M6 | ۱ × Go (اسکواد امنیت)، ۱ × مهندس شبکهٔ ارشد (S2)، ۱ × QA/perf |
| M12 | نویسندهٔ فنی (پاره‌وقت/قراردادی) |

---

## ۵. قطار انتشار (Release Train)

| نسخه | ماه | دامنه | وضعیت تجاری |
|---|---|---|---|
| **R0.1 α** | M6 | زیرساخت + اینترفیس/L2 + تلمتری زنده | آلفای داخلی |
| **R0.5 β (MVP)** | M10 | + L3/static/VRF + NAT44 پایه + IPsec S2S + DHCP/DNS/NTP | **اولین PoC مشتری — قابل فروش محدود** |
| **R1.0 GA-1** | M16 | + BGP/OSPF/BFD + ACL کامل + NAT کامل + VRRP + SNMP + پشتیبان‌گیری/ارتقا | **GA «روتر لبه و کانسنتریتور IPsec»** |
| **R1.5 GA-2** | M27 | + IS-IS/OSPFv3/RIP + CGNAT/MAP + WireGuard + PKI + QoS + IPFIX/sFlow + RESTCONF + HA state-sync + AAA | **GA «کریر» — همتایی کامل با TNSR** |
| **R2.0 GA-3** | M30 | + MPLS/SR-MPLS + Multicast/BIER + SRv6 + LISP + Host Stack/QUIC/HTTP3 + Load Balancer + چنداجاره‌ای + ابر | **پوشش کامل سطح قابلیت VPP 26.06** |
| **R2.5 GA-Final** | M36 | سخت‌سازی، secure boot، مجوز، آفلاین، تست نفوذ، soak ۷۲ ساعته، مستندات کامل، گواهی‌نامه | **محصول نهایی سازمانی/دولتی** |

---

## ۶. زمان‌بندی سه‌ماهه

### Q1 (ماه ۱–۳) — «اسلایس عمودی اول»
- **S1:** آزمایشگاه QEMU با VPP 26.06؛ اتصال govpp؛ dump اینترفیس؛ هستهٔ reconciler (Descriptor، گراف وابستگی، Retrieve)؛ خوانندهٔ stats
- **S4:** اسکلت NestJS؛ پکیج `schema` با Zod؛ datastore candidate/running؛ اولین commit اتمی
- **S5:** پوستهٔ UI، تم MUI، ورود، چیدمان، i18n fa/en + RTL، اولین `SchemaForm`
- **S6:** monorepo، CI، بیلد `.deb` از VPP پین‌شده، مخزن APT
- **QA:** مهار Robot Framework، توپولوژی سه‌گرهی containerlab
- **گیت G1:** نصب تازه روی سرور فیزیکی → ورود → فهرست NIC با شمارندهٔ زنده → ریبوت → سالم

### Q2 (ماه ۴–۶) — اینترفیس و L2 کامل
- **S1:** D1.1–D1.6 (bind درایورها، MTU/MAC/queue، VLAN/QinQ، LACP، bridge domain)
- **S2:** D0.6 مولد `startup.conf` + تنظیم NUMA/worker؛ شروع مطالعهٔ linux-cp
- **S4:** confirmed-commit، revisions، rollback، audit، RBAC، API-key
- **S5:** صفحات اینترفیس، نوار تغییرات معلق + نمایشگر diff، DataGrid سمت سرور
- **S6:** ایمیج ISO autoinstall؛ ارتقای بسته‌محور
- **QA:** perf harness با TRex؛ خط پایهٔ pps
- **گیت G2 = R0.1 α:** پینگ روی زیراینترفیس VLAN در نرخ خط؛ شمارنده‌ها با `vppctl show int` یکسان؛ `kill -9 vpp` → بازسازی خودکار کامل پیکربندی

### Q3 (ماه ۷–۹) — L3، NAT پایه، IPsec پایه
- **S1:** D2.1–D2.6 (VRF، مسیر ثابت، ECMP، ARP/ND، مرورگر FIB، ابزارها)
- **S2:** **D3.1 سخت‌سازی linux-cp** (بلندترین قلم مسیر بحرانی — از الان شروع)
- **S3:** D6.1 هستهٔ IPsec + D6.2 آغاز strongSwan؛ D4.1 آغاز NAT44-ED
- **S4:** اعتبارسنجی سه‌لایه، dry-run، قفل candidate، تلمتری WS
- **S5:** صفحات مسیریابی، داشبورد نسخهٔ ۱، ابزار ping/traceroute
- **S6:** بسته‌بندی سرویس‌ها (FRR، strongSwan، Kea، Unbound، chrony)
- **گیت G3:** توپولوژی سه‌روتری با مسیر ثابت؛ تونل IPsec بالا با strongSwan همتا

### Q4 (ماه ۱۰–۱۲) — MVP و شروع BGP
- **S1:** D1.8–D1.11، D2.7 classifier/PBR
- **S2:** D3.2 چارچوب FRR + D3.3 آغاز BGP
- **S3:** تکمیل D4.1 NAT44-ED کامل + D5.1 مدل اشیا
- **S4:** پشتیبان‌گیری/بازیابی، صدور پیکربندی، رویداد و هشدار
- **S5:** ویرایشگر قواعد ACL/NAT، صفحات سرویس‌ها
- **S7 (D7):** Kea DHCP، Unbound، chrony
- **گیت G4 = R0.5 β (MVP):** اولین PoC مشتری؛ NAT + IPsec + static در تولید سبک

### Q5 (ماه ۱۳–۱۵) — مسیریابی دینامیک
- **S2:** تکمیل D3.3 BGP (route-map، prefix-list، RR، graceful restart، RFC 9234) + D3.4 OSPFv2 + D3.8 BFD + D3.10 ماتریس redistribution
- **S3:** D5.2 پلاگین ACL کامل + D5.3 host ACL/nftables
- **S1:** D2.4 RPF/ADL، بهینه‌سازی کارایی linux-cp در جدول کامل BGP
- **S5:** D5.4 ویرایشگر ۱۰۰٬۰۰۰ قاعده، صفحات BGP/OSPF/BFD
- **QA:** تست جدول کامل BGP (۱M مسیر)، تست همگرایی BFD

### Q6 (ماه ۱۶–۱۸) — GA-1 و تثبیت
- تکمیل D9.1 VRRP، D8.5–D8.7 عملیات، D7.5 SNMP، D0.11 CLI نسخهٔ ۱، D0.5 ارتقای A/B
- **QA:** interop با Cisco/Juniper/FortiGate؛ soak ۷۲ ساعته؛ اولین دور تست نفوذ
- **گیت G5 = R1.0 GA-1:** جدول کامل BGP < ۹۰ ثانیه همگرایی؛ failover BFD < ۳۰۰ms؛ ACL صد هزار قاعده بدون افت pps؛ تونل ۲۰Gbps AES-GCM

### Q7 (ماه ۱۹–۲۱) — کریر: CGNAT، QoS، VPN پیشرفته
- **S3:** D4.3–D4.6 (NAT64/66، NPTv6، DET44، MAP-E/T، DS-Lite، CNAT) + D6.5 WireGuard + D6.4 PKI
- **S1:** D7.8 QoS/HQoS + D1.10 SPAN/ERSPAN
- **S2:** D3.5 OSPFv3 + D3.6 IS-IS + D3.7 RIP
- **S4:** D8.8 RESTCONF/NETCONF + YANG
- **S5:** صفحات CGNAT، QoS، PKI، WireGuard

### Q8 (ماه ۲۲–۲۴) — تکمیل دامنهٔ کریر
- D7.6 IPFIX/sFlow، D9.3 state-sync، D10.2 AAA، D6.9 VPN دسترسی راه دور، D8.9 Terraform/Ansible/SDK
- **گیت G6 = R1.5 GA-2 (پایان Q9، ماه ۲۷):** ماتریس قابلیت TNSR ۱۰۰٪ پوشش داده شده و مستند؛ قطع برق master → failover < ۱ ثانیه با حفظ session ها

### Q9 (ماه ۲۵–۲۷) — MPLS و Multicast
- **S1+S2:** D2.8 MPLS/SR-MPLS + D2.9 IGMPv3/PIM/mfib/BIER
- **S3:** D6.7 آغاز SRv6 (هسته + network programming)
- **S4/S5:** صفحات MPLS/multicast، مرورگر LSP

### Q10 (ماه ۲۸–۳۰) — سطح کامل VPP
- **S3:** تکمیل D6.7 SRv6 service chaining + SRv6-mobile؛ D6.8 LISP
- **S1:** D7.10 Host Stack (session layer، VCL، TLS، QUIC/quicly، HTTP static، HTTP/3 CONNECT و UDP proxy، SRTP، HSI) + D7.9 Load Balancer
- **S4:** D10.1 چنداجاره‌ای/VDOM
- **S6:** D11.1 ایمیج‌های ابری + marketplace
- **گیت G7 = R2.0 GA-3:** هر پلاگین VPP 26.06 یا در UI/API عرضه شده یا رسماً در فهرست «خارج از دامنه» ثبت شده

### Q11 (ماه ۳۱–۳۳) — سخت‌سازی و مستندات
- D12.1 سخت‌سازی CIS/secure boot/بستهٔ آفلاین، D12.2 مجوز، D12.4 مستندات کامل، D8.2 Trace Path و ابزار عیب‌یابی نهایی
- **QA:** رگرسیون کامل، ماتریس interop گسترده، chaos testing

### Q12 (ماه ۳۴–۳۶) — انتشار نهایی
- D12.3 تست نفوذ مستقل + رفع، D12.6 soak ۷۲ ساعته در نرخ خط، D12.7 گواهی‌نامه، freeze و انتشار
- **گیت G8 = R2.5 GA-Final:** صفر نشتی حافظه و صفر crash در ۷۲ ساعت؛ تست نفوذ پاک؛ مستندات کامل؛ SBOM و NOTICE

---

## ۷. زمان‌بندی اسپرینتی شش ماه اول (اسپرینت دوهفته‌ای)

| اسپرینت | ماه | خروجی مشخص و قابل اثبات |
|---|---|---|
| **S01** | 1.0 | monorepo + CI سبز + Vagrant/QEMU با Ubuntu 24.04 و VPP 26.06 و دو اینترفیس virtio متصل به VPP (همه اتوماسیون، نه دستی) |
| **S02** | 1.5 | `vrx-agent` با govpp وصل می‌شود و `Retrieve` اینترفیس‌ها را برمی‌گرداند؛ یک RPC gRPC روی سوکت یونیکس |
| **S03** | 2.0 | `vrx-api`: `GET /api/v1/state/interfaces`؛ پکیج `schema` با Zod؛ خط codegen (OpenAPI + client) |
| **S04** | 2.5 | صفحهٔ React فهرست اینترفیس‌ها با شمارندهٔ زنده روی WebSocket → **اسلایس عمودی کامل اثبات شد** |
| **S05** | 3.0 | هستهٔ reconciler: Descriptor + گراف وابستگی + diff/plan/apply؛ datastore candidate/running؛ اولین commit اتمی روی MTU |
| **S06** | 3.5 | بیلد `.deb` + مخزن APT امضاشده؛ نصب روی سرور فیزیکی؛ **گیت G1** |
| **S07** | 4.0 | bind درایور DPDK از UI؛ موجودی NIC؛ مولد `startup.conf` با hugepages/worker/RSS |
| **S08** | 4.5 | آدرس‌دهی v4/v6، MTU، MAC، rx-queue placement؛ confirmed-commit با تایمر بازگشت خودکار |
| **S09** | 5.0 | زیراینترفیس 802.1q و QinQ؛ نوار تغییرات معلق + نمایشگر diff در UI |
| **S10** | 5.5 | Bonding/LACP؛ bridge domain و L2XC؛ RBAC + audit + API-key |
| **S11** | 6.0 | GSO/checksum offload/jumbo؛ loopback/BVI؛ LLDP؛ ISO autoinstall |
| **S12** | 6.5 | تست `kill -9 vpp` در CI شبانه؛ خط پایهٔ TRex؛ **گیت G2 = R0.1 α** |

از اسپرینت ۱۳ به بعد، برنامه در سطح سه‌ماهه (بخش ۶) مدیریت و در ابتدای هر سه‌ماهه
به اسپرینت شکسته می‌شود — برنامه‌ریزی اسپرینتی دقیق‌تر از دو سه‌ماهه جلوتر، تخمین خیالی است.

---

## ۸. مسیر بحرانی (Critical Path)

```
هستهٔ reconciler (S05) ──► سخت‌سازی linux-cp (Q3) ──► BGP (Q4–Q5) ──► GA-1 (Q6)
        │                            │
        └──► موتور commit (Q2) ──────┘
                                     └──► state-sync برای HA (Q8) ──► GA-2
IPsec core (Q3) ──► strongSwan+VPP (Q3–Q4) ──► PKI (Q7) ──► GA-2
MPLS/SRv6/Host-Stack (Q9–Q10) ──► GA-3 ──► سخت‌سازی و گواهی (Q11–Q12)
```

**سه قلم که هیچ راهی برای موازی‌سازی‌شان نیست و تأخیرشان کل پروژه را جابه‌جا می‌کند:**
۱. **هستهٔ reconciler** — همه چیز روی آن سوار است. دو هفتهٔ تأخیر = دو هفته تأخیر در GA.
۲. **linux-cp ↔ FRR** — باتلاق کلاسیک. **در Q3 نمونهٔ اولیه بسازید، نه در Q5.**
۳. **مجوز/گواهی‌نامه** — فهرست الزامات را قبل از پایان Q2 بگیرید، وگرنه در Q11 دوباره‌کاری معماری دارید.

---

## ۹. سرمایه‌گذاری آزمایشگاه (CapEx)

| ماه | اقلام | برآورد |
|---|---|---|
| M0 | ۲ سرور آزمایش (single-socket، AES-NI، AVX2) + کارت ۲۵G Intel XXV710 + سوییچ ۲۵G | ~۴۰ هزار دلار |
| M4 | **دستگاه مولد ترافیک TRex** + کارت‌های ۱۰۰G (E810) — بدون این هیچ عدد کارایی قابل ادعا نیست | ~۳۵ هزار دلار |
| M10 | جفت HA + سوییچ دوم + کارت ConnectX-6 برای تست درایور rdma | ~۳۰ هزار دلار |
| M18 | سوییچ ۱۰۰G + ماژول‌ها + دستگاه QAT برای offload رمزنگاری | ~۳۵ هزار دلار |
| M24 | تجهیزات interop (روتر Cisco/Juniper دست دوم، FortiGate) برای ماتریس سازگاری | ~۲۵ هزار دلار |
| **جمع** | | **~۱۶۵ هزار دلار** |

---

## ۱۰. سناریوها

| سناریو | دامنه | تیم (FTE تحویل) | مدت | ریسک |
|---|---|---|---|---|
| **A — تهاجمی** | کامل (T1+T2+T3) | ۲۲ | ۲۴ ماه | بالا: مسیر بحرانی موازی نمی‌شود؛ سرباره هماهنگی ۱۵٪+؛ جذب ۲۲ نفر متخصص VPP در ایران/منطقه عملاً ناممکن |
| **B — پیشنهادی** | کامل (T1+T2+T3) | ۱۴ | **۳۶ ماه** (مدل: ۳۴ ماه + ۲ ماه لقی برنامه‌ریزی) | متعادل |
| **C — کم‌بودجه** | کامل | ۱۰ | ۴۶ ماه | خطر عقب افتادن از نسخه‌های VPP و جابه‌جایی بازار |
| **D — هوشمندانه** | فقط T1 | ۱۴ | **۲۴ ماه** | **کم‌ترین ریسک، بیشترین بازگشت سرمایه** |
| **E — همتایی TNSR** | T1+T2 | ۱۴ | ۳۲ ماه | متعادل |

**توصیهٔ صریح من: سناریوی D را اجرا کنید و بفروشید، درآمد آن T2 و T3 را تأمین کند.**
۲۴ ماه تا محصول فروش‌پذیر با همهٔ قابلیت‌های حیاتی، در برابر ۳۶ ماه تا محصولی که
۸٪ از دامنه‌اش (SRv6-mobile، LISP، BIER، quicly) هیچ مشتری‌ای نمی‌خواهد.

| سطح | حجم | سهم |
|---|---|---|
| T1 (ضروری) | ۳٬۳۴۵ PD | ۶۵٪ |
| T2 (همتایی TNSR) | ۱٬۴۲۷ PD | ۲۷٪ |
| T3 (پوشش کامل VPP) | ۴۱۳ PD | ۸٪ |

---

## ۱۱. اگر زمان کم است، اینها را حذف یا تعویق کنید

| قلم | PD | چرا |
|---|---|---|
| D7.10 Host Stack (QUIC، quicly، HTTP/3، VCL، SRTP، HSI) | 130 | VPP اینها را برای کاربردهای host-stack دارد، نه روتر لبه. هیچ مشتری روتری آن را نمی‌خواهد. |
| D6.7 SRv6 + SRv6-mobile | 110 | فقط اپراتورهای موبایل بزرگ. تا وقتی مشتری مشخص ندارید نسازید. |
| D2.9 Multicast/BIER | 70 | BIER تقریباً بی‌استفاده؛ IGMP/PIM را نگه دارید اگر مشتری IPTV دارید. |
| D6.8 LISP/LISP-GPE | 40 | عملاً مرده. |
| D7.9 Load Balancer | 40 | با HAProxy/nginx بیرون از محصول حل می‌شود. |
| D5.5 ADL/Auto-SDL | 15 | با ACL پوشش داده می‌شود. |
| D1.9 nsim | 8 | ابزار تست، نه قابلیت محصول. |
| **جمع آزادشده** | **۴۱۳ PD ≈ ۲.۵ ماه تقویمی** | |

برای رسیدن به ۲۲ ماه، علاوه بر بالا اینها را هم به فاز دوم ببرید:
MPLS/SR-MPLS (۹۰)، RESTCONF/YANG (۹۰)، ایمیج ابری (۹۰)، MAP-E/T و DS-Lite (۷۵)،
چنداجاره‌ای (۷۰)، HA state-sync (۶۰)، VPN دسترسی راه دور (۶۰)، QoS سلسله‌مراتبی (۸۵ → ۳۰).

---

## ۱۲. معیار پذیرش گیت‌ها (بدون اینها گیت باز نمی‌شود)

| گیت | ماه | معیار عددی و قابل اندازه‌گیری |
|---|---|---|
| **G1** | ۳ | نصب خودکار روی سخت‌افزار خام < ۱۵ دقیقه؛ ورود؛ فهرست NIC؛ ریبوت و سالم ماندن |
| **G2** | ۶ | ≥ ۱۰ Mpps/core فوروارد IPv4 در ۶۴ بایت؛ `kill -9 vpp` → بازسازی کامل پیکربندی < ۳۰ ثانیه؛ شمارنده‌های UI = `vppctl` |
| **G3** | ۹ | تونل IPsec با strongSwan و FortiGate و Cisco بالا می‌آید؛ ۳ روتر static فوروارد می‌کنند |
| **G4** | ۱۲ | یک مشتری واقعی PoC را گذرانده؛ ۱۰۰٬۰۰۰ session NAT پایدار؛ commit/rollback بدون قطعی |
| **G5** | ۱۶ | جدول کامل BGP (~۱M مسیر) همگرا < ۹۰ ثانیه؛ BFD failover < ۳۰۰ms؛ ACL ۱۰۰k قاعده با افت < ۲٪ pps؛ IPsec ≥ ۲۰ Gbps AES-GCM-128؛ soak ۷۲ ساعته پاک |
| **G6** | ۲۷ | ماتریس ۱۰۰٪ قابلیت‌های TNSR امضاشده؛ failover VRRP < ۱ ثانیه با حفظ session؛ ۱M session NAT؛ ≥ ۱۰۰ Gbps روی ۱۶ worker |
| **G7** | ۳۰ | هر پلاگین VPP 26.06 یا عرضه شده یا در فهرست «خارج از دامنه» با دلیل ثبت شده |
| **G8** | ۳۶ | صفر crash و صفر نشتی در ۷۲ ساعت نرخ خط؛ تست نفوذ مستقل پاک؛ بستهٔ به‌روزرسانی آفلاین امضاشده؛ SBOM و NOTICE کامل؛ مستندات ~۴۰۰ صفحه |
