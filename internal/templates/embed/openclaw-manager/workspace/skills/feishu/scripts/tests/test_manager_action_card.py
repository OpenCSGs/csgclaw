from argparse import Namespace
from contextlib import redirect_stdout
from pathlib import Path
from io import StringIO
import json
import sys
import unittest

SCRIPTS_DIR = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(SCRIPTS_DIR))

from feishu_setup import commands, csgclaw  # noqa: E402


class ManagerActionCardTest(unittest.TestCase):
    def test_manager_auto_recreate_returns_frontend_action_card_without_api_recreate(self):
        calls = []
        original_api_json = csgclaw.api_json

        def fake_api_json(*args, **kwargs):
            calls.append((args, kwargs))
            raise AssertionError("manager finalize must not call recreate API from the skill")

        csgclaw.api_json = fake_api_json
        try:
            args = Namespace(recreate="auto", role="manager")
            result = csgclaw.maybe_recreate(
                args,
                {"bot_id": "u-manager", "role": "manager"},
                worker_existed_before_ensure=None,
            )
        finally:
            csgclaw.api_json = original_api_json

        self.assertEqual(calls, [])
        self.assertEqual(result["type"], "csgclaw.action_card")
        self.assertEqual(result["status"], "manager_recreate_pending")
        self.assertEqual(result["bot_id"], "u-manager")
        self.assertEqual(result["actions"][0]["id"], "rebuild-manager")
        self.assertEqual(result["actions"][0]["method"], "manager-bootstrap-replace")
        self.assertNotIn("fallback", result)
        self.assertNotIn("non_web_instruction", result)

    def test_worker_finalize_continues_recreate_when_same_bot_already_exists(self):
        originals = {
            "load_state": commands.load_state,
            "poll_until_success": commands.poll_until_success,
            "configure_csgclaw": commands.configure_csgclaw,
            "ensure_bot": commands.ensure_bot,
            "maybe_recreate": commands.maybe_recreate,
            "delete_state": commands.delete_state,
            "add_manager_group_permission_info": commands.add_manager_group_permission_info,
        }
        observed = {}

        def fake_ensure_bot(args, state, result):
            raise RuntimeError(
                'CSGClaw API POST /api/v1/channels/feishu/participants failed: HTTP 400: '
                'bot name "web-dev" already exists in channel "feishu" with id "u-web-dev"'
            )

        def fake_maybe_recreate(args, state, worker_existed_before_ensure):
            observed["worker_existed_before_ensure"] = worker_existed_before_ensure
            return {"recreated": True}

        commands.load_state = lambda args: {
            "registration_id": "reg-worker",
            "bot_id": "u-web-dev",
            "role": "worker",
            "bot_name": "web-dev",
        }
        commands.poll_until_success = lambda args, state, wait: {
            "app_id": "cli_worker",
            "app_secret": "secret-value",
            "domain": "feishu",
        }
        commands.configure_csgclaw = lambda args, state, result: {
            "bot_id": "u-web-dev",
            "app_id": "cli_worker",
            "app_secret": "present",
            "reloaded": True,
        }
        commands.ensure_bot = fake_ensure_bot
        commands.maybe_recreate = fake_maybe_recreate
        commands.delete_state = lambda args, registration_id: None
        commands.add_manager_group_permission_info = lambda args, state, result, output: None
        try:
            args = Namespace(
                registration_id="reg-worker",
                timeout=1,
                no_configure=False,
                no_ensure_bot=False,
                role="worker",
                bot_name="",
                description="",
                recreate="auto",
                keep_state=True,
            )
            stdout = StringIO()
            with redirect_stdout(stdout):
                exit_code = commands.cmd_finalize(args)
        finally:
            for name, value in originals.items():
                setattr(commands, name, value)

        self.assertEqual(exit_code, 0)
        payload = json.loads(stdout.getvalue())
        self.assertIs(observed["worker_existed_before_ensure"], None)
        self.assertTrue(payload["bot_ensured"])
        self.assertEqual(payload["worker_recreate_policy"], "worker_recreated_after_config")
        self.assertEqual(payload["recreate"], {"recreated": True})

    def test_worker_finalize_recreates_after_new_bot_ensure(self):
        originals = {
            "load_state": commands.load_state,
            "poll_until_success": commands.poll_until_success,
            "configure_csgclaw": commands.configure_csgclaw,
            "ensure_bot": commands.ensure_bot,
            "maybe_recreate": commands.maybe_recreate,
            "delete_state": commands.delete_state,
            "add_manager_group_permission_info": commands.add_manager_group_permission_info,
        }
        observed = {}

        def fake_maybe_recreate(args, state, worker_existed_before_ensure):
            observed["worker_existed_before_ensure"] = worker_existed_before_ensure
            return {"recreated": True}

        commands.load_state = lambda args: {
            "registration_id": "reg-worker",
            "bot_id": "u-new-worker",
            "role": "worker",
            "bot_name": "new-worker",
        }
        commands.poll_until_success = lambda args, state, wait: {
            "app_id": "cli_worker",
            "app_secret": "secret-value",
            "domain": "feishu",
        }
        commands.configure_csgclaw = lambda args, state, result: {
            "bot_id": "u-new-worker",
            "app_id": "cli_worker",
            "app_secret": "present",
            "reloaded": True,
        }
        commands.ensure_bot = lambda args, state, result: {"id": "u-new-worker"}
        commands.maybe_recreate = fake_maybe_recreate
        commands.delete_state = lambda args, registration_id: None
        commands.add_manager_group_permission_info = lambda args, state, result, output: None
        try:
            args = Namespace(
                registration_id="reg-worker",
                timeout=1,
                no_configure=False,
                no_ensure_bot=False,
                role="worker",
                bot_name="",
                description="",
                recreate="auto",
                keep_state=True,
            )
            stdout = StringIO()
            with redirect_stdout(stdout):
                exit_code = commands.cmd_finalize(args)
        finally:
            for name, value in originals.items():
                setattr(commands, name, value)

        self.assertEqual(exit_code, 0)
        payload = json.loads(stdout.getvalue())
        self.assertIs(observed["worker_existed_before_ensure"], None)
        self.assertTrue(payload["bot_ensured"])
        self.assertEqual(payload["worker_recreate_policy"], "worker_recreated_after_config")
        self.assertEqual(payload["recreate"], {"recreated": True})

    def test_manager_finalize_promotes_action_card_to_top_level(self):
        originals = {
            "load_state": commands.load_state,
            "poll_until_success": commands.poll_until_success,
            "configure_csgclaw": commands.configure_csgclaw,
            "ensure_bot": commands.ensure_bot,
            "delete_state": commands.delete_state,
        }
        commands.load_state = lambda args: {
            "registration_id": "reg-1",
            "bot_id": "u-manager",
            "role": "manager",
            "bot_name": "manager",
        }
        commands.poll_until_success = lambda args, state, wait: {
            "app_id": "cli_example",
            "app_secret": "secret-value",
            "domain": "feishu",
            "open_id": "ou_example",
        }
        commands.configure_csgclaw = lambda args, state, result: {
            "bot_id": "u-manager",
            "app_id": "cli_example",
            "app_secret": "present",
            "reloaded": True,
        }
        commands.ensure_bot = lambda args, state, result: {"id": "u-manager"}
        commands.delete_state = lambda args, registration_id: None
        try:
            args = Namespace(
                registration_id="reg-1",
                timeout=1,
                no_configure=False,
                no_ensure_bot=False,
                role="manager",
                bot_name="",
                description="",
                recreate="auto",
                keep_state=True,
            )
            stdout = StringIO()
            with redirect_stdout(stdout):
                exit_code = commands.cmd_finalize(args)
        finally:
            for name, value in originals.items():
                setattr(commands, name, value)

        self.assertEqual(exit_code, 0)
        payload = json.loads(stdout.getvalue())
        self.assertEqual(payload["type"], "csgclaw.action_card")
        self.assertEqual(payload["status"], "manager_recreate_pending")
        self.assertEqual(payload["setup_status"], "configured")
        self.assertEqual(payload["actions"][0]["id"], "rebuild-manager")
        self.assertEqual(payload["app_secret"], "present")
        self.assertEqual(payload["manager_group_permission_app_id"], "cli_example")
        self.assertIn("/app/cli_example/auth", payload["manager_group_permission_url"])
        self.assertIn("im:chat.members:write_only", payload["manager_group_permission_url"])
        self.assertNotIn("fallback", payload)
        self.assertNotIn("non_web_instruction", payload)
        self.assertNotIn("render_target", payload)


if __name__ == "__main__":
    unittest.main()
