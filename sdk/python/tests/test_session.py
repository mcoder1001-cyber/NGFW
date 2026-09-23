"""VrxSession against a mocked HTTP layer: auth header, URL/pointer encoding, problem+json → typed exceptions,
candidate → commit(confirm) → confirm, discard on failure, secrets never in repr/logs."""
from __future__ import annotations

import logging

import pytest

import vrx
from vrx.transport import HttpResponse

from .fake import FAKE_KEY, FakeTransport, problem

URL = "https://vrx.test"


@pytest.fixture
def fake() -> FakeTransport:
    return FakeTransport()


@pytest.fixture
def s(fake: FakeTransport) -> vrx.VrxSession:
    return vrx.VrxSession(URL, FAKE_KEY, transport=fake)


def pending_commit(_seen):  # type: ignore[no-untyped-def]
    return 200, {"status": "pending", "txnId": "t1", "confirmDeadline": "2026-09-24T00:00:00Z", "results": [],
                 "warnings": [], "notApplied": []}


def test_api_key_header_and_no_key_in_repr(s: vrx.VrxSession, fake: FakeTransport) -> None:
    fake.on("GET", "/api/v1/config", (200, {}))
    s.running()
    assert fake.seen[0].headers["authorization"] == f"ApiKey {FAKE_KEY}"
    assert FAKE_KEY not in repr(s) and FAKE_KEY not in str(vars(s))
    assert repr(s) == "VrxSession(url='https://vrx.test', verify=True)"


def test_key_from_file_and_env(tmp_path, monkeypatch, fake: FakeTransport) -> None:  # type: ignore[no-untyped-def]
    f = tmp_path / "key"
    f.write_text(FAKE_KEY + "\n")
    assert vrx.VrxSession(URL, api_key_file=str(f), transport=fake)._key.get() == FAKE_KEY
    monkeypatch.setenv("VRX_API_KEY", FAKE_KEY)
    assert vrx.VrxSession(URL, transport=fake)._key.get() == FAKE_KEY
    monkeypatch.delenv("VRX_API_KEY")
    with pytest.raises(ValueError, match="no API key"):
        vrx.VrxSession(URL, transport=fake)


def test_rejects_credentials_in_url(fake: FakeTransport) -> None:
    with pytest.raises(ValueError, match="credentials in the URL"):
        vrx.VrxSession("https://admin:pw@vrx.test", FAKE_KEY, transport=fake)


def test_tls_verification_is_on_by_default() -> None:
    import ssl

    t = vrx.VrxSession(URL, FAKE_KEY)._transport
    assert t._ctx.verify_mode == ssl.CERT_REQUIRED and t._ctx.check_hostname  # type: ignore[attr-defined]
    with pytest.warns(UserWarning, match="DISABLED"):
        vrx.VrxSession(URL, FAKE_KEY, verify=False)


def test_pointer_to_url_encoding(s: vrx.VrxSession, fake: FakeTransport) -> None:
    fake.on("PUT", "/api/v1/config/interfaces/TenGigabitEthernet0~10~10", (200, {"pointer": "x"}))
    fake.on("PATCH", "/api/v1/config/interfaces/a%20b", (200, {"pointer": "y"}))
    s.set(vrx.pointer.join("interfaces", "TenGigabitEthernet0/0/0"), {"mtu": 1500})
    s.merge("interfaces/a b/", {"mtu": 9000})
    assert fake.calls() == ["PUT /api/v1/config/interfaces/TenGigabitEthernet0~10~10", "PATCH /api/v1/config/interfaces/a%20b"]
    assert fake.seen[0].headers["content-type"] == "application/json"
    with pytest.raises(ValueError):
        s.set("/", {})
    with pytest.raises(ValueError, match="escape"):
        s.set("/interfaces/bad~2", {})


def test_body_less_posts_send_no_content_type(s: vrx.VrxSession, fake: FakeTransport) -> None:
    # Fastify rejects an empty body with content-type: application/json (seen live)
    fake.on("POST", "/api/v1/config/commit", (200, {"status": "unchanged", "results": [], "warnings": [], "notApplied": []}))
    s.commit(comment="c")
    assert "content-type" not in fake.seen[0].headers
    assert fake.seen[0].query == {"comment": "c"}


def test_validation_problem_becomes_typed_error_with_pointer(s: vrx.VrxSession, fake: FakeTransport) -> None:
    fake.on("PUT", "/api/v1/config/interfaces/loop1", (400, problem(
        400, "Validation failed", "the document does not match the schema",
        [{"pointer": "/interfaces/loop1/ipv4/0", "message": "Invalid IPv4 range"}])))
    with pytest.raises(vrx.ValidationError) as ei:
        s.set("/interfaces/loop1", {"ipv4": ["bad"]})
    e = ei.value
    assert e.status == 400 and e.pointer == "/interfaces/loop1/ipv4/0"
    assert e.errors == [vrx.FieldError("/interfaces/loop1/ipv4/0", "Invalid IPv4 range")]
    assert "Invalid IPv4 range" in str(e) and FAKE_KEY not in str(e)


@pytest.mark.parametrize(("status", "cls"), [(401, vrx.Unauthorized), (403, vrx.Forbidden), (404, vrx.NotFound),
                                             (409, vrx.Conflict), (422, vrx.CommitFailed), (503, vrx.Unavailable),
                                             (500, vrx.ApiError)])
def test_status_to_exception(s: vrx.VrxSession, fake: FakeTransport, status: int, cls: type) -> None:
    fake.on("GET", "/api/v1/config/diff", (status, problem(status, "x", lock={"owner": "bob"}, sync={"state": "unknown"})))
    with pytest.raises(cls) as ei:
        s.diff()
    if status == 409:
        assert ei.value.lock == {"owner": "bob"}
    if status == 503:
        assert ei.value.sync == {"state": "unknown"}


def test_non_json_error_body(s: vrx.VrxSession, fake: FakeTransport) -> None:
    fake.on("GET", "/api/v1/config", (502, HttpResponse(502, {"content-type": "text/html"}, b"<html>bad gateway</html>")))
    with pytest.raises(vrx.Unavailable, match="bad gateway"):
        s.running()


def test_transaction_commits_with_confirm_then_confirms(s: vrx.VrxSession, fake: FakeTransport) -> None:
    diffs = iter([[], [{"op": "add", "pointer": "/interfaces/loop1"}]])
    fake.on("GET", "/api/v1/config/diff", lambda _: (200, {"baseRevision": 1, "changes": next(diffs)}))
    fake.on("PUT", "/api/v1/config/interfaces/loop1", (200, {"pointer": "/interfaces/loop1"}))
    fake.on("POST", "/api/v1/config/commit", pending_commit)
    fake.on("GET", "/api/v1/state/system", (200, {"sync": {"state": "in-sync"}}))
    fake.on("POST", "/api/v1/config/commit/confirm", (200, {"status": "confirmed", "revision": {"id": 2}}))
    with s.transaction(confirm=30, comment="add loop1") as tx:
        tx.set("/interfaces/loop1", {"ipv4": ["192.0.2.1/32"]})
    assert tx.result == {"status": "confirmed", "revision": {"id": 2}}
    assert fake.calls() == [
        "GET /api/v1/config/diff", "PUT /api/v1/config/interfaces/loop1", "GET /api/v1/config/diff",
        "POST /api/v1/config/commit?comment=add loop1&confirm=30", "GET /api/v1/state/system",
        "POST /api/v1/config/commit/confirm"]


def test_transaction_refuses_dirty_candidate(s: vrx.VrxSession, fake: FakeTransport) -> None:
    fake.on("GET", "/api/v1/config/diff", (200, {"changes": [{"op": "add", "pointer": "/x"}]}))
    fake.on("GET", "/api/v1/config/lock", (200, {"locked": True, "owner": "alice"}))
    with pytest.raises(vrx.VrxError, match="1 uncommitted change.*alice"):
        with s.transaction():
            pass


def test_transaction_discards_on_exception_and_on_failed_commit(s: vrx.VrxSession, fake: FakeTransport) -> None:
    fake.on("GET", "/api/v1/config/diff", (200, {"changes": []}))
    fake.on("POST", "/api/v1/config/discard", (200, {"discarded": True}))
    with pytest.raises(RuntimeError):
        with s.transaction():
            raise RuntimeError("boom")
    assert fake.calls()[-1] == "POST /api/v1/config/discard"

    diffs = iter([[], [{"op": "add"}]])
    fake.on("GET", "/api/v1/config/diff", lambda _: (200, {"changes": next(diffs)}))
    fake.on("PUT", "/api/v1/config/interfaces/loop1", (200, {}))
    fake.on("POST", "/api/v1/config/commit", (400, problem(400, "Validation failed", errors=[
        {"pointer": "/interfaces/loop1/ipv4/0", "message": "overlaps /interfaces/loop2/ipv4/0"}])))
    with pytest.raises(vrx.BadRequest) as ei:
        with s.transaction() as tx:
            tx.set("/interfaces/loop1", {})
    assert ei.value.pointer == "/interfaces/loop1/ipv4/0"
    assert fake.calls()[-1] == "POST /api/v1/config/discard"


def test_failed_check_does_not_confirm(s: vrx.VrxSession, fake: FakeTransport) -> None:
    fake.on("POST", "/api/v1/config/commit", pending_commit)

    def unreachable(_s: vrx.VrxSession) -> None:
        raise vrx.TransportError("management path lost")

    with pytest.raises(vrx.ConfirmError, match="not confirming") as ei:
        s.commit_confirmed(confirm=10, check=unreachable)
    assert ei.value.commit is not None and ei.value.commit["status"] == "pending"
    assert "POST /api/v1/config/commit/confirm" not in fake.calls()


def test_unchanged_transaction_discards_without_commit(s: vrx.VrxSession, fake: FakeTransport) -> None:
    fake.on("GET", "/api/v1/config/diff", (200, {"changes": []}))
    fake.on("POST", "/api/v1/config/discard", (200, {"discarded": False}))
    with s.transaction() as tx:
        pass
    assert tx.result == {"status": "unchanged"}
    assert not any("commit" in c for c in fake.calls())


def test_rollback_and_revisions(s: vrx.VrxSession, fake: FakeTransport) -> None:
    fake.on("POST", "/api/v1/config/rollback/7", (200, {"status": "applied", "revision": {"id": 9}}))
    fake.on("GET", "/api/v1/config/revisions", (200, {"items": [{"id": 9}], "total": 1}))
    assert s.rollback(7, comment="back")["revision"]["id"] == 9
    assert s.revisions(limit=5) == [{"id": 9}]
    assert fake.calls() == ["POST /api/v1/config/rollback/7?comment=back", "GET /api/v1/config/revisions?limit=5&offset=0"]


def test_logs_never_carry_key_or_write_only_values(s: vrx.VrxSession, fake: FakeTransport, caplog) -> None:  # type: ignore[no-untyped-def]
    s.log_bodies = True
    hashed = "$argon2id$v=19$m=65536,t=3,p=4$VRX_TEST_PSK_hash"
    fake.on("PUT", "/api/v1/config/management/users", (200, {}))
    fake.on("POST", "/api/v1/secrets", (200, {"ref": "psk/site-a", "created": True, "version": 1}))
    with caplog.at_level(logging.DEBUG, logger="vrx"):
        s.set("/management/users", [{"username": "ops", "role": "operator", "passwordHash": hashed}])
        s.put_secret("psk", "site-a", "VRX_TEST_PSK_site_a")
    text = caplog.text
    assert "PUT /api/v1/config/management/users → 200" in text
    assert '"passwordHash": "<redacted>"' in text and "ops" in text
    assert hashed not in text and "VRX_TEST_PSK_site_a" not in text and FAKE_KEY not in text
    assert fake.seen[0].body[0]["passwordHash"] == hashed  # the API still gets it


def test_redact_helpers() -> None:
    doc = {"management": {"users": [{"username": "a", "passwordHash": "h"}, {"username": "b"}]}}
    assert vrx.secret_pointers(doc) == ["/management/users/0/passwordHash"]
    red = vrx.redact(doc)
    assert red["management"]["users"][0]["passwordHash"] == vrx.REDACTED and doc["management"]["users"][0]["passwordHash"] == "h"
    assert vrx.redact({"username": "a", "passwordHash": "h"}, "/management/users/3") == {"username": "a", "passwordHash": vrx.REDACTED}
    assert vrx.redact("h", "/management/users/0/passwordHash") == vrx.REDACTED
    assert vrx.redact({"x": 1}, "/interfaces/loop1") == {"x": 1}
