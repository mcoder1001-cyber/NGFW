"""NgfwSession against a mocked HTTP layer: auth header, URL/pointer encoding, problem+json → typed exceptions,
candidate → commit(confirm) → confirm, discard on failure, secrets never in repr/logs."""
from __future__ import annotations

import logging

import pytest

import ngfw
from ngfw.transport import HttpResponse

from .fake import FAKE_KEY, FakeTransport, problem

URL = "https://ngfw.test"


@pytest.fixture
def fake() -> FakeTransport:
    return FakeTransport()


SYNCED = (200, {"sync": {"state": "in-sync", "reason": ""}, "pendingCommit": None})


@pytest.fixture
def s(fake: FakeTransport) -> ngfw.NgfwSession:
    fake.on("GET", "/api/v1/state/system", SYNCED)
    return ngfw.NgfwSession(URL, FAKE_KEY, transport=fake)


def pending_commit(_seen):  # type: ignore[no-untyped-def]
    return 200, {"status": "pending", "txnId": "t1", "confirmDeadline": "2026-09-24T00:00:00Z", "results": [],
                 "warnings": [], "notApplied": []}


def test_api_key_header_and_no_key_in_repr(s: ngfw.NgfwSession, fake: FakeTransport) -> None:
    fake.on("GET", "/api/v1/config", (200, {}))
    s.running()
    assert fake.seen[0].headers["authorization"] == f"ApiKey {FAKE_KEY}"
    assert FAKE_KEY not in repr(s) and FAKE_KEY not in str(vars(s))
    assert repr(s) == "NgfwSession(url='https://ngfw.test', verify=True)"


def test_key_from_file_and_env(tmp_path, monkeypatch, fake: FakeTransport) -> None:  # type: ignore[no-untyped-def]
    f = tmp_path / "key"
    f.write_text(FAKE_KEY + "\n")
    assert ngfw.NgfwSession(URL, api_key_file=str(f), transport=fake)._key.get() == FAKE_KEY
    monkeypatch.setenv("NGFW_API_KEY", FAKE_KEY)
    assert ngfw.NgfwSession(URL, transport=fake)._key.get() == FAKE_KEY
    monkeypatch.delenv("NGFW_API_KEY")
    with pytest.raises(ValueError, match="no API key"):
        ngfw.NgfwSession(URL, transport=fake)


def test_rejects_credentials_in_url(fake: FakeTransport) -> None:
    with pytest.raises(ValueError, match="credentials in the URL"):
        ngfw.NgfwSession("https://admin:pw@ngfw.test", FAKE_KEY, transport=fake)


def test_tls_verification_is_on_by_default() -> None:
    import ssl

    t = ngfw.NgfwSession(URL, FAKE_KEY)._transport
    assert t._ctx.verify_mode == ssl.CERT_REQUIRED and t._ctx.check_hostname  # type: ignore[attr-defined]
    with pytest.warns(UserWarning, match="DISABLED"):
        ngfw.NgfwSession(URL, FAKE_KEY, verify=False)


def test_pointer_to_url_encoding(s: ngfw.NgfwSession, fake: FakeTransport) -> None:
    fake.on("PUT", "/api/v1/config/interfaces/TenGigabitEthernet0~10~10", (200, {"pointer": "x"}))
    fake.on("PATCH", "/api/v1/config/interfaces/a%20b", (200, {"pointer": "y"}))
    s.set(ngfw.pointer.join("interfaces", "TenGigabitEthernet0/0/0"), {"mtu": 1500})
    s.merge("interfaces/a b/", {"mtu": 9000})
    assert fake.calls() == ["PUT /api/v1/config/interfaces/TenGigabitEthernet0~10~10", "PATCH /api/v1/config/interfaces/a%20b"]
    assert fake.seen[0].headers["content-type"] == "application/json"
    with pytest.raises(ValueError):
        s.set("/", {})
    with pytest.raises(ValueError, match="escape"):
        s.set("/interfaces/bad~2", {})


def test_body_less_posts_send_no_content_type(s: ngfw.NgfwSession, fake: FakeTransport) -> None:
    # Fastify rejects an empty body with content-type: application/json (seen live)
    fake.on("POST", "/api/v1/config/commit", (200, {"status": "unchanged", "results": [], "warnings": [], "notApplied": []}))
    s.commit(comment="c")
    assert "content-type" not in fake.seen[0].headers
    assert fake.seen[0].query == {"comment": "c"}


def test_validation_problem_becomes_typed_error_with_pointer(s: ngfw.NgfwSession, fake: FakeTransport) -> None:
    fake.on("PUT", "/api/v1/config/interfaces/loop1", (400, problem(
        400, "Validation failed", "the document does not match the schema",
        [{"pointer": "/interfaces/loop1/ipv4/0", "message": "Invalid IPv4 range"}])))
    with pytest.raises(ngfw.ValidationError) as ei:
        s.set("/interfaces/loop1", {"ipv4": ["bad"]})
    e = ei.value
    assert e.status == 400 and e.pointer == "/interfaces/loop1/ipv4/0"
    assert e.errors == [ngfw.FieldError("/interfaces/loop1/ipv4/0", "Invalid IPv4 range")]
    assert "Invalid IPv4 range" in str(e) and FAKE_KEY not in str(e)


@pytest.mark.parametrize(("status", "cls"), [(401, ngfw.Unauthorized), (403, ngfw.Forbidden), (404, ngfw.NotFound),
                                             (409, ngfw.Conflict), (422, ngfw.CommitFailed), (503, ngfw.Unavailable),
                                             (500, ngfw.ApiError)])
def test_status_to_exception(s: ngfw.NgfwSession, fake: FakeTransport, status: int, cls: type) -> None:
    fake.on("GET", "/api/v1/config/diff", (status, problem(status, "x", lock={"owner": "bob"}, sync={"state": "unknown"})))
    with pytest.raises(cls) as ei:
        s.diff()
    if status == 409:
        assert ei.value.lock == {"owner": "bob"}
    if status == 503:
        assert ei.value.sync == {"state": "unknown"}


def test_non_json_error_body(s: ngfw.NgfwSession, fake: FakeTransport) -> None:
    fake.on("GET", "/api/v1/config", (502, HttpResponse(502, {"content-type": "text/html"}, b"<html>bad gateway</html>")))
    with pytest.raises(ngfw.Unavailable, match="bad gateway"):
        s.running()


def test_transaction_commits_with_confirm_then_confirms(s: ngfw.NgfwSession, fake: FakeTransport) -> None:
    diffs = iter([[], [{"op": "add", "pointer": "/interfaces/loop1"}]])
    fake.on("GET", "/api/v1/config/diff", lambda _: (200, {"baseRevision": 1, "changes": next(diffs)}))
    fake.on("PUT", "/api/v1/config/interfaces/loop1", (200, {"pointer": "/interfaces/loop1"}))
    fake.on("POST", "/api/v1/config/commit", pending_commit)
    fake.on("POST", "/api/v1/config/commit/confirm", (200, {"status": "confirmed", "revision": {"id": 2}, "notApplied": []}))
    with s.transaction(confirm=30, comment="add loop1") as tx:
        tx.set("/interfaces/loop1", {"ipv4": ["192.0.2.1/32"]})
    assert tx.result == {"status": "confirmed", "revision": {"id": 2}, "notApplied": []}
    assert fake.calls() == [
        "GET /api/v1/state/system", "GET /api/v1/config/diff", "PUT /api/v1/config/interfaces/loop1",
        "GET /api/v1/config/diff", "POST /api/v1/config/commit?comment=add loop1&confirm=30",
        "GET /api/v1/state/system", "POST /api/v1/config/commit/confirm"]


def test_transaction_refuses_dirty_candidate(s: ngfw.NgfwSession, fake: FakeTransport) -> None:
    fake.on("GET", "/api/v1/config/diff", (200, {"changes": [{"op": "add", "pointer": "/x"}]}))
    fake.on("GET", "/api/v1/config/lock", (200, {"locked": True, "owner": "alice"}))
    with pytest.raises(ngfw.NgfwError, match="1 uncommitted change.*alice"):
        with s.transaction():
            pass
    assert "POST /api/v1/config/discard" not in fake.calls()


def test_transaction_refuses_unsynced_appliance(s: ngfw.NgfwSession, fake: FakeTransport) -> None:
    """review M2: sync unknown/degraded or a pending commit → nothing is edited (unless allow_unsynced)."""
    fake.on("GET", "/api/v1/state/system", (200, {"sync": {"state": "unknown", "reason": "lost Apply answer"}}))
    with pytest.raises(ngfw.NotInSync, match="'unknown'"):
        with s.transaction() as tx:
            tx.set("/interfaces/loop1", {})
    assert not any(c.startswith("PUT") for c in fake.calls())
    fake.on("GET", "/api/v1/state/system", (200, {"sync": {"state": "in-sync"}, "pendingCommit": {"txnId": "t"}}))
    with pytest.raises(ngfw.NotInSync, match="pending"):
        with s.transaction():
            pass


def test_pending_answer_out_of_sync_is_not_confirmed(s: ngfw.NgfwSession, fake: FakeTransport) -> None:
    fake.on("POST", "/api/v1/config/commit", (200, {"status": "pending", "confirmDeadline": "d", "sync": {"state": "degraded"}}))
    with pytest.raises(ngfw.ConfirmError, match="sync is degraded"):
        s.commit_confirmed(confirm=10)
    assert "POST /api/v1/config/commit/confirm" not in fake.calls()


def test_concurrent_edit_is_neither_committed_nor_discarded(s: ngfw.NgfwSession, fake: FakeTransport) -> None:
    """review M3: another run of the same user staged /system/hostname meanwhile."""
    diffs = iter([[], [{"op": "add", "pointer": "/interfaces/loop1"}, {"op": "replace", "pointer": "/system/hostname"}]])
    fake.on("GET", "/api/v1/config/diff", lambda _: (200, {"changes": next(diffs)}))
    fake.on("PUT", "/api/v1/config/interfaces/loop1", (200, {}))
    with pytest.raises(ngfw.ConcurrentEdit, match="/system/hostname"):
        with s.transaction() as tx:
            tx.set("/interfaces/loop1", {})
    assert not any("commit" in c or "discard" in c for c in fake.calls())


def test_transaction_discards_only_its_own_changes(s: ngfw.NgfwSession, fake: FakeTransport) -> None:
    fake.on("POST", "/api/v1/config/discard", (200, {"discarded": True}))
    # exception inside the block: our change only → discarded
    diffs = iter([[], [{"op": "add", "pointer": "/interfaces/loop1"}]])
    fake.on("GET", "/api/v1/config/diff", lambda _: (200, {"changes": next(diffs)}))
    fake.on("PUT", "/api/v1/config/interfaces/loop1", (200, {}))
    with pytest.raises(RuntimeError):
        with s.transaction() as tx:
            tx.set("/interfaces/loop1", {})
            raise RuntimeError("boom")
    assert fake.calls()[-1] == "POST /api/v1/config/discard"

    # exception while another run's change is also in the candidate → NOT discarded
    fake.seen.clear()
    diffs = iter([[], [{"op": "add", "pointer": "/interfaces/loop1"}, {"op": "add", "pointer": "/nat/x"}]])
    with pytest.raises(RuntimeError):
        with s.transaction() as tx:
            tx.set("/interfaces/loop1", {})
            raise RuntimeError("boom")
    assert "POST /api/v1/config/discard" not in fake.calls()

    # failed commit (validation) → running untouched, our own change discarded
    fake.seen.clear()
    diffs = iter([[], [{"op": "add", "pointer": "/interfaces/loop1"}], [{"op": "add", "pointer": "/interfaces/loop1"}]])
    fake.on("POST", "/api/v1/config/commit", (400, problem(400, "Validation failed", errors=[
        {"pointer": "/interfaces/loop1/ipv4/0", "message": "overlaps /interfaces/loop2/ipv4/0"}])))
    with pytest.raises(ngfw.BadRequest) as ei:
        with s.transaction() as tx:
            tx.set("/interfaces/loop1", {})
    assert ei.value.pointer == "/interfaces/loop1/ipv4/0"
    assert fake.calls()[-1] == "POST /api/v1/config/discard"


def test_failed_check_does_not_confirm(s: ngfw.NgfwSession, fake: FakeTransport) -> None:
    fake.on("POST", "/api/v1/config/commit", pending_commit)

    def unreachable(_s: ngfw.NgfwSession) -> None:
        raise ngfw.TransportError("management path lost")

    with pytest.raises(ngfw.ConfirmError, match="NOT confirmed.*discard") as ei:
        s.commit_confirmed(confirm=10, check=unreachable)
    assert ei.value.commit is not None and ei.value.commit["status"] == "pending"
    assert "POST /api/v1/config/commit/confirm" not in fake.calls()


def test_not_enforced_warns_or_raises(s: ngfw.NgfwSession, fake: FakeTransport) -> None:
    """review M1: notApplied from the pending answer survives the confirm; stored-but-not-enforced is never silent."""
    fake.on("POST", "/api/v1/config/commit", (200, {"status": "pending", "confirmDeadline": "d", "notApplied": ["nat"],
                                                    "sync": {"state": "in-sync"}}))
    fake.on("POST", "/api/v1/config/commit/confirm", (200, {"status": "confirmed", "notApplied": []}))
    with pytest.warns(ngfw.NotEnforcedWarning, match=r"\['nat'\]"):
        r = s.commit_confirmed(confirm=10)
    assert r["notApplied"] == ["nat"]
    with pytest.raises(ngfw.NotEnforced) as ei:
        s.commit_confirmed(confirm=10, require_enforced=True)
    assert ei.value.result["notApplied"] == ["nat"]
    fake.on("POST", "/api/v1/config/commit", (200, {"status": "partially-applied", "notApplied": ["vpn"]}))
    with pytest.raises(ngfw.NotEnforced, match="partially-applied"):
        s.commit_confirmed(confirm=10, require_enforced=True)


def test_http_refused_for_remote_hosts(fake: FakeTransport) -> None:
    """review L2."""
    with pytest.raises(ValueError, match="refusing plain http"):
        ngfw.NgfwSession("http://ngfw-a.example", FAKE_KEY, transport=fake)
    with pytest.warns(UserWarning, match="without TLS"):
        ngfw.NgfwSession("http://ngfw-a.example", FAKE_KEY, transport=fake, allow_http=True)
    ngfw.NgfwSession("http://127.0.0.1:3500", FAKE_KEY, transport=fake)
    ngfw.NgfwSession("http://[::1]:3500", FAKE_KEY, transport=fake)


def test_unchanged_transaction_commits_nothing(s: ngfw.NgfwSession, fake: FakeTransport) -> None:
    fake.on("GET", "/api/v1/config/diff", (200, {"changes": []}))
    with s.transaction() as tx:
        pass
    assert tx.result == {"status": "unchanged", "notApplied": []}
    assert not any("commit" in c or "discard" in c for c in fake.calls())


def test_rollback_and_revisions(s: ngfw.NgfwSession, fake: FakeTransport) -> None:
    fake.on("POST", "/api/v1/config/rollback/7", (200, {"status": "applied", "revision": {"id": 9}}))
    fake.on("GET", "/api/v1/config/revisions", (200, {"items": [{"id": 9}], "total": 1}))
    assert s.rollback(7, comment="back")["revision"]["id"] == 9
    assert s.revisions(limit=5) == [{"id": 9}]
    assert fake.calls() == ["POST /api/v1/config/rollback/7?comment=back", "GET /api/v1/config/revisions?limit=5&offset=0"]


def test_logs_never_carry_key_or_write_only_values(s: ngfw.NgfwSession, fake: FakeTransport, caplog) -> None:  # type: ignore[no-untyped-def]
    s.log_bodies = True
    hashed = "$argon2id$v=19$m=65536,t=3,p=4$NGFW_TEST_PSK_hash"
    fake.on("PUT", "/api/v1/config/management/users", (200, {}))
    fake.on("POST", "/api/v1/secrets", (200, {"ref": "psk/site-a", "created": True, "version": 1}))
    with caplog.at_level(logging.DEBUG, logger="ngfw"):
        s.set("/management/users", [{"username": "ops", "role": "operator", "passwordHash": hashed}])
        s.put_secret("psk", "site-a", "NGFW_TEST_PSK_site_a")
    text = caplog.text
    assert "PUT /api/v1/config/management/users → 200" in text
    assert '"passwordHash": "<redacted>"' in text and "ops" in text
    assert hashed not in text and "NGFW_TEST_PSK_site_a" not in text and FAKE_KEY not in text
    assert fake.seen[0].body[0]["passwordHash"] == hashed  # the API still gets it


def test_redact_helpers() -> None:
    doc = {"management": {"users": [{"username": "a", "passwordHash": "h"}, {"username": "b"}]}}
    assert ngfw.secret_pointers(doc) == ["/management/users/0/passwordHash"]
    red = ngfw.redact(doc)
    assert red["management"]["users"][0]["passwordHash"] == ngfw.REDACTED and doc["management"]["users"][0]["passwordHash"] == "h"
    assert ngfw.redact({"username": "a", "passwordHash": "h"}, "/management/users/3") == {"username": "a", "passwordHash": ngfw.REDACTED}
    assert ngfw.redact("h", "/management/users/0/passwordHash") == ngfw.REDACTED
    assert ngfw.redact({"x": 1}, "/interfaces/loop1") == {"x": 1}
