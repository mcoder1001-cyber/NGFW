# برنامهٔ ۳۰ روزه — تولید VRX با ایجنت‌های کدنویس

هدف: در **۳۰ روز تقویمی** با ۱ تا ۲ مهندس انسانی به‌عنوان ناظر/یکپارچه‌ساز و **۶ تا ۱۲ ایجنت
موازی** (Claude Code)، به یک محصول نرم‌افزاری قابل‌دمو برسیم که روی VPP واقعی (در VM و
کانتینر) کار کند و معماری کامل محصول را پیاده کرده باشد.

---

## ۱. حرف صریح: چه چیزی در ۳۰ روز شدنی است و چه چیزی نه

| در ۳۰ روز **شدنی** است | در ۳۰ روز **شدنی نیست** — با هیچ تعداد ایجنت |
|---|---|
| معماری کامل: agent (Go/govpp) + API (NestJS) + UI (React/MUI) + reconciler + commit/rollback/confirmed-commit | اعداد کارایی روی سخت‌افزار واقعی (۱۰۰G، ۱۰ Mpps/core) — نیاز به NIC فیزیکی، TRex و هفته‌ها تیونینگ |
| اینترفیس‌ها، VLAN/QinQ، bonding، bridge، آدرس‌دهی، VRF، مسیر ثابت، ECMP، ARP/ND | ماتریس کارت شبکه و درایورهای DPDK/rdma روی سخت‌افزار |
| NAT44-ED، ACL + مدل اشیا، IPsec S2S (strongSwan+VPP)، WireGuard | CGNAT/MAP-T/E، DET44، SRv6، MPLS، multicast، host stack |
| FRR + linux-cp با BGP و OSPFv2 و BFD در توپولوژی مجازی | همگرایی جدول کامل BGP (۱M مسیر) و تیونینگ linux-cp در بار واقعی |
| DHCP (Kea)، DNS (Unbound)، NTP (chrony)، syslog، Prometheus | HA با state-sync، RESTCONF/YANG، چنداجاره‌ای، SNMP کامل |
| auth/RBAC/audit، backup/restore، تلمتری زنده، داشبورد، i18n fa/en RTL | تست نفوذ مستقل، سخت‌سازی CIS، secure boot، گواهی‌نامه |
| بستهٔ `.deb`، ایمیج ISO نصاب، CI کامل، تست E2E روی توپولوژی مجازی | soak ۷۲ ساعته در نرخ خط، interop با Cisco/Juniper/FortiGate واقعی |
| CLI پایه (show/config/commit) | ارتقای A/B، مجوز، مستندات ۴۰۰ صفحه‌ای |

**نتیجهٔ ۳۰ روز = R0.5 MVP از برنامهٔ اصلی + بخش‌هایی از R1.0**، قابل دمو و PoC داخلی،
**نه** قابل فروش به مشتری تولیدی. برای رسیدن به فروش، مسیر بعد از روز ۳۰ همان
[`08-master-schedule-fa.md`](08-master-schedule-fa.md) است — با این تفاوت که ایجنت‌ها
سرعت بخش کدنویسی را چند برابر کرده‌اند و آنچه می‌ماند عمدتاً آزمایشگاه، سخت‌افزار و اعتبارسنجی است.

---

## ۲. مدل اجرا با ایجنت‌ها

### اصول (بدون این‌ها ۱۰ ایجنت موازی فقط ۱۰ برابر آشوب می‌سازند)

1. **قرارداد اول، کد بعد.** روز ۱ تا ۳ فقط سه قرارداد بسته می‌شود و بعد **قفل**:
   `packages/schema` (Zod → JSON Schema/OpenAPI)، `packages/proto` (gRPC agent↔api) و
   چیدمان monorepo. هر ایجنتی که قرارداد را تغییر دهد باید PR جدا با برچسب `contract` بدهد
   و انسان تأیید کند.
2. **هر ایجنت در worktree و شاخهٔ خودش.** هیچ‌کس روی `main` کار نمی‌کند.
   `git worktree add ../vrx-<task> -b feat/<task>`.
3. **CI قاضی است، نه انسان.** lint + typecheck + unit + integration (VPP در کانتینر) روی هر
   PR. ایجنت تا CI سبز نشود PR را «تمام‌شده» اعلام نمی‌کند.
4. **ایجنت بازبین جدا.** بعد از هر PR، ایجنت دوم با پرامت بازبینی (`/code-review`) بررسی
   می‌کند: امنیت، تطبیق با قرارداد، تست واقعی (نه تست تقلبی).
5. **یک ایجنت یکپارچه‌ساز روزانه.** هر روز آخر وقت شاخه‌ها را روی `main` merge می‌کند،
   توپولوژی مجازی را بالا می‌آورد و تست E2E را می‌زند. اگر قرمز شد، صبح فردا اولویت اول است.
6. **binapi تولیدشده منبع حقیقت است.** ایجنت‌ها نام پیام‌های VPP API را از حافظه نمی‌نویسند؛
   فقط از bindings تولیدشدهٔ `binapi-generator` از همان نسخهٔ VPP استفاده می‌کنند.
   این جلوی بزرگ‌ترین خطای ایجنت‌ها در کار با VPP را می‌گیرد (نام API ساختگی).
7. **هر پرامت خودبسنده است.** پیش‌درآمد مشترک [`prompts/00-CONTEXT.md`](../prompts/00-CONTEXT.md)
   + پرامت وظیفه. ایجنت بدون تاریخچهٔ گفتگو باید بتواند کار را انجام دهد.

### نقش‌ها

| نقش | تعداد | کار |
|---|---|---|
| **انسان — معمار/یکپارچه‌ساز** | ۱ | تأیید قراردادها، merge، تصمیم‌های معماری، بالا بردن آزمایشگاه |
| **انسان — مهندس شبکه/بازبین** | ۱ (نیمه‌وقت) | راستی‌آزمایی رفتار شبکه (این پینگ واقعاً از VPP رد شد؟)، تست دستی |
| ایجنت‌های ساخت | ۶–۱۲ موازی | هر کدام یک پرامت از `prompts/` |
| ایجنت بازبین | ۱–۲ | بازبینی هر PR |
| ایجنت یکپارچه‌ساز | ۱ | merge روزانه + E2E |

### محیط توسعهٔ ایجنت‌ها (کلید سرعت)

VPP بدون DPDK در کانتینر با `af_packet` روی veth یا `memif` بالا می‌آید. این یعنی
هر ایجنت روی هر لپ‌تاپ/سرور CI می‌تواند VPP **واقعی** را اجرا و بسته رد کند — بدون NIC
فیزیکی. `docker compose up` → postgres + valkey + vpp + frr + agent + api. این را
روز ۱ می‌سازیم (پرامت P04) و همه‌چیز روی آن سوار است.

---

## ۳. زمان‌بندی ۴ هفته

### هفتهٔ ۱ — قرارداد، اسکلت، اسلایس عمودی (روز ۱–۷)

| روز | موج | ایجنت‌های موازی | خروجی |
|---|---|---|---|
| ۱ | W1 | P01 monorepo · P04 dev-env (VPP در Docker) | اسکلت + `docker compose up` سبز |
| ۲–۳ | W2 | P02 schema · P03 proto · P09 CI | **قراردادها قفل می‌شوند** (تأیید انسان) |
| ۳–۵ | W3 | P05 agent-core · P06 api-core · P07 ui-shell | سه لایه به‌طور مستقل با قرارداد کار می‌کنند |
| ۶–۷ | W4 | P08 اسلایس عمودی اینترفیس‌ها | **پینگ واقعی از VPP، شمارندهٔ زنده در UI، commit/rollback روی MTU** |

گیت انسانی هفتهٔ ۱: `kill -9 vpp` → agent کل پیکربندی را < ۳۰ ثانیه بازسازی کند.

### هفتهٔ ۲ — L2/L3 کامل، تراکنش، auth، بسته (روز ۸–۱۴)

| موج | ایجنت‌های موازی (با FEATURE-TEMPLATE) |
|---|---|
| W5 | F-vlan-qinq · F-bonding · F-bridge-domain · F-addressing-unnumbered |
| W6 | F-vrf · F-static-routes-ecmp · F-neighbors-arp-nd · F-fib-browser |
| W7 | C-confirmed-commit-revisions · C-auth-rbac-audit · C-telemetry-ws |
| W8 | P10 packaging `.deb` · F-tools-ping-traceroute · F-dashboard-v1 |

گیت انسانی: توپولوژی سه‌روتری مجازی با مسیر ثابت پینگ می‌دهد؛ ریبوت کامل → همه‌چیز برمی‌گردد.

### هفتهٔ ۳ — NAT، ACL، IPsec، سرویس‌ها، FRR (روز ۱۵–۲۱)

| موج | ایجنت‌های موازی |
|---|---|
| W9 | F-object-model · F-acl · F-nat44-ed · F-session-browser |
| W10 | P11 strongswan-vpp build · F-ipsec-s2s · F-pki-basic · F-wireguard |
| W11 | F-kea-dhcp · F-unbound-dns · F-chrony-ntp · F-syslog |
| W12 | P12 frr-linuxcp-framework · F-bgp-basic · F-prometheus |

گیت انسانی: تونل IPsec بین دو VRX مجازی + یک strongSwan خام بالا می‌آید و ترافیک رمز می‌شود؛
BGP بین VRX و یک کانتینر FRR مسیر تبادل می‌کند و مسیر در VPP FIB می‌نشیند.

### هفتهٔ ۴ — OSPF/BFD، backup، CLI، ISO، تثبیت (روز ۲۲–۳۰)

| موج | ایجنت‌های موازی |
|---|---|
| W13 | F-ospfv2 · F-bfd · F-redistribution · F-backup-restore |
| W14 | P13 cli-basic · F-alarms-events · F-i18n-fa-complete · P14 iso-installer |
| W15 (روز ۲۶–۲۸) | **فقط رفع باگ و تثبیت** — هیچ قابلیت جدیدی · ایجنت بازبین امنیتی روی کل کد |
| W16 (روز ۲۹–۳۰) | soak ۲۴ ساعته در توپولوژی مجازی · دمو · مستند «چه داریم / چه نداریم» |

---

## ۴. فهرست پرامت‌ها

| فایل | موج | نوع |
|---|---|---|
| [`00-CONTEXT.md`](../prompts/00-CONTEXT.md) | همه | پیش‌درآمد مشترک — به ابتدای **هر** پرامت بچسبانید |
| [`P01-monorepo-scaffold.md`](../prompts/P01-monorepo-scaffold.md) | W1 | اسکلت |
| [`P04-dev-environment.md`](../prompts/P04-dev-environment.md) | W1 | VPP در Docker |
| [`P02-schema-package.md`](../prompts/P02-schema-package.md) | W2 | قرارداد |
| [`P03-proto-contract.md`](../prompts/P03-proto-contract.md) | W2 | قرارداد |
| [`P09-ci-pipeline.md`](../prompts/P09-ci-pipeline.md) | W2 | زیرساخت |
| [`P05-agent-core.md`](../prompts/P05-agent-core.md) | W3 | Go |
| [`P06-api-core.md`](../prompts/P06-api-core.md) | W3 | NestJS |
| [`P07-ui-shell.md`](../prompts/P07-ui-shell.md) | W3 | React |
| [`P08-vertical-slice-interfaces.md`](../prompts/P08-vertical-slice-interfaces.md) | W4 | اسلایس عمودی |
| [`P10-packaging-deb.md`](../prompts/P10-packaging-deb.md) | W8 | بسته‌بندی |
| [`P11-strongswan-vpp.md`](../prompts/P11-strongswan-vpp.md) | W10 | بیلد |
| [`P12-frr-linuxcp.md`](../prompts/P12-frr-linuxcp.md) | W12 | مسیریابی |
| [`P13-cli-basic.md`](../prompts/P13-cli-basic.md) | W14 | CLI |
| [`P14-iso-installer.md`](../prompts/P14-iso-installer.md) | W14 | ISO |
| [`features/F-vlan-qinq.md`](../prompts/features/F-vlan-qinq.md) · [`features/F-nat44-ed.md`](../prompts/features/F-nat44-ed.md) | W5, W9 | نمونهٔ پرشدهٔ قالب |
| [`FEATURE-TEMPLATE.md`](../prompts/FEATURE-TEMPLATE.md) | W5+ | **قالب هر قابلیت جدید** — برای همهٔ F-* ها |
| [`REVIEW-PROMPT.md`](../prompts/REVIEW-PROMPT.md) | همه | پرامت ایجنت بازبین |
| [`INTEGRATOR-PROMPT.md`](../prompts/INTEGRATOR-PROMPT.md) | روزانه | پرامت ایجنت یکپارچه‌ساز |

پرامت‌ها انگلیسی هستند — ایجنت‌های کدنویس با انگلیسی دقیق‌تر کار می‌کنند و نام‌های فنی
همه انگلیسی‌اند. متن UI فارسی در فایل‌های i18n می‌آید.

---

## ۵. گیت‌های انسانی (چیزی که ایجنت نمی‌تواند برای خودش تأیید کند)

| هفته | انسان باید شخصاً ببیند |
|---|---|
| ۱ | بستهٔ ICMP واقعاً از VPP رد شد (`vppctl show int` و `trace`)، نه از کرنل میزبان |
| ۱ | `kill -9 vpp` و بازسازی خودکار |
| ۲ | ریبوت کامل VM و بازگشت پیکربندی؛ confirmed-commit واقعاً بعد از تایمر برمی‌گردد |
| ۳ | ESP روی خط دیده می‌شود (tcpdump بین دو VM)، نه ترافیک خام |
| ۳ | مسیر BGP در `vppctl show ip fib` نشسته است، نه فقط در `vtysh` |
| ۴ | ISO روی یک VM خام نصب می‌شود و بدون دست‌زدن بالا می‌آید |
| ۴ | ایجنت بازبین امنیتی: هیچ `exec`/`shell` با ورودی کاربر، هیچ secret در لاگ/GET |

---

## ۶. ریسک‌های مخصوص کار با ایجنت

| ریسک | نشانه | پادزهر |
|---|---|---|
| **API ساختگی VPP** | کد کامپایل می‌شود ولی VPP خطای «unknown message» می‌دهد | فقط binapi تولیدشده؛ CI با VPP واقعی |
| **تست تقلبی** | تست بدون VPP «پاس» می‌شود، assert روی mock خودش | قانون: تست قابلیت باید بسته را از VPP رد کند (`trace` یا شمارنده) |
| **انحراف بین ایجنت‌ها** | دو ایجنت دو مدل داده برای یک چیز | قراردادها قفل روز ۳؛ تغییر فقط با PR برچسب `contract` |
| **کیفیت ظاهری** | UI زیبا، پشتش کار نمی‌کند | DoD: هیچ صفحه‌ای بدون endpoint واقعی و رفتار واقعی VPP |
| **انفجار دامنه** | ایجنت «به‌صورت اضافی» ۵ قابلیت دیگر هم می‌سازد | هر پرامت بخش «نکن» دارد؛ بازبین آن را چک می‌کند |
| **اسرار** | PSK در لاگ، کلید در repo | بازبین امنیتی + secret scanning در CI |
| **خستگی انسان** | merge بدون خواندن | حداکثر ۱۲ ایجنت؛ اگر انسان نمی‌رسد، ایجنت کمتر |

---

## ۷. بعد از روز ۳۰

آنچه دارید: معماری کامل و اثبات‌شده، ~۴۰٪ از قابلیت‌های Tier-1، آزمایشگاه مجازی و CI.
آنچه می‌ماند از [برنامهٔ اصلی](08-master-schedule-fa.md): سخت‌افزار واقعی و کارایی
(D0.6 تیونینگ واقعی، D1.1 ماتریس NIC)، بقیهٔ T1 (CGNAT پایه، VRRP، A/B upgrade، CLI کامل)،
و کل D12 (کیفیت، امنیت، مستندات، گواهی). با ایجنت‌ها، بخش کدنویسی این باقی‌مانده ۳ تا ۵
برابر سریع‌تر می‌رود؛ بخش آزمایشگاه و اعتبارسنجی همان سرعت انسانی را دارد.

برآورد واقع‌بینانه تا محصول فروش‌پذیر (R1.0 GA-1) با همین مدل ایجنت‌محور: **۴ تا ۶ ماه**،
به‌شرط داشتن سخت‌افزار آزمایشگاه از هفتهٔ ۲.
