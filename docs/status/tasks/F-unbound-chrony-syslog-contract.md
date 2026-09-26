# F-unbound-chrony-syslog — contract changes (additive)

Committed first on `task/F-unbound-chrony-syslog` as `contract(schema): …` and `contract(proto): …` (envelope: never a
`contract/` branch; P08 pattern). Numbers only from wave-A-hotspots §2 / wave-BC-numbers "Batch-2 follow-ons".

## Schema (`packages/schema`)

| change | where | why |
|---|---|---|
| `management.syslog[i].facilities` (array of the 20 syslog facility names, optional; empty/absent = all) | `domains/ext/syslog.ts` → one spread line in `SyslogServerSchema` (SY4; no anchor was seeded there) | D-086: RF-4 stand-in → contract |
| `management.syslog[i].format` (`rfc5424` \| `rfc3164`, optional; absent = rfc5424) | same | D-086 |
| `management.syslog[i].queueSize` (int 100..1000000, optional; absent = 10000) | same | D-086 |
| `management.syslog[i].tls` (`{caRef cert/…, certRef?, keyRef?, authMode? x509/name\|x509/certvalid, permittedPeers?}`; certRef/keyRef together) | same | D-086; the task's "syslog TLS requires a CA ref" |
| rule `services.unbound-chrony-syslog-vpp-cache-port` | `semantic/unbound-chrony-syslog.ts` (+ one import, one spread under the C2 anchors) | "VPP cache and an Unbound resolver must not listen on the same address:port" — VPP's dns plugin registers UDP 53 for all its addresses (`dns.c` `udp_register_dst_port(UDP_DST_PORT_dns)`), and resolver listen addresses are VPP addresses or wildcards (`services.bind-address-configured`), so every enabled port-53 listener conflicts |
| rule `services.unbound-chrony-syslog-forwarder-loop` | same | forward-zone forwarders / other resolvers' listen sockets (one Unbound instance serves all resolvers); the schema already covers a resolver's own default forwarders |
| rule `management.unbound-chrony-syslog-tls` | same | protocol tls ⇒ `tls` (whose `caRef` is required); `tls` on udp/tcp rejected |

Every new key is optional without a default, so documents written before the change parse to exactly the same value
(unit test `absent keys parse to exactly the pre-D-086 value`). No log-explorer retention leaf: the explorer reads
journald through bounded queries (retention is journald's own, P10), so the UI needs no document leaf.

## Proto (`packages/proto/vrx/v1/dataplane.proto`)

| change | number | source |
|---|---|---|
| `SyslogTarget.facilities` (repeated string) | 6 | wave-A-hotspots §2 / wave-BC-numbers batch-2 |
| `SyslogTarget.format` (optional string) | 7 | same |
| `SyslogTarget.queue_size` (optional uint32) | 8 | same |
| `SyslogTarget.tls` (`SyslogTls`) | 9 | same |
| `ActionRequest.dns_lookup` (`DnsLookupAction{name, timeout_ms}`) | oneof 7 | same |
| rpc `DnsState`, `NtpState`, `SyslogState`, `SyslogEntries` | — (no number) | envelope: "DNS/NTP/log state rpcs" |
| new messages in `// ----- F-unbound-chrony-syslog -----`: `SyslogTls`, `DnsLookupAction`, `ServiceDaemonAction`, `DnsState{Request,Response}`, `DnsZoneState`, `DnsLocalZoneState`, `DnsVppCacheState`, `NtpState{Request,Response}`, `NtpTracking`, `NtpSource`, `NtpSourceStats`, `SyslogState{Request,Response}`, `SyslogTargetState`, `SyslogEntries{Request,Response}`, `SyslogEntry` | own numbers from 1 | §0 rule 5 (names start with the feature noun; `ServiceDaemonAction` is shared by the three state responses) |

`buf lint` clean; generated Go/TS and `packages/api-client/src/generated/schema.d.ts` regenerated with `pnpm gen`.
Drift corpus: `packages/proto/test/fixtures/unbound-chrony-syslog-full.json` (C4) exercises every new field.
`docs/contracts/proto.md` §11 has the `### F-unbound-chrony-syslog: …` section. The fake agent answers the four RPCs
with UNIMPLEMENTED in the contract commit (P5); the feature commit replaces them with `features/unbound-chrony-syslog/fake.ts`.
