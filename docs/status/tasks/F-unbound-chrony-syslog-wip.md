# F-unbound-chrony-syslog — WIP

Started 2026-09-24 23:45 (+0330), slot 10 (w10).

- [x] envelope committed
- [x] contract(schema) + contract(proto) (SyslogTarget 6–9, ActionRequest 7, 4 state RPCs)
- [ ] agent: renderer singletons (unbound.config / chrony.config / rsyslog.config), projection + assemble, vppCache → dns.* (globals owner)
- [ ] agent: DnsState / NtpState / SyslogState / SyslogEntries RPCs, dns_lookup action
- [ ] API: state + action routes, fake, e2e
- [ ] UI: Services → DNS / NTP / Logging tabs, en + fa
- [ ] evidence: slot instances, restart simulation, rollback, 400, screenshot
- [ ] docs, status, CI
