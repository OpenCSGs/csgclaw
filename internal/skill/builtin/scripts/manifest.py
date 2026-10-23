"""Regenerate embedded bundle hashes after editing skills (Go WalkDir order)."""
import hashlib
import json
from pathlib import Path

base = Path(__file__).resolve().parents[1]
hashes = {}
for skill in sorted((base / "embed").iterdir()):
    hasher = hashlib.sha256()

    def visit(directory):
        for path in sorted(directory.iterdir()):
            if path.is_dir():
                visit(path)
            else:
                hasher.update(path.relative_to(skill).as_posix().encode() + b"\0")
                hasher.update(path.read_bytes())

    visit(skill)
    hashes[skill.name] = hasher.hexdigest()
(base / "manifest.json").write_text(json.dumps(hashes, indent=2) + "\n")
