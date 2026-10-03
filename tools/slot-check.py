#!/usr/bin/env python3
"""tools/slot-check.py - prove the shared-host slot scheme is collision-free (D-156, docs/lab/shared-host-rules.md §1).

Slots: 1-11 workers, 12 CI, 13 reserved (13000-13999 = tools/app), 14-32 workers -> 30 developer slots.
For every slot it derives every host resource the repo's tests and tools use (the exports of `tools/lab env`
plus the hard-coded sub-port formulas found in the tests) and fails when two slots - or a slot and a fixed
host service - share a port, a VPP id range, a database or a rig subnet. It also checks that `tools/lab env <N>`
prints exactly the formula below. Run: python3 tools/slot-check.py   (tools/ci.sh check/quick run it).
"""
import os
import re
import subprocess
import sys

WORKER_SLOTS = list(range(1, 12)) + list(range(14, 33))
CI_SLOT = 12
ALL_SLOTS = sorted(WORKER_SLOTS + [CI_SLOT])
RESERVED_TABLES = {"tools/app": (13000, 13999)}
# ports owned by the integrated main build and host services on the shared host
FIXED_PORTS = {22: "ssh", 53: "dns", 3000: "main api", 5173: "main vite", 8080: "tools/app web", 9101: "main metrics",
               5432: "postgresql", 6379: "valkey", 9090: "prometheus", 9100: "node exporter"}
EPHEMERAL_LO = 32768  # Linux ip_local_port_range start: a listener above it can race outgoing connections


def http_port(n):
    return 3000 + 100 * n if n <= 12 else 10000 + 100 * n


def web_port(n):
    return 5000 + 100 * n if n <= 12 else 14000 + 100 * n


def lab_env(n):
    return {
        "NGFW_SLOT": str(n), "NGFW_TEST_PREFIX": f"w{n}", "NGFW_HTTP_PORT": str(http_port(n)),
        "NGFW_WEB_PORT": str(web_port(n)), "NGFW_METRICS_PORT": str(9100 + 10 * n + 1),
        "NGFW_AGENT_SOCKET": f"/run/ngfw-test/w{n}/agent.sock", "NGFW_PG_DATABASE": f"ngfw_w{n}",
        "NGFW_VALKEY_DB": str(n), "NGFW_VPP_TABLE_BASE": str(1000 * n),
    }


def ports(n):
    """every TCP/UDP port slot n may listen on, with where the formula lives"""
    p = {}
    h, w = http_port(n), web_port(n)
    p[h] = "NGFW_HTTP_PORT"
    p[w] = "NGFW_WEB_PORT"
    p[9100 + 10 * n + 1] = "NGFW_METRICS_PORT"
    for off, where in ((60, "srv6 api"), (70, "wireguard api")):
        p[h + off] = where
    for off, where in ((60, "srv6 web"), (70, "wireguard web")):
        p[w + off] = where
    # daemon test sub-ports. Slots 1-12 keep their legacy formulas: numeric 3000 + 100*N + x (unbound 53, chrony 23,
    # ucs/system-identity 16/23/53/54) and string-built "3<N><xx>" (rsyslog 14-17, snmpd 61-63, ipfix 71, hoststack 90-99).
    # Slots 14-32 take NGFW_HTTP_PORT + x instead (vpptest.SubPort; the legacy formulas collide there).
    legacy_num, legacy_str = (16, 23, 53, 54), (14, 15, 16, 17, 61, 62, 63, 71) + tuple(range(90, 100))
    if n <= 12:
        for off in legacy_num:
            p.setdefault(3000 + 100 * n + off, f"3000+100N+{off}")
        for off in legacy_str:
            p.setdefault(int(f"3{n}{off:02d}"), f'"3<N>{off:02d}"')
    else:
        for off in legacy_num + legacy_str:
            p.setdefault(h + off, f"SubPort NGFW_HTTP_PORT+{off}")
    for off in (10, 11):
        p[20000 + 100 * n + off] = f"wireguard 20000+100N+{off}"
    p.setdefault(4739 + n, "ipfix collector 4739+N")
    return p


def main():
    errs, warns = [], []
    owner = {}
    for n in ALL_SLOTS:
        for port, where in ports(n).items():
            if not 1024 <= port < 65536:
                errs.append(f"slot {n}: {where} = {port} is not a valid unprivileged port")
            if port in FIXED_PORTS:
                errs.append(f"slot {n}: {where} = {port} collides with {FIXED_PORTS[port]}")
            if port >= EPHEMERAL_LO and not where.startswith('"3<N>'):
                errs.append(f"slot {n}: {where} = {port} is in the ephemeral range")
            elif port >= EPHEMERAL_LO:
                warns.append(f"slot {n}: {where} = {port} (legacy string formula) is in the ephemeral range")
            if port in owner and owner[port][0] != n:
                errs.append(f"port {port}: slot {owner[port][0]} ({owner[port][1]}) and slot {n} ({where})")
            owner.setdefault(port, (n, where))
    ranges = [(1000 * n, 1000 * n + 999, f"slot {n}") for n in ALL_SLOTS]
    ranges += [(lo, hi, name) for name, (lo, hi) in RESERVED_TABLES.items()]
    ranges.sort()
    for (a_lo, a_hi, a), (b_lo, b_hi, b) in zip(ranges, ranges[1:]):
        if b_lo <= a_hi:
            errs.append(f"VPP id ranges overlap: {a} {a_lo}-{a_hi} and {b} {b_lo}-{b_hi}")
    for n in ALL_SLOTS:
        if len(f"w{n}") > 6:
            errs.append(f"slot {n}: prefix too long for IFNAMSIZ")
        if not 1 <= n <= 254:
            errs.append(f"slot {n}: rig subnet 10.{n}.0.0/16 invalid")
    if len(WORKER_SLOTS) != 30:
        errs.append(f"expected 30 developer slots, have {len(WORKER_SLOTS)}")

    lab = os.path.join(os.path.dirname(os.path.abspath(__file__)), "lab")
    checked = 0
    if os.access(lab, os.X_OK):
        for n in ALL_SLOTS:
            out = subprocess.run([lab, "env", str(n)], capture_output=True, text=True, check=False)
            got = dict(re.findall(r"^export (NGFW_[A-Z_]+)=(\S*)$", out.stdout, re.M))
            for k, v in lab_env(n).items():
                if got.get(k) != v:
                    errs.append(f"tools/lab env {n}: {k}={got.get(k)!r}, expected {v!r}")
            checked += 1
        for bad in (0, 13, 33):
            if subprocess.run([lab, "env", str(bad)], capture_output=True, check=False).returncode == 0:
                errs.append(f"tools/lab env {bad} must be refused")
    for w in warns:
        print(f"note: {w}")
    if errs:
        print("\n".join(errs))
        return 1
    print(f"ok: {len(WORKER_SLOTS)} developer slots + CI slot {CI_SLOT}; {len(owner)} ports, "
          f"{len(ranges)} id ranges, no collision; tools/lab env verified for {checked} slots")
    return 0


if __name__ == "__main__":
    sys.exit(main())
