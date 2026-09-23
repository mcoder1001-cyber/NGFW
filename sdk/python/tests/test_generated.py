"""The generated layer: every OpenAPI operation has a method, models import, secret patterns are present."""
from __future__ import annotations

import inspect

import vrx
from vrx._generated import models
from vrx._generated.operations import OPERATIONS, Operations
from vrx._generated.secrets import SECRET_REF_POINTERS, WRITE_ONLY_POINTERS

from .fake import FAKE_KEY, FakeTransport


def test_every_operation_has_a_method() -> None:
    names = {n for n, _ in inspect.getmembers(Operations, inspect.isfunction) if not n.startswith("_")}
    assert len(names) == len(OPERATIONS) >= 40
    for must in ("config_put_at", "config_commit", "config_confirm", "config_rollback", "state_interfaces",
                 "secrets_put", "auth_create_api_key"):
        assert must in names


def test_generated_method_builds_the_request() -> None:
    fake = FakeTransport()
    fake.on("POST", "/api/v1/config/rollback/3", (200, {"status": "pending"}))
    fake.on("DELETE", "/api/v1/secrets/psk/site-a", (204, None))
    s = vrx.VrxSession("http://h:1", FAKE_KEY, transport=fake)
    s.config_rollback("3", confirm=30)
    assert s.secrets_delete("psk", "site-a") is None
    assert fake.calls() == ["POST /api/v1/config/rollback/3?confirm=30", "DELETE /api/v1/secrets/psk/site-a"]


def test_models_cover_the_config_domains() -> None:
    for name in ("RootConfig", "InterfacesConfig", "InterfacesConfigValue", "ConfigCommitResponse", "Problem"):
        assert hasattr(models, name), name
    assert models.ConfigCommitResponse.__required_keys__ >= {"status", "results", "warnings", "notApplied"}
    assert "passwordHash" in models.ManagementConfigUsersItem.__annotations__


def test_secret_patterns_generated_from_the_schema() -> None:
    assert "/management/users/*/passwordHash" in WRITE_ONLY_POINTERS
    assert "/vpn/ipsec/tunnels/*/auth/secretRef" in SECRET_REF_POINTERS
