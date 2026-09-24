"""Live run against a real VRX API + vrx-agent + VPP on the caller's slot (P06 e2e pattern).

    eval "$(tools/lab env 5)"; test/topology/sdk-terraform-ansible/live.sh run sdk/python/.venv/bin/pytest -s sdk/python/tests/test_live.py

Skipped unless VRX_INTEGRATION=1 and VRX_SDK_URL / VRX_SDK_API_KEY_FILE are set (live.sh exports them). Objects carry
the slot prefix: loop<slot>11 on 10.<slot>.111.0/24 (docs/lab/shared-host-rules.md); everything is removed at the end.
"""
from __future__ import annotations

import logging
import os
import re
import shutil
import subprocess

import pytest

import vrx

URL = os.environ.get("VRX_SDK_URL")
KEY_FILE = os.environ.get("VRX_SDK_API_KEY_FILE")
pytestmark = pytest.mark.skipif(
    os.environ.get("VRX_INTEGRATION") != "1" or not URL or not KEY_FILE,
    reason="live run: VRX_INTEGRATION=1 + VRX_SDK_URL + VRX_SDK_API_KEY_FILE (test/topology/sdk-terraform-ansible/live.sh run …)",
)
SLOT = int(re.sub(r"\D", "", os.environ.get("VRX_TEST_PREFIX", "w5")) or 5)
IF = f"loop{SLOT}11"
PTR = vrx.pointer.join("interfaces", IF)


def addr(n: int) -> str:
    return f"10.{SLOT}.111.{n}/24"


def state_of(s: vrx.VrxSession) -> dict | None:
    return next((i for i in s.state("interfaces")["items"] if i["name"] == IF), None)


def vpp_addrs() -> str:
    if not shutil.which("vppctl"):
        return "(vppctl not available)"
    out = subprocess.run(["vppctl", "show", "int", "addr"], capture_output=True, text=True, timeout=10).stdout
    lines = out.splitlines()
    keep = [ln for i, ln in enumerate(lines) if ln.startswith(IF) or (i and lines[i - 1].startswith(IF))]
    return "\n".join(keep) or f"(no {IF} in `vppctl show int addr`)"


@pytest.fixture(scope="module")
def s() -> vrx.VrxSession:
    sess = vrx.VrxSession(URL or "", api_key_file=KEY_FILE, timeout=60)
    if sess.pending():
        pytest.fail("a confirmed commit is pending on this API — not touching it")
    if sess.exists(PTR):  # leftover of an interrupted run: remove it first
        with sess.transaction(confirm=None, comment="sdk live: pre-clean", allow_dirty=True) as tx:
            tx.delete(PTR)
    yield sess
    if sess.exists(PTR):
        sess.discard()
        with sess.transaction(confirm=None, comment="sdk live: cleanup") as tx:
            tx.delete(PTR)


def test_live_candidate_commit_confirm_state_rollback(s: vrx.VrxSession, caplog) -> None:  # type: ignore[no-untyped-def]
    s.log_bodies = True
    caplog.set_level(logging.DEBUG, logger="vrx")
    print(f"\n[live] {s!r} · API {s.health_health()['version']} · agent owner {s.state('system')['agent'].get('owner')}")

    # 1. candidate edit → diff → confirmed commit (confirm=30 s, checked, confirmed)
    with s.transaction(confirm=30, comment=f"sdk live: add {IF}") as tx:
        tx.set(PTR, {"enabled": True, "ipv4": [addr(1)]})
        print("[live] diff:", [(c["op"], c["pointer"], c.get("to")) for c in tx.diff()])
    r1 = tx.result or {}
    print(f"[live] commit → status={r1['status']} revision={r1['revision']['id']} txn={r1['txnId']}")
    assert r1["status"] == "confirmed"
    rev_a = r1["revision"]["id"]
    st = state_of(s)
    print(f"[live] /state/interfaces {IF}: config={st and st['config']} swIfIndex={st and (st['counters'] or {}).get('swIfIndex')}")
    assert st is not None and st["config"]["ipv4"] == [addr(1)]
    print("[live] vppctl show int addr:\n" + vpp_addrs())

    # 2. change the address (merge patch) → second revision
    with s.transaction(confirm=30, comment=f"sdk live: readdress {IF}") as tx:
        tx.merge(PTR, {"ipv4": [addr(2)]})
    r2 = tx.result or {}
    print(f"[live] commit → status={r2['status']} revision={r2['revision']['id']}")
    assert (state_of(s) or {})["config"]["ipv4"] == [addr(2)]

    # 3. rollback to the first revision (itself a confirmed commit) → the old address is back
    rb = s.rollback(rev_a, confirm=30, comment="sdk live: rollback")
    assert rb["status"] == "pending"
    rbc = s.confirm()
    print(f"[live] rollback/{rev_a} → {rb['status']} → confirm → {rbc['status']} revision={rbc['revision']['id']}")
    st = state_of(s)
    print(f"[live] /state/interfaces {IF} after rollback: config={st and st['config']}")
    assert st is not None and st["config"]["ipv4"] == [addr(1)]
    print("[live] vppctl show int addr:\n" + vpp_addrs())

    # 4. validation failure → typed exception with the pointer; the candidate is discarded
    with pytest.raises(vrx.ValidationError) as ei:
        with s.transaction(confirm=30) as tx:
            tx.merge(PTR, {"ipv4": ["10.999.0.1/24"]})
    print(f"[live] invalid edit → {type(ei.value).__name__} pointer={ei.value.pointer} ({ei.value.errors[0].message})")
    assert ei.value.pointer == f"{PTR}/ipv4/0"
    assert s.diff() == []

    # 5. remove it → nothing left in running, state or VPP
    with s.transaction(confirm=30, comment=f"sdk live: remove {IF}") as tx:
        tx.delete(PTR)
    print(f"[live] delete → {(tx.result or {})['status']}; in running: {s.exists(PTR)}; in state: {state_of(s) is not None}")
    assert not s.exists(PTR) and state_of(s) is None
    print("[live] vppctl show int addr:\n" + vpp_addrs())

    # the SDK log saw every request, never the key
    key = open(KEY_FILE or "", encoding="utf-8").read().strip()
    assert key not in caplog.text and "PUT /api/v1/config/interfaces/" in caplog.text
    print(f"[live] {len(caplog.records)} SDK log records, API key present in them: {key in caplog.text}")
