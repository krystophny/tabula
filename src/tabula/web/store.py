from __future__ import annotations

import hashlib
import hmac
import secrets
import sqlite3
import time
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


SCHEMA_VERSION = 2

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
CREATE TABLE IF NOT EXISTS auth_sessions (
    token      TEXT PRIMARY KEY,
    created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS remote_sessions (
    session_id TEXT PRIMARY KEY,
    host_id    INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    FOREIGN KEY(host_id) REFERENCES hosts(id) ON DELETE CASCADE
);
"""


_PBKDF2_ITERATIONS = 600_000

_HOST_FIELD_TYPES: dict[str, type] = {
    "name": str, "hostname": str, "port": int,
    "username": str, "key_path": str, "project_dir": str,
}


def _hash_password(password: str, salt: str) -> str:
    return hashlib.pbkdf2_hmac(
        "sha256", password.encode("utf-8"), salt.encode("utf-8"), _PBKDF2_ITERATIONS,
    ).hex()


class Store:
    """SQLite store — must only be called from the aiohttp event loop thread."""

    def __init__(self, db_path: Path) -> None:
        self._db_path = db_path
        db_path.parent.mkdir(parents=True, exist_ok=True)
        self._conn = sqlite3.connect(str(db_path))
        self._conn.row_factory = sqlite3.Row
        self._conn.execute("PRAGMA journal_mode=WAL")
        self._conn.execute("PRAGMA foreign_keys=ON")
        self._migrate()

    def _migrate(self) -> None:
        self._conn.executescript(_SCHEMA_SQL)
        row = self._conn.execute("SELECT value FROM meta WHERE key='schema_version'").fetchone()
        if row is None:
            self._conn.execute(
                "INSERT INTO meta (key, value) VALUES ('schema_version', ?)",
                (str(SCHEMA_VERSION),),
            )
        else:
            try:
                current = int(row["value"])
            except (ValueError, TypeError):
                current = 0
            if current < SCHEMA_VERSION:
                self._conn.execute(
                    "UPDATE meta SET value=? WHERE key='schema_version'",
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
        self._conn.execute("DELETE FROM auth_sessions")
        self._conn.execute(
            "INSERT INTO admin (id, pw_hash, pw_salt) VALUES (1, ?, ?)",
            (pw_hash, salt),
        )
        self._conn.commit()

    def verify_admin_password(self, password: str) -> bool:
        row = self._conn.execute("SELECT pw_hash, pw_salt FROM admin WHERE id=1").fetchone()
        if row is None:
            return False
        return hmac.compare_digest(_hash_password(password, row["pw_salt"]), row["pw_hash"])

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
        updates: dict[str, Any] = {}
        for k, v in fields.items():
            if k not in _HOST_FIELD_TYPES or v is None:
                continue
            if not isinstance(v, _HOST_FIELD_TYPES[k]):
                raise ValueError(f"{k} must be {_HOST_FIELD_TYPES[k].__name__}")
            updates[k] = v
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

    def add_auth_session(self, token: str) -> None:
        if not token:
            raise ValueError("token must be non-empty")
        self._conn.execute(
            "INSERT OR REPLACE INTO auth_sessions (token, created_at) VALUES (?, ?)",
            (token, int(time.time())),
        )
        self._conn.commit()

    def has_auth_session(self, token: str) -> bool:
        if not token:
            return False
        row = self._conn.execute(
            "SELECT 1 FROM auth_sessions WHERE token=?",
            (token,),
        ).fetchone()
        return row is not None

    def delete_auth_session(self, token: str) -> None:
        if not token:
            return
        self._conn.execute("DELETE FROM auth_sessions WHERE token=?", (token,))
        self._conn.commit()

    def list_auth_sessions(self) -> list[str]:
        rows = self._conn.execute(
            "SELECT token FROM auth_sessions ORDER BY created_at ASC"
        ).fetchall()
        return [str(row["token"]) for row in rows]

    def add_remote_session(self, session_id: str, host_id: int) -> None:
        if not session_id:
            raise ValueError("session_id must be non-empty")
        self._conn.execute(
            "INSERT OR REPLACE INTO remote_sessions (session_id, host_id, created_at) VALUES (?, ?, ?)",
            (session_id, host_id, int(time.time())),
        )
        self._conn.commit()

    def delete_remote_session(self, session_id: str) -> None:
        if not session_id:
            return
        self._conn.execute("DELETE FROM remote_sessions WHERE session_id=?", (session_id,))
        self._conn.commit()

    def list_remote_sessions(self) -> list[tuple[str, int]]:
        rows = self._conn.execute(
            "SELECT session_id, host_id FROM remote_sessions ORDER BY created_at ASC"
        ).fetchall()
        return [(str(row["session_id"]), int(row["host_id"])) for row in rows]
