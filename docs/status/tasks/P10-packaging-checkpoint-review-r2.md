# Independent P10 checkpoint fix verification, round 2

Reviewed frozen review SHA `b7ca10b7e4facfacbe1d39c00cce1b472b46827f`; product changes correspond to developer `4ca988d79e74b62149c5029691e72e69f22f0204`, remote checkpoint `10b445` (manager-provided abbreviated ref). Original BLOCK report against `ff6223db` is preserved. Reviewer wrote no product code.

## Findings resolved

1. Narrow agent ReadWritePaths now includes actual capture `/var/lib/vrx/captures` and rsyslog TLS `/etc/vrx/rsyslog-tls`. Postinst provisions both root-owned with 0700/0750 respectively; existing privilege/capability/address-family restrictions remain. Regression checks compare against actual consumer defaults. Original missing-path MAJOR resolved by source inspection; installed sandbox execution remains acceptance not run.
2. Debian install mappings now place startupgen and vppcheck under `/usr/lib/vrx/bin`, exactly matching shipped apply-startup product default. The agent stays `/usr/sbin/vrx-agent`; staging locations do not change runtime mapping. Regression reads the actual product script default. Original tool-discovery MAJOR resolved.

## TLS/proxy delta review

TLS bootstrap serializes with flock, generates RSA3072 under private temporary directory, preserves an existing matching nonsymlink key/certificate pair, rejects incomplete or mismatched pairs, and publishes key 0600/certificate 0644. Interrupted two-file publication explicitly fails closed on retry. No credentials or key contents are printed/committed. Package scripts do not run bootstrap or enable services; later firstboot must invoke it. HTTP listener only redirects; HTTPS proxies API with WebSocket upgrade, RESTCONF and host-meta to loopback API. Forwarded-for is set from actual peer instead of retaining client-supplied chains. Nginx config installation is scaffolding, not evidence an existing default site is disabled or that HTTPS is running.

R1/R2/R7/R8: **APPROVE this checkpoint**; no unresolved BLOCKER/MAJOR in reviewed delta. This is NOT full P10 completion or release approval. Firstboot, signed APT publication, base policy, copyright/licensing, actual packages, chroot lifecycle and hosted full quick still require implementation/acceptance.

## Actual independent verification

```text
python3 deploy/debian/vrx/tests/test_packaging.py
Ran 7 tests in 0.851s
OK
```

Those tests execute real OpenSSL first-run generation in a private reviewer temporary directory, retry without replacement, verify 0600 key permissions and preserve the key when an incomplete pair is rejected. They do not start nginx or use /etc.

```text
bash -n deploy/debian/vrx/prepare.sh deploy/debian/vrx/assets/tls-bootstrap.sh
exit 0
tools/ci.sh check --base origin/main
ok: gitleaks — scanned ~24193 bytes (24.19 KB) in 155ms no leaks found
check PASSED (0m02s)
git diff --check
exit 0, no output
```

No daemon starts, host package installs, global firewall/config changes, systemd sandbox execution, nginx -t, full CI, .deb lifecycle or lab PASS is asserted. No public/private TLS material retained in the report.
