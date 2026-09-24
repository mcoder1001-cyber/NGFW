"""The HTTP layer (stdlib `urllib`). Swappable: tests pass a fake with the same `request` method."""
from __future__ import annotations

import socket
import ssl
import urllib.error
import urllib.request
import warnings
from dataclasses import dataclass, field
from typing import Protocol

from .errors import TransportError


@dataclass
class HttpResponse:
    status: int
    headers: dict[str, str] = field(default_factory=dict)
    body: bytes = b""


class Transport(Protocol):
    def request(
        self, method: str, url: str, headers: dict[str, str], body: bytes | None, timeout: float
    ) -> HttpResponse: ...


class UrllibTransport:
    """TLS verification is ON by default (system trust store, or `ca_file`). `verify=False` is for lab boxes with a
    self-signed certificate only and warns on every session."""

    def __init__(self, verify: bool = True, ca_file: str | None = None):
        if verify:
            self._ctx = ssl.create_default_context(cafile=ca_file)
        else:
            warnings.warn("vrx: TLS certificate verification is DISABLED (verify=False)", stacklevel=3)
            self._ctx = ssl.create_default_context()
            self._ctx.check_hostname = False
            self._ctx.verify_mode = ssl.CERT_NONE
        # never follow redirects: an Authorization header must not travel to another origin
        self._opener = urllib.request.build_opener(
            urllib.request.HTTPSHandler(context=self._ctx), _NoRedirect()
        )

    def request(
        self, method: str, url: str, headers: dict[str, str], body: bytes | None, timeout: float
    ) -> HttpResponse:
        req = urllib.request.Request(url, data=body, method=method, headers=headers)
        try:
            with self._opener.open(req, timeout=timeout) as r:
                return HttpResponse(r.status, {k.lower(): v for k, v in r.headers.items()}, r.read())
        except urllib.error.HTTPError as e:
            data = e.read() if e.fp is not None else b""
            return HttpResponse(e.code, {k.lower(): v for k, v in (e.headers or {}).items()}, data)
        except (urllib.error.URLError, socket.timeout, TimeoutError, ssl.SSLError, ConnectionError) as e:
            reason = getattr(e, "reason", e)
            # the URL is safe to show (no credentials in it); headers are not
            raise TransportError(f"{method} {url}: {reason}") from None


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):  # type: ignore[no-untyped-def]
        return None
