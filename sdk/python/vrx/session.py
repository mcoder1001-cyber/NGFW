"""`VrxSession` — the thin hand-written layer over the generated operations: API-key auth, TLS verification on by
default, problem+json → typed exceptions, and the candidate → commit(confirm) → confirm/rollback workflow."""
from __future__ import annotations

import json
import logging
import os
import time
from collections.abc import Callable, Iterator
from contextlib import contextmanager
from typing import Any
from urllib.parse import quote, urlencode, urlsplit

from ._generated.operations import OPERATIONS, Operations
from .errors import ConfirmError, VrxError, error_for
from .pointer import normalize, to_url_path
from .redact import redact
from .transport import HttpResponse, Transport, UrllibTransport

log = logging.getLogger("vrx")

_NO_BODY: Any = object()
# request bodies of these operations are never logged, even with log_bodies=True (passwords, secret values, keys)
_NEVER_LOG = frozenset({"Auth_login", "Auth_password", "Auth_createApiKey", "Secrets_put"})


class _Secret:
    """Holds a credential; its repr/str never shows it."""

    __slots__ = ("_v",)

    def __init__(self, value: str):
        self._v = value

    def get(self) -> str:
        return self._v

    def __repr__(self) -> str:
        return "<redacted>"

    __str__ = __repr__


class VrxSession(Operations):
    """A connection to one VRX appliance.

    >>> s = VrxSession("https://vrx-a.example:443", api_key_file="/run/secrets/vrx-key")
    >>> with s.transaction(confirm=60, comment="add loop1") as tx:
    ...     tx.set("/interfaces/loop1", {"enabled": True, "ipv4": ["192.0.2.1/32"]})
    >>> tx.result["status"]   # 'confirmed'

    The API key comes from `api_key`, `api_key_file` or `$VRX_API_KEY` (in that order) and is sent as
    `Authorization: ApiKey <key>`. It is never logged and never part of `repr()`.
    """

    def __init__(
        self,
        url: str,
        api_key: str | None = None,
        *,
        api_key_file: str | None = None,
        verify: bool = True,
        ca_file: str | None = None,
        timeout: float = 60.0,
        transport: Transport | None = None,
        log_bodies: bool = False,
    ):
        parts = urlsplit(url)
        if parts.scheme not in ("http", "https") or not parts.netloc:
            raise ValueError(f"url must be http(s)://host[:port], got {url!r}")
        if parts.username or parts.password:
            raise ValueError("credentials in the URL are not accepted; use api_key / api_key_file")
        self.url = f"{parts.scheme}://{parts.netloc}{parts.path.rstrip('/')}"
        if api_key is None and api_key_file:
            with open(api_key_file, encoding="utf-8") as f:
                api_key = f.read().strip()
        if api_key is None:
            api_key = os.environ.get("VRX_API_KEY") or None
        if not api_key:
            raise ValueError("no API key: pass api_key / api_key_file or set VRX_API_KEY")
        self._key = _Secret(api_key)
        self.verify = verify
        self.timeout = timeout
        self.log_bodies = log_bodies
        self._transport: Transport = transport or UrllibTransport(verify=verify, ca_file=ca_file)

    def __repr__(self) -> str:
        return f"VrxSession(url={self.url!r}, verify={self.verify})"

    # ------------------------------------------------------------------ HTTP core

    def request(self, method: str, path: str, *, query: dict[str, Any] | None = None, body: Any = _NO_BODY,
                operation_id: str = "", log_pointer: str | None = None) -> Any:
        """One API call. Returns the decoded JSON (or None); raises an `ApiError` subclass on problem+json."""
        q = {k: ("true" if v is True else "false" if v is False else v) for k, v in (query or {}).items() if v is not None}
        target = self.url + path + (f"?{urlencode(q)}" if q else "")
        headers = {"authorization": f"ApiKey {self._key.get()}", "accept": "application/json, application/problem+json",
                   "user-agent": "vrx-python-sdk/0.1.0"}
        data = None
        if body is not _NO_BODY:
            data = json.dumps(body, separators=(",", ":")).encode()
            headers["content-type"] = "application/json"
        t0 = time.monotonic()
        resp: HttpResponse = self._transport.request(method, target, headers, data, self.timeout)
        ms = int((time.monotonic() - t0) * 1000)
        log.debug("%s %s → %s (%d ms)", method, path, resp.status, ms)
        if self.log_bodies and body is not _NO_BODY and operation_id not in _NEVER_LOG:
            log.debug("  body %s", json.dumps(redact(body, log_pointer) if log_pointer is not None else "<not logged>"))
        payload = _decode(resp)
        if 200 <= resp.status < 300:
            return payload
        raise error_for(resp.status, payload if isinstance(payload, dict) else {"title": str(payload)[:200]},
                        method, path)

    def _call(self, operation_id: str, path_params: dict[str, Any], query: dict[str, Any], body: Any = _NO_BODY) -> Any:
        method, template, _, _, _ = OPERATIONS[operation_id]
        path = template
        log_pointer: str | None = None
        for name, value in path_params.items():
            if name == "path":  # the JSON-pointer parameter of the generic config routes
                enc = to_url_path(str(value))
                log_pointer = normalize(str(value))
            else:
                enc = quote(str(value), safe="")
            path = path.replace("{" + name + "}", enc)
        if log_pointer is None and operation_id in ("Config_patchRoot", "Config_import"):
            log_pointer = ""
        return self.request(method, path, query=query, body=body, operation_id=operation_id, log_pointer=log_pointer)

    # ------------------------------------------------------------------ configuration (candidate)

    def running(self, pointer: str = "") -> Any:
        """Running configuration at `pointer` (redacted by the API)."""
        p = normalize(pointer)
        return self.config_running_at(p) if p else self.config_running()

    def candidate(self, pointer: str = "") -> Any:
        p = normalize(pointer)
        return self.config_candidate_at(p) if p else self.config_candidate()

    def exists(self, pointer: str, *, candidate: bool = False) -> bool:
        from .errors import NotFound

        try:
            (self.candidate if candidate else self.running)(pointer)
            return True
        except NotFound:
            return False

    def set(self, pointer: str, value: Any) -> dict[str, Any]:
        """Replace the candidate node at `pointer` (PUT; creates it when absent)."""
        return self.config_put_at(_nonroot(pointer), value)

    def merge(self, pointer: str, patch: Any) -> dict[str, Any]:
        """RFC 7386 merge patch of the candidate node at `pointer` (`None` deletes a member)."""
        p = normalize(pointer)
        return self.config_patch_at(p, patch) if p else self.config_patch_root(patch)

    def delete(self, pointer: str) -> dict[str, Any]:
        return self.config_delete_at(_nonroot(pointer))

    def diff(self) -> list[dict[str, Any]]:
        return list(self.config_diff().get("changes") or [])

    def validate(self) -> dict[str, Any]:
        return self.config_validate()

    def discard(self) -> dict[str, Any]:
        return self.config_discard()

    def lock(self) -> dict[str, Any]:
        return self.config_lock()

    # ------------------------------------------------------------------ commit / confirm / rollback

    def commit(self, *, confirm: int | None = None, comment: str | None = None) -> dict[str, Any]:
        """Validate + apply the candidate. With `confirm=<sec>` the result is `pending` until `confirm()`."""
        return self.config_commit(confirm=confirm, comment=comment)

    def confirm(self) -> dict[str, Any]:
        return self.config_confirm()

    def pending(self) -> dict[str, Any] | None:
        return self.config_pending().get("pending")

    def rollback(self, revision: int, *, confirm: int | None = None, comment: str | None = None) -> dict[str, Any]:
        """Apply an old revision as a new one (optionally as a confirmed commit)."""
        return self.config_rollback(str(revision), confirm=confirm, comment=comment)

    def revisions(self, limit: int = 50, offset: int = 0) -> list[dict[str, Any]]:
        return list(self.config_revisions(limit=limit, offset=offset).get("items") or [])

    def commit_confirmed(self, *, confirm: int = 60, comment: str | None = None,
                         check: Callable[[VrxSession], object] | None = None) -> dict[str, Any]:
        """Commit with `?confirm=<sec>`, run `check(session)` (default: the API still answers `/state/system`),
        then confirm. If the check fails the commit is NOT confirmed and the agent reverts it at the deadline."""
        r = self.commit(confirm=confirm, comment=comment)
        if r.get("status") != "pending":
            return r  # unchanged (nothing to do) — nothing waits for a confirmation
        try:
            (check or _default_check)(self)
        except Exception as e:  # noqa: BLE001 — any failure of the check means "do not confirm"
            raise ConfirmError(f"post-commit check failed, not confirming (auto-revert at "
                               f"{r.get('confirmDeadline')}): {e}", r) from e
        try:
            return self.confirm()
        except VrxError as e:
            raise ConfirmError(f"confirm failed (auto-revert at {r.get('confirmDeadline')}): {e}", r) from e

    @contextmanager
    def transaction(self, *, confirm: int | None = 60, comment: str | None = None,
                    check: Callable[[VrxSession], object] | None = None,
                    allow_dirty: bool = False) -> Iterator[Transaction]:
        """Edit the candidate inside the block; on exit commit (confirmed when `confirm` is set) — or discard on an
        exception. Refuses to start on a candidate that already holds uncommitted changes (they would be committed
        with ours) unless `allow_dirty=True`."""
        if not allow_dirty:
            pending_changes = self.diff()
            if pending_changes:
                owner = self.lock().get("owner")
                raise VrxError(f"the candidate already has {len(pending_changes)} uncommitted change(s) "
                               f"(lock owner: {owner}); commit or discard them first, or pass allow_dirty=True")
        tx = Transaction(self)
        try:
            yield tx
        except BaseException:
            _quiet_discard(self)
            raise
        if not self.diff():
            _quiet_discard(self)
            tx.result = {"status": "unchanged"}
            return
        try:
            tx.result = (self.commit_confirmed(confirm=confirm, comment=comment, check=check)
                         if confirm else self.commit(comment=comment))
        except ConfirmError:
            raise
        except VrxError:
            _quiet_discard(self)  # running is untouched (commit is atomic); release the lock
            raise

    # ------------------------------------------------------------------ state, secrets

    def state(self, name: str, **query: Any) -> Any:
        """`/api/v1/state/<name>` — system, interfaces, routes (vrf, page, pageSize), drift, events, neighbors."""
        return self.request("GET", f"/api/v1/state/{quote(name.strip('/'), safe='/')}", query=query)

    def put_secret(self, kind: str, name: str, value: str, *, replace: bool = False) -> dict[str, Any]:
        """Store a secret (admin). The value is write-only: never returned, never logged. Returns `{ref, version}`."""
        return self.secrets_put({"kind": kind, "name": name, "value": value}, replace=replace or None)


class Transaction:
    """The candidate edits of one `VrxSession.transaction()` block. `result` holds the commit answer afterwards."""

    def __init__(self, session: VrxSession):
        self.session = session
        self.result: dict[str, Any] | None = None

    def set(self, pointer: str, value: Any) -> dict[str, Any]:
        return self.session.set(pointer, value)

    def merge(self, pointer: str, patch: Any) -> dict[str, Any]:
        return self.session.merge(pointer, patch)

    def delete(self, pointer: str) -> dict[str, Any]:
        return self.session.delete(pointer)

    def diff(self) -> list[dict[str, Any]]:
        return self.session.diff()


def _decode(resp: HttpResponse) -> Any:
    if not resp.body:
        return None
    ctype = resp.headers.get("content-type", "")
    if "json" in ctype or resp.body[:1] in (b"{", b"["):
        try:
            return json.loads(resp.body)
        except ValueError:
            pass
    return resp.body.decode("utf-8", "replace")


def _nonroot(pointer: str) -> str:
    p = normalize(pointer)
    if not p:
        raise ValueError("a pointer below the root is required (use merge('', …) or config_import for the root)")
    return p


def _default_check(s: VrxSession) -> None:
    s.state("system")


def _quiet_discard(s: VrxSession) -> None:
    try:
        s.discard()
    except VrxError as e:
        log.warning("discard after a failed transaction: %s", e)
