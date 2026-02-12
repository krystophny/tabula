from __future__ import annotations

import hashlib
import secrets
import sqlite3
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any


@dataclass
class HostConfig:
    id: int
    name: str
    hostname: str
    port: int
    username: str
    key_path: str
    project_dir: str


SCHEMA_VERSION = 1

_SCHEMA_SQL = """\
CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS hosts (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL UNIQUE,
    hostname    TEXT NOT NULL,
    port        INTEGER NOT NULL DEFAULT 22,
    username    TEXT NOT NULL,
    key_path    TEXT NOT NULL DEFAULT '',
    project_dir TEXT NOT NULL DEFAULT '~'
);
CREATE TABLE IF NOT EXISTS admin (
    id        INTEGER PRIMARY KEY CHECK (id = 1),
    pw_hash   TEXT NOT NULL,
    pw_salt   TEXT NOT NULL
);
"""


_PBKDF2_ITERATIONS = 600_000


def _hash_password(password: str, salt: str) -> str:
    return hashlib.pbkdf2_hmac(
        "sha256", password.encode("utf-8"), salt.encode("utf-8"), _PBKDF2_ITERATIONS,
    ).hex()


class Store:
    def __init__(self, db_path: Path) -> None:
        self._db_path = db_path
        db_path.parent.mkdir(parents=True, exist_ok=True)
        self._conn = sqlite3.connect(str(db_path), check_same_thread=False)
        self._conn.row_factory = sqlite3.Row
        self._conn.execute("PRAGMA journal_mode=WAL")
        self._migrate()

    def _migrate(self) -> None:
        self._conn.executescript(_SCHEMA_SQL)
        row = self._conn.execute("SELECT value FROM meta WHERE key='schema_version'").fetchone()
        if row is None:
            self._conn.execute(
                "INSERT INTO meta (key, value) VALUES ('schema_version', ?)",
                (str(SCHEMA_VERSION),),
            )
            self._conn.commit()

    def close(self) -> None:
        self._conn.close()

    def has_admin_password(self) -> bool:
        return self._conn.execute("SELECT COUNT(*) FROM admin").fetchone()[0] > 0

    def set_admin_password(self, password: str) -> None:
        if len(password) < 8:
            raise ValueError("password must be at least 8 characters")
        salt = secrets.token_hex(16)
        pw_hash = _hash_password(password, salt)
        self._conn.execute("DELETE FROM admin")
        self._conn.execute(
            "INSERT INTO admin (id, pw_hash, pw_salt) VALUES (1, ?, ?)",
            (pw_hash, salt),
        )
        self._conn.commit()

    def verify_admin_password(self, password: str) -> bool:
        row = self._conn.execute("SELECT pw_hash, pw_salt FROM admin WHERE id=1").fetchone()
        if row is None:
            return False
        return _hash_password(password, row["pw_salt"]) == row["pw_hash"]

    def add_host(self, *, name: str, hostname: str, port: int = 22, username: str, key_path: str = "", project_dir: str = "~") -> HostConfig:
        if not name.strip():
            raise ValueError("name must be non-empty")
        if not hostname.strip():
            raise ValueError("hostname must be non-empty")
        if not username.strip():
            raise ValueError("username must be non-empty")
        cursor = self._conn.execute(
            "INSERT INTO hosts (name, hostname, port, username, key_path, project_dir) VALUES (?, ?, ?, ?, ?, ?)",
            (name.strip(), hostname.strip(), port, username.strip(), key_path.strip(), project_dir.strip()),
        )
        self._conn.commit()
        return self.get_host(cursor.lastrowid)

    def get_host(self, host_id: int) -> HostConfig:
        row = self._conn.execute("SELECT * FROM hosts WHERE id=?", (host_id,)).fetchone()
        if row is None:
            raise KeyError(f"host {host_id} not found")
        return HostConfig(**dict(row))

    def list_hosts(self) -> list[HostConfig]:
        rows = self._conn.execute("SELECT * FROM hosts ORDER BY name").fetchall()
        return [HostConfig(**dict(r)) for r in rows]

    def update_host(self, host_id: int, **fields: Any) -> HostConfig:
        allowed = {"name", "hostname", "port", "username", "key_path", "project_dir"}
        updates = {k: v for k, v in fields.items() if k in allowed and v is not None}
        if not updates:
            return self.get_host(host_id)
        set_clause = ", ".join(f"{k}=?" for k in updates)
        values = list(updates.values()) + [host_id]
        self._conn.execute(f"UPDATE hosts SET {set_clause} WHERE id=?", values)
        self._conn.commit()
        return self.get_host(host_id)

    def delete_host(self, host_id: int) -> None:
        self._conn.execute("DELETE FROM hosts WHERE id=?", (host_id,))
        self._conn.commit()

    def host_to_dict(self, host: HostConfig) -> dict[str, Any]:
        return asdict(host)
