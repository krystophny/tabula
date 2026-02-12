from __future__ import annotations

from pathlib import Path

import pytest

from tabula.web.store import Store


def test_store_creates_db(tmp_path: Path) -> None:
    db_path = tmp_path / "test.db"
    store = Store(db_path)
    assert db_path.exists()
    store.close()


def test_admin_password_lifecycle(tmp_path: Path) -> None:
    store = Store(tmp_path / "test.db")
    assert store.has_admin_password() is False
    assert store.verify_admin_password("anything") is False

    store.set_admin_password("securepass")
    assert store.has_admin_password() is True
    assert store.verify_admin_password("securepass") is True
    assert store.verify_admin_password("wrongpass") is False
    store.close()


def test_admin_password_minimum_length(tmp_path: Path) -> None:
    store = Store(tmp_path / "test.db")
    with pytest.raises(ValueError, match="at least 8"):
        store.set_admin_password("short")
    store.close()


def test_host_crud(tmp_path: Path) -> None:
    store = Store(tmp_path / "test.db")

    host = store.add_host(name="dev", hostname="10.0.0.1", username="user")
    assert host.id == 1
    assert host.name == "dev"
    assert host.hostname == "10.0.0.1"
    assert host.port == 22
    assert host.username == "user"
    assert host.project_dir == "~"

    retrieved = store.get_host(host.id)
    assert retrieved.name == "dev"

    hosts = store.list_hosts()
    assert len(hosts) == 1

    updated = store.update_host(host.id, hostname="10.0.0.2", port=2222)
    assert updated.hostname == "10.0.0.2"
    assert updated.port == 2222
    assert updated.name == "dev"

    store.delete_host(host.id)
    assert len(store.list_hosts()) == 0
    store.close()


def test_host_get_not_found(tmp_path: Path) -> None:
    store = Store(tmp_path / "test.db")
    with pytest.raises(KeyError):
        store.get_host(999)
    store.close()


def test_host_validation(tmp_path: Path) -> None:
    store = Store(tmp_path / "test.db")
    with pytest.raises(ValueError, match="name"):
        store.add_host(name="", hostname="h", username="u")
    with pytest.raises(ValueError, match="hostname"):
        store.add_host(name="n", hostname="", username="u")
    with pytest.raises(ValueError, match="username"):
        store.add_host(name="n", hostname="h", username="")
    store.close()


def test_host_unique_name(tmp_path: Path) -> None:
    store = Store(tmp_path / "test.db")
    store.add_host(name="dev", hostname="h1", username="u1")
    with pytest.raises(Exception):
        store.add_host(name="dev", hostname="h2", username="u2")
    store.close()


def test_host_to_dict(tmp_path: Path) -> None:
    store = Store(tmp_path / "test.db")
    host = store.add_host(name="dev", hostname="h", username="u", key_path="/k", project_dir="/proj")
    d = store.host_to_dict(host)
    assert d["name"] == "dev"
    assert d["hostname"] == "h"
    assert d["key_path"] == "/k"
    assert d["project_dir"] == "/proj"
    store.close()


def test_update_host_rejects_wrong_types(tmp_path: Path) -> None:
    store = Store(tmp_path / "test.db")
    host = store.add_host(name="dev", hostname="h", username="u")
    with pytest.raises(ValueError, match="port must be int"):
        store.update_host(host.id, port="not-a-number")
    with pytest.raises(ValueError, match="hostname must be str"):
        store.update_host(host.id, hostname=123)
    store.close()


def test_update_host_ignores_unknown_fields(tmp_path: Path) -> None:
    store = Store(tmp_path / "test.db")
    host = store.add_host(name="dev", hostname="h", username="u")
    updated = store.update_host(host.id, unknown_field="value")
    assert updated.hostname == "h"
    store.close()


def test_store_persists_across_instances(tmp_path: Path) -> None:
    db_path = tmp_path / "test.db"
    store1 = Store(db_path)
    store1.set_admin_password("password1")
    store1.add_host(name="dev", hostname="h", username="u")
    store1.close()

    store2 = Store(db_path)
    assert store2.has_admin_password() is True
    assert store2.verify_admin_password("password1") is True
    assert len(store2.list_hosts()) == 1
    store2.close()
