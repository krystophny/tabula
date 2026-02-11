from __future__ import annotations

import subprocess
from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class RunResult:
    returncode: int
    stdout: str = ""
    stderr: str = ""


class SubprocessRunner:
    def run(self, argv: list[str], *, cwd: Path | None = None, capture: bool = False) -> RunResult:
        if capture:
            proc = subprocess.run(argv, cwd=cwd, text=True, capture_output=True)
            return RunResult(returncode=proc.returncode, stdout=proc.stdout, stderr=proc.stderr)

        proc = subprocess.run(argv, cwd=cwd)
        return RunResult(returncode=proc.returncode)
