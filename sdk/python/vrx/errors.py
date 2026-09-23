"""Typed exceptions. Every non-2xx answer of the API is an RFC 9457 problem (`application/problem+json`) with
`errors[]{pointer, message, rule?}`; it becomes an `ApiError` subclass chosen by the HTTP status."""
from __future__ import annotations

from dataclasses import dataclass
from typing import Any


class VrxError(Exception):
    """Base class of everything the SDK raises."""


class TransportError(VrxError):
    """The request never produced an HTTP answer (DNS, TCP, TLS, timeout)."""


@dataclass(frozen=True)
class FieldError:
    """One `errors[]` entry of a problem: the offending JSON pointer and why."""

    pointer: str
    message: str
    rule: str | None = None

    def __str__(self) -> str:
        return f"{self.pointer or '/'}: {self.message}" + (f" [{self.rule}]" if self.rule else "")


class ApiError(VrxError):
    """A problem+json answer. `errors` carries the pointers; `problem` the whole (server-redacted) document."""

    def __init__(self, status: int, problem: dict[str, Any] | None, method: str, url_path: str):
        self.status = status
        self.problem: dict[str, Any] = problem if isinstance(problem, dict) else {}
        self.method = method
        self.path = url_path
        self.type: str = str(self.problem.get("type", "about:blank"))
        self.title: str = str(self.problem.get("title", ""))
        self.detail: str = str(self.problem.get("detail", ""))
        self.errors: list[FieldError] = [
            FieldError(str(e.get("pointer", "")), str(e.get("message", "")), e.get("rule"))
            for e in self.problem.get("errors") or []
            if isinstance(e, dict)
        ]
        super().__init__(self._message())

    @property
    def pointer(self) -> str | None:
        """The first offending pointer, if the problem names one."""
        return self.errors[0].pointer if self.errors else None

    def _message(self) -> str:
        head = f"{self.method} {self.path} → {self.status} {self.title}".rstrip()
        if self.detail:
            head += f": {self.detail}"
        return head + "".join(f"\n  {e}" for e in self.errors)


class BadRequest(ApiError):
    """400 — schema/semantic validation failed (see `errors[].pointer`) or a malformed request."""


ValidationError = BadRequest


class Unauthorized(ApiError):
    """401 — missing, expired or revoked API key."""


class Forbidden(ApiError):
    """403 — the key's role may not do this (readonly: GET only; operator: no users/AAA/secrets)."""


class NotFound(ApiError):
    """404 — nothing at that pointer / no such revision."""


class Conflict(ApiError):
    """409 — the candidate is locked by someone else (`lock`), a commit is pending, or the object is in use."""

    @property
    def lock(self) -> dict[str, Any] | None:
        lock = self.problem.get("lock")
        return lock if isinstance(lock, dict) else None


class CommitFailed(ApiError):
    """422 — the agent refused or failed to apply; running is untouched (see `results` and `sync`)."""

    @property
    def results(self) -> list[dict[str, Any]]:
        return list(self.problem.get("results") or [])


class Unavailable(ApiError):
    """502/503/504 — the agent is unreachable or its answer was lost. `sync` says whether running is known."""

    @property
    def sync(self) -> dict[str, Any] | None:
        s = self.problem.get("sync")
        return s if isinstance(s, dict) else None


class ConfirmError(VrxError):
    """A confirmed commit could not be confirmed; the agent reverts it at the deadline."""

    def __init__(self, message: str, commit: dict[str, Any] | None = None):
        super().__init__(message)
        self.commit = commit


_BY_STATUS: dict[int, type[ApiError]] = {
    400: BadRequest,
    401: Unauthorized,
    403: Forbidden,
    404: NotFound,
    409: Conflict,
    422: CommitFailed,
    502: Unavailable,
    503: Unavailable,
    504: Unavailable,
}


def error_for(status: int, problem: dict[str, Any] | None, method: str, url_path: str) -> ApiError:
    return _BY_STATUS.get(status, ApiError)(status, problem, method, url_path)
