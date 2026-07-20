"""Detect project technology stack and determine test strategy.

Detection priority:
1. Scan actual source files in project directory — count extensions to determine
   the dominant language. This is the most reliable signal.
2. Marker file detection (package.json, pyproject.toml, etc.) — used when the
   directory scan is inconclusive.

Usage: python3 scripts/detect.py --project-dir "./gitlab_projects/project/_source"
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from collections import Counter
from pathlib import Path


# File extensions mapped to technology stacks
_NODE_EXTENSIONS = {".vue", ".jsx", ".tsx", ".svelte"}
_NODE_OR_GENERIC_JS = {".js", ".ts", ".mjs", ".mts", ".cjs"}
_PYTHON_EXTENSIONS = {".py", ".pyi", ".pyx"}
_GO_EXTENSIONS = {".go"}

# Directories to skip during source file scanning
_SKIP_DIRS = {
    "node_modules",
    ".git",
    "__pycache__",
    ".venv",
    "venv",
    "env",
    "dist",
    "build",
    ".tox",
    ".mypy_cache",
    ".pytest_cache",
    ".ruff_cache",
    "vendor",
}


def _scan_project_files(project_dir: Path) -> str | None:
    """Scan actual source files in project directory to determine dominant language.

    Walks the directory tree, counts file extensions, and returns the dominant stack.
    This is more reliable than checking marker files because it reflects what the
    project actually contains.

    Returns:
        "node", "python", "go", or None if undetermined.
    """
    stack_scores: Counter[str] = Counter()

    for _root, dirs, files in os.walk(project_dir):
        # Skip irrelevant directories in-place
        dirs[:] = [d for d in dirs if d not in _SKIP_DIRS and not d.startswith(".")]

        for f in files:
            ext = Path(f).suffix.lower()
            if ext in _NODE_EXTENSIONS:
                # .vue, .jsx, .tsx — unambiguously frontend/Node
                stack_scores["node"] += 3
            elif ext in _NODE_OR_GENERIC_JS:
                stack_scores["node"] += 1
            elif ext in _PYTHON_EXTENSIONS:
                stack_scores["python"] += 1
            elif ext in _GO_EXTENSIONS:
                stack_scores["go"] += 1

    if not stack_scores:
        return None

    # Only return if there's a clear winner (at least 2x the runner-up)
    top_two = stack_scores.most_common(2)
    winner_stack, winner_score = top_two[0]

    if len(top_two) == 1 or winner_score >= top_two[1][1] * 2:
        return winner_stack

    # Close race — check for strong Node indicators (.vue/.jsx/.tsx files exist)
    if stack_scores.get("node", 0) > 0:
        # Any .vue/.jsx/.tsx file is a strong signal for Node
        for _root, dirs, files in os.walk(project_dir):
            dirs[:] = [d for d in dirs if d not in _SKIP_DIRS and not d.startswith(".")]
            for f in files:
                if Path(f).suffix.lower() in _NODE_EXTENSIONS:
                    return "node"

    return winner_stack


def _detect_node_framework(project_dir: Path) -> dict:
    """Detect Node.js framework and test runner from package.json."""
    pkg_path = project_dir / "package.json"
    try:
        pkg = json.loads(pkg_path.read_text(encoding="utf-8"))
    except (json.JSONDecodeError, OSError) as e:
        return {"stack": "node", "framework": "unknown", "error": str(e)}

    deps = {**pkg.get("dependencies", {}), **pkg.get("devDependencies", {})}
    scripts = pkg.get("scripts", {})

    # Detect framework
    framework = "generic"
    if "vue" in deps or "@vue/cli-service" in deps:
        framework = "vue"
    elif "react" in deps:
        framework = "react"
    elif "next" in deps:
        framework = "next"

    # Detect test runner
    test_runner = None
    test_command = None
    if "vitest" in deps:
        test_runner = "vitest"
        test_command = "npx vitest run"
    elif "jest" in deps:
        test_runner = "jest"
        test_command = "npx jest"
    elif "@vue/test-utils" in deps:
        test_runner = "vue-test-utils"
        test_command = "npm test" if "test" in scripts else None
    elif "test" in scripts:
        test_runner = "npm-test"
        test_command = "npm test"

    # Detect build and lint commands
    build_command = "npm run build" if "build" in scripts else None
    lint_command = "npm run lint" if "lint" in scripts else None

    # Detect package manager
    install_command = "npm install"
    if (project_dir / "pnpm-lock.yaml").exists():
        install_command = "pnpm install"
    elif (project_dir / "yarn.lock").exists():
        install_command = "yarn install"

    return {
        "stack": "node",
        "framework": framework,
        "test_runner": test_runner,
        "install_command": install_command,
        "test_command": test_command,
        "build_command": build_command,
        "lint_command": lint_command,
    }


def _detect_python_runner(project_dir: Path) -> dict:
    """Detect Python test runner and tools."""
    test_runner = "pytest"
    test_command = "python -m pytest -v --tb=short"
    lint_command = "ruff check ."
    install_command = None

    # Determine install command
    if (project_dir / "pyproject.toml").exists() or (project_dir / "setup.py").exists():
        install_command = 'uv pip install -e ".[dev]" --system 2>/dev/null || uv pip install -e . --system'
    elif (project_dir / "requirements.txt").exists():
        install_command = "uv pip install -r requirements.txt --system"

    # Check for unittest pattern if no pytest config
    pyproject = project_dir / "pyproject.toml"
    if pyproject.exists():
        content = pyproject.read_text(encoding="utf-8")
        if "unittest" in content and "pytest" not in content:
            test_runner = "unittest"
            test_command = "python -m unittest discover -v"

    return {
        "stack": "python",
        "framework": "python",
        "test_runner": test_runner,
        "install_command": install_command,
        "test_command": test_command,
        "build_command": None,
        "lint_command": lint_command,
    }


def _detect_go(project_dir: Path) -> dict:
    """Detect Go project."""
    return {
        "stack": "go",
        "framework": "go",
        "test_runner": "go-test",
        "install_command": "go mod download",
        "test_command": "go test ./... -v -count=1",
        "build_command": "go build ./...",
        "lint_command": "golangci-lint run 2>/dev/null || true",
    }


# Maps stack name to its detailed detector function
_STACK_DETECTORS = {
    "node": _detect_node_framework,
    "python": _detect_python_runner,
    "go": _detect_go,
}

# Marker files for fallback detection (order matters — first match wins)
_MARKER_DETECTORS = [
    ("package.json", _detect_node_framework),
    ("pyproject.toml", _detect_python_runner),
    ("setup.py", _detect_python_runner),
    ("requirements.txt", _detect_python_runner),
    ("go.mod", _detect_go),
]


def detect_stack(project_dir: Path) -> dict:
    """Detect the project's technology stack.

    Priority:
    1. Scan actual source files in the project directory — the most reliable signal
       because it reflects what the project really is (e.g., a directory full of .vue
       files is a Vue project, even if it also has a requirements.txt).
    2. Marker file detection — fallback when source scan is inconclusive.

    Args:
        project_dir: Path to the project root.

    Returns:
        Strategy dict with stack, framework, test_runner, commands, etc.
    """
    # Priority 1: scan actual source files
    scanned_stack = _scan_project_files(project_dir)
    if scanned_stack:
        detector = _STACK_DETECTORS.get(scanned_stack)
        if detector:
            result = detector(project_dir)
            result["confidence"] = "high"
            result["detection_source"] = "source_scan"
            return result

    # Priority 2: marker file detection
    for marker_file, detector in _MARKER_DETECTORS:
        if (project_dir / marker_file).exists():
            result = detector(project_dir)
            result["confidence"] = "medium"
            result["detection_source"] = "marker_file"
            return result

    # Unknown stack — analysis only
    return {
        "stack": "unknown",
        "framework": "unknown",
        "test_runner": None,
        "install_command": None,
        "test_command": None,
        "build_command": None,
        "lint_command": None,
        "confidence": "none",
        "detection_source": "none",
    }


def main() -> None:
    parser = argparse.ArgumentParser(description="Detect project technology stack")
    parser.add_argument("--project-dir", required=True, help="Path to project root directory")
    args = parser.parse_args()

    project_dir = Path(args.project_dir)
    if not project_dir.is_dir():
        print(json.dumps({"error": f"Directory not found: {args.project_dir}"}), file=sys.stderr)
        sys.exit(1)

    result = detect_stack(project_dir)
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as e:
        print(f"Unexpected error: {e}", file=sys.stderr)
        sys.exit(3)
