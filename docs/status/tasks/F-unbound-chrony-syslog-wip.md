# F-unbound-chrony-syslog — WIP

Started 2026-09-24 23:45 (+0330), slot 10 (w10). State 2026-09-25 09:40: **done**, pending the F-kea fold (Q5).

- [x] envelope committed
- [x] contract(schema) + contract(proto) (SyslogTarget 6–9, ActionRequest 7, 4 state RPCs)
- [x] agent: renderer singletons, projection + assemble, vppCache → dns.* (owner applies, others require), TD-11b declarations
- [x] agent: DnsState / NtpState / SyslogState / SyslogEntries, dns_lookup (guarded, D-137)
- [x] API: state + action routes, fake, e2e
- [x] UI: Services › DNS / NTP / Logging, en + fa, screenshots
- [x] evidence: slot end-to-end run (resolver, chrony, rsyslog, restart simulation, rollback, 400), CI green
- [x] docs, status (`F-unbound-chrony-syslog.md`), questions Q1–Q8
- [ ] fold with F-kea-dhcp-relay once it is on main (merge main, keep one ServicesImplemented/Services const/Domains entry/nav item)
