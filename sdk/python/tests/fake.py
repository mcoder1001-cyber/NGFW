"""A fake HTTP layer for unit tests: routes (method, path-without-query) → handler; records every request."""
from __future__ import annotations

import json
from dataclasses import dataclass
from typing import Any, Callable
from urllib.parse import parse_qsl, urlsplit

from vrx.transport import HttpResponse

# a stand-in credential: the literal marker the repository's secret scanner allows in fixtures
FAKE_KEY = "VRX_TEST_PSK_sdk"


@dataclass
class Seen:
    method: str
    path: str
    query: dict[str, str]
    headers: dict[str, str]
    body: Any


Handler = Callable[[Seen], tuple[int, Any]]


class FakeTransport:
    def __init__(self) -> None:
        self.routes: dict[tuple[str, str], Handler | tuple[int, Any]] = {}
        self.seen: list[Seen] = []

    def on(self, method: str, path: str, answer: Handler | tuple[int, Any]) -> None:
        self.routes[(method, path)] = answer

    def request(self, method: str, url: str, headers: dict[str, str], body: bytes | None, timeout: float) -> HttpResponse:
        u = urlsplit(url)
        seen = Seen(method, u.path, dict(parse_qsl(u.query)), dict(headers), json.loads(body) if body else None)
        self.seen.append(seen)
        answer = self.routes.get((method, u.path))
        if answer is None:
            return problem(404, "Not found", f"no fake route {method} {u.path}")
        status, payload = answer(seen) if callable(answer) else answer
        if isinstance(payload, HttpResponse):
            return payload
        ctype = "application/problem+json" if status >= 400 else "application/json"
        return HttpResponse(status, {"content-type": ctype}, b"" if payload is None else json.dumps(payload).encode())

    def calls(self) -> list[str]:
        return [f"{s.method} {s.path}" + (f"?{'&'.join(f'{k}={v}' for k, v in sorted(s.query.items()))}" if s.query else "")
                for s in self.seen]


def problem(status: int, title: str, detail: str = "", errors: list[dict[str, str]] | None = None,
            **extra: Any) -> HttpResponse:
    doc: dict[str, Any] = {"type": "about:blank", "title": title, "status": status, "detail": detail, **extra}
    if errors is not None:
        doc["errors"] = errors
    return HttpResponse(status, {"content-type": "application/problem+json"}, json.dumps(doc).encode())
