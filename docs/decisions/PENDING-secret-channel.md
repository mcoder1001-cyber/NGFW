# PENDING: secret-channel

- raised: 2026-09-24 15:00 by the manager (ngfw-46), from the wave-A prep: the P11 envelope, the F-wireguard/P12/F-unbound prompts, and P08's note "P11 adds the secret resolver"
- decision: **<empty until the product owner fills it>**
- parked tasks: none as whole tasks. Only the **end-to-end secret steps** wait: P11 (IKE PSK/private key), F-wireguard (private key), P12 (`passwordRef`), F-unbound-chrony-syslog (TLS key), and since D-119 also F-ikev2-native, F-pki, F-ra-vpn, F-snmp (community/USM keys), F-ospf/F-isis-rip/F-bfd/F-mpls-ldp (auth keys), F-host-stack, F-vrrp-config-sync (keepalived auth + cluster sync key). Their schema, descriptors, renderers, UI and tests proceed with a slot-local `vpn.MapResolver` fixture (`VRX_TEST_PSK_<id>_*`).

## Context
The API stores secrets encrypted in PostgreSQL: AES-GCM, with the secret name as AAD (D-091). It strips secret leaves before sending desired state to the agent (D-040), and `desired.pb` must never hold plaintext (rule 10). The agent needs the plaintext to render swanctl/WireGuard/FRR/TLS files. Nothing carries secret material from the API to the agent today. `docs/04-api-datamodel.md` specifies only the `secret` table, not the channel. That makes this a security-boundary and secret-storage decision (decision-policy always-ask #4).

## Options
| # | Option | Cost now | Reversal | Risk |
|---|---|---|---|---|
| 1 | **API push:** the commit RPC carries the resolved secrets in a separate, never-persisted request field (`SecretBundle`, redacted in fmt/slog). The agent keeps them in memory **and** in an agent-local sealed cache (0600, a key under the agent state dir, like D-096) so it can reconcile after its own restart without the API | 6 h | 4 h | low: the channel is the root:vrx 0660 agent socket (server.go:122, VRX_SOCKET_GROUP=vrx); only the API runs in group vrx; plaintext exists in the agent only, the same as it must for rendering. Retrieve, DryRun and error messages never echo SecretBundle contents |
| 2 | **Agent pull:** the agent calls a new API endpoint over a local unix socket (`ResolveSecret(ref)`) whenever it renders | 8 h | 6 h | medium: a new agent→API trust direction and endpoint; the agent cannot reconcile secret-bearing config while the API is down |
| 3 | **Agent resolver:** the agent reads ciphertext from PostgreSQL itself and holds the decryption key | 5 h | 8 h | high: widens the agent's privileges (DB credentials + master key), two processes can decrypt everything |

## Recommendation
**Option 1.** It keeps one trust direction (API → agent over the root:vrx 0660 agent socket, server.go:122, VRX_SOCKET_GROUP=vrx; only the API runs in group vrx) and keeps the master key in the API only. Retrieve, DryRun and error messages never echo SecretBundle contents. The sealed cache is needed because of restart safety: the agent must rebuild the data plane after `kill -9 vpp` or its own restart without the API. It is built once, in P11, and F-wireguard, P12 and F-unbound consume it.

## What continues meanwhile
Everything, except the end-to-end secret steps named above. Workers build against the `vpn.Resolver` interface with the fixture resolver. The architecture audit's ARCH-07 cleanup (6 resolver interfaces; Go regexes looser than the Zod ones) is done together with the answer.

_Refreshed 2026-09-24 (D-125): the socket wording now matches the code, and the no-echo rule was added. Nothing else changed._

## خلاصهٔ فارسی
- **مسئله:** کلیدها و رمزهای VPN (مثل PSK و کلید خصوصی) در API رمزنگاری‌شده ذخیره می‌شوند، ولی ایجنت برای نوشتن فایل‌های strongSwan، WireGuard و FRR به متن ساده‌شان نیاز دارد. هنوز هیچ کانالی این اسرار را از API به ایجنت نمی‌رساند.
- **دلیل نیاز به تصمیم شما:** این یک مرز امنیتی است، پس طبق سیاست تصمیم‌گیری با شماست.
- **پیشنهاد من، گزینهٔ ۱:** API موقع commit اسرار را در یک فیلد جداگانه که هیچ‌جا ذخیره نمی‌شود به ایجنت می‌فرستد. این کار از راه سوکت ایجنت با دسترسی root:vrx 0660 انجام می‌شود و فقط API در گروه vrx است. Retrieve، DryRun و پیام‌های خطا هرگز محتوای اسرار را برنمی‌گردانند. ایجنت آن‌ها را در یک کش محلیِ مهروموم‌شده نگه می‌دارد تا بعد از ری‌استارت هم بدون API کار کند. کلید اصلی فقط پیش API می‌ماند.
- **چه چیزی منتظر می‌ماند:** فقط مرحلهٔ end-to-end اسرار در P11، F-wireguard، P12 و F-unbound. بقیهٔ کارها ادامه دارد.
