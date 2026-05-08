#!/usr/bin/env python3

import importlib.util
import pathlib
import sys
import unittest


SCRIPT_PATH = pathlib.Path(__file__).with_name("install-skill-from-github.py")
sys.path.insert(0, str(SCRIPT_PATH.parent))
SPEC = importlib.util.spec_from_file_location("install_skill", SCRIPT_PATH)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC is not None and SPEC.loader is not None
sys.modules[SPEC.name] = MODULE
SPEC.loader.exec_module(MODULE)


class ResolveSourceTests(unittest.TestCase):
    def test_github_repo_requires_owner_repo(self):
        args = MODULE.Args(repo="openai/skills", path=["skills/.experimental/demo"])
        source = MODULE._resolve_source(args)
        self.assertEqual("openai", source.owner)
        self.assertEqual("skills", source.repo)
        self.assertEqual(["skills/.experimental/demo"], source.paths)
        self.assertEqual("skills", source.default_name)

    def test_git_repo_url_defaults_to_repo_root(self):
        args = MODULE.Args(repo="https://opencsg-stg.com/skills/wanghj/gitlab-csgclaw.git")
        source = MODULE._resolve_source(args)
        self.assertIsNone(source.owner)
        self.assertIsNone(source.repo)
        self.assertEqual(["."], source.paths)
        self.assertEqual(
            "https://opencsg-stg.com/skills/wanghj/gitlab-csgclaw.git",
            source.repo_url,
        )
        self.assertEqual("gitlab-csgclaw", source.default_name)

    def test_skill_name_for_repo_root_uses_repo_name(self):
        source = MODULE.Source(
            owner=None,
            repo=None,
            ref="main",
            paths=["."],
            repo_url="https://opencsg-stg.com/skills/wanghj/gitlab-csgclaw.git",
            default_name="gitlab-csgclaw",
        )
        self.assertEqual(
            "gitlab-csgclaw",
            MODULE._skill_name_for_path(source, ".", None, False),
        )


if __name__ == "__main__":
    unittest.main()
