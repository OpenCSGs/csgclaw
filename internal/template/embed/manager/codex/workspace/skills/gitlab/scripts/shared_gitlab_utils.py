"""Explicit shared GitLab utilities for subskills.

Subskill scripts should import from this module to avoid ambiguity with
same-name modules in their local directories.
"""

from gitlab_utils import (
    WORKSPACE,
    configure_git_credentials,
    fail,
    git_repo_url,
    load_credentials,
    run_git,
    run_git_optional,
    run_git_with_credentials,
    set_clean_origin,
)

__all__ = [
    "WORKSPACE",
    "configure_git_credentials",
    "fail",
    "git_repo_url",
    "load_credentials",
    "run_git",
    "run_git_optional",
    "run_git_with_credentials",
    "set_clean_origin",
]
