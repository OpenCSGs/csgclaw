from argparse import Namespace
from contextlib import redirect_stdout
from pathlib import Path
from io import BytesIO, StringIO
import json
import sys
import unittest
from unittest.mock import patch

SCRIPTS_DIR = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(SCRIPTS_DIR))

from feishu_setup import commands, csgclaw  # noqa: E402


class ManagerActionCardTest(unittest.TestCase):
    def test_manager_auto_recreate_returns_frontend_action_card_without_api_recreate(self):
        result = csgclaw.manager_recreate_action_card("u-manager")

        self.assertEqual(result["type"], "csgclaw.action_card")
        self.assertEqual(result["status"], "manager_recreate_pending")
        self.assertEqual(result["agent_id"], "u-manager")
        self.assertEqual(result["bot_id"], "u-manager")
        self.assertEqual(result["actions"][0]["id"], "rebuild-manager")
        self.assertEqual(result["actions"][0]["method"], "manager-bootstrap-replace")
        self.assertNotIn("fallback", result)
        self.assertNotIn("non_web_instruction", result)

    def test_bind_manager_reconciles_bridge_without_action_card(self):
        calls = []
        api_calls = []
        original_csgclaw_cli_json = commands.csgclaw_cli_json
        original_api_json = commands.api_json

        def fake_csgclaw_cli_json(args, cli_args, input_text=None):
            calls.append((cli_args, input_text))
            if "--feishu-kind" in cli_args and cli_args[cli_args.index("--feishu-kind") + 1] == "human":
                return {"participant_id": "admin", "config_saved": True}
            return {
                "participant_id": "manager",
                "agent_id": "u-manager",
                "restart_status": "restart_skipped",
            }

        def fake_api_json(args, method, path, body=None):
            api_calls.append((method, path, body))
            return {"id": "u-manager", "status": "running"}

        commands.csgclaw_cli_json = fake_csgclaw_cli_json
        commands.api_json = fake_api_json
        try:
            args = Namespace(
                agent="u-manager",
                app_id="cli_example",
                open_id="ou_example",
                name="",
                domain="feishu",
                app_secret_file="",
                app_secret_env="FEISHU_SECRET",
                app_secret_stdin=False,
            )
            stdout = StringIO()
            with redirect_stdout(stdout):
                exit_code = commands.cmd_bind_manager(args)
        finally:
            commands.csgclaw_cli_json = original_csgclaw_cli_json
            commands.api_json = original_api_json

        self.assertEqual(exit_code, 0)
        payload = json.loads(stdout.getvalue())
        self.assertEqual(payload["status"], "configured")
        self.assertEqual(payload["agent_id"], "u-manager")
        self.assertEqual(payload["bot_id"], "u-manager")
        self.assertEqual(payload["config"]["bot_bind"]["restart_status"], "restart_skipped")
        self.assertEqual(payload["config"]["binding_activation"]["status"], "running")
        self.assertTrue(payload["binding_activated"])
        self.assertNotIn("type", payload)
        self.assertNotIn("actions", payload)
        self.assertEqual(payload["manager_group_permission_app_id"], "cli_example")
        self.assertIn("/app/cli_example/auth", payload["manager_group_permission_url"])
        self.assertIn("im:chat.members:write_only", payload["manager_group_permission_url"])
        self.assertFalse(any("--restart" in call[0] for call in calls))
        self.assertEqual(api_calls, [("POST", "/api/v1/agents/u-manager/bindings:apply?channel=feishu", None)])
        self.assertTrue(any("--app-secret-env" in call[0] for call in calls))

    def test_start_recognizes_all_manager_references_without_registry_lookup(self):
        saved_states = []
        original_api_json = commands.api_json
        original_init_registration = commands.init_registration
        original_begin_registration = commands.begin_registration
        original_save_state = commands.save_state
        original_state_path = commands.state_path

        commands.api_json = lambda *args: self.fail(f"manager start must not query the worker registry: {args}")
        commands.init_registration = lambda domain: None
        commands.begin_registration = lambda domain: {
            "device_code": "device-code",
            "qr_url": "https://example.test/qr",
            "interval": 5,
            "expire_in": 600,
        }
        commands.save_state = lambda args, state: saved_states.append(state.copy())
        commands.state_path = lambda args, registration_id: Path("/tmp") / f"{registration_id}.json"
        try:
            for agent_ref in ("manager", "u-manager", "agent-manager"):
                with self.subTest(agent_ref=agent_ref):
                    args = Namespace(
                        agent=agent_ref,
                        domain="feishu",
                        role="",
                        bot_name="",
                        description="",
                        timeout=600,
                        json=True,
                        qr=False,
                        state_dir="",
                    )
                    stdout = StringIO()
                    with redirect_stdout(stdout):
                        exit_code = commands.cmd_start(args)
                    self.assertEqual(exit_code, 0)
                    payload = json.loads(stdout.getvalue())
                    self.assertEqual(payload["agent_id"], agent_ref)
                    self.assertEqual(payload["role"], "manager")
                    self.assertEqual(saved_states[-1]["agent_id"], agent_ref)
                    self.assertEqual(saved_states[-1]["role"], "manager")
        finally:
            commands.api_json = original_api_json
            commands.init_registration = original_init_registration
            commands.begin_registration = original_begin_registration
            commands.save_state = original_save_state
            commands.state_path = original_state_path

    def test_start_resolves_existing_worker_by_display_name_with_one_registry_request(self):
        api_calls = []
        saved_states = []
        originals = {
            "api_json": commands.api_json,
            "init_registration": commands.init_registration,
            "begin_registration": commands.begin_registration,
            "save_state": commands.save_state,
            "state_path": commands.state_path,
        }

        def fake_api_json(args, method, path, body=None):
            api_calls.append((method, path, body))
            if path == "/api/v1/agents":
                return {"items": [
                    {"id": "agent-meds2g", "name": "generic-assistant-codex", "role": "worker"},
                    {"id": "agent-otep2a", "name": "generic-assistant-codex-2", "role": "worker"},
                ]}
            self.fail(f"unexpected API request: {method} {path}")

        commands.api_json = fake_api_json
        commands.init_registration = lambda domain: None
        commands.begin_registration = lambda domain: {
            "device_code": "device-code",
            "qr_url": "https://example.test/qr",
            "interval": 5,
            "expire_in": 600,
        }
        commands.save_state = lambda args, state: saved_states.append(state.copy())
        commands.state_path = lambda args, registration_id: Path("/tmp") / f"{registration_id}.json"
        try:
            args = Namespace(
                agent="generic-assistant-codex",
                domain="feishu",
                role="worker",
                bot_name="",
                description="",
                timeout=600,
                json=True,
                qr=False,
                state_dir="",
            )
            stdout = StringIO()
            with redirect_stdout(stdout):
                exit_code = commands.cmd_start(args)
        finally:
            for name, value in originals.items():
                setattr(commands, name, value)

        self.assertEqual(exit_code, 0)
        payload = json.loads(stdout.getvalue())
        self.assertEqual(payload["agent_id"], "agent-meds2g")
        self.assertEqual(saved_states[0]["agent_id"], "agent-meds2g")
        self.assertEqual(saved_states[0]["bot_name"], "generic-assistant-codex")
        self.assertEqual(
            api_calls,
            [
                ("GET", "/api/v1/agents", None),
            ],
        )

    def test_resolve_worker_reference_matches_exact_runtime_id(self):
        api_calls = []
        original_api_json = commands.api_json

        def fake_api_json(args, method, path, body=None):
            api_calls.append((method, path, body))
            self.assertEqual(path, "/api/v1/agents")
            return {"items": [{"id": "agent-meds2g", "name": "generic-assistant-codex", "role": "worker"}]}

        commands.api_json = fake_api_json
        try:
            resolved = commands.resolve_worker_agent_reference(Namespace(), "agent-meds2g")
        finally:
            commands.api_json = original_api_json

        self.assertEqual(resolved, ("agent-meds2g", "generic-assistant-codex"))
        self.assertEqual(api_calls, [("GET", "/api/v1/agents", None)])

    def test_worker_registry_decodes_http_items_response(self):
        args = Namespace(csgclaw_base_url="http://localhost:18080", csgclaw_access_token="test-token")
        body = {"items": [{"id": "agent-reviewer", "name": "Reviewer", "role": "worker"}]}
        with patch.object(csgclaw, "urlopen", return_value=BytesIO(json.dumps(body).encode())) as request:
            resolved = commands.resolve_worker_agent_reference(args, "Reviewer")
        self.assertEqual(resolved, ("agent-reviewer", "Reviewer"))
        request.assert_called_once()
        self.assertEqual(request.call_args.args[0].full_url, "http://localhost:18080/api/v1/agents")
        self.assertEqual(request.call_args.args[0].get_method(), "GET")
        self.assertEqual(request.call_args.args[0].get_header("Authorization"), "Bearer test-token")

    def test_worker_registry_rejects_invalid_http_collection(self):
        args = Namespace(csgclaw_base_url="http://localhost:18080", csgclaw_access_token="test-token")
        for body in ({}, {"items": None}, {"items": {}}, []):
            with self.subTest(body=body):
                with patch.object(csgclaw, "urlopen", return_value=BytesIO(json.dumps(body).encode())):
                    with self.assertRaisesRegex(RuntimeError, "invalid agent list"):
                        commands.resolve_worker_agent_reference(args, "Reviewer")

    def test_start_rejects_ambiguous_worker_reference_before_registration(self):
        initialization = []
        original_api_json = commands.api_json
        original_init_registration = commands.init_registration

        def fake_api_json(args, method, path, body=None):
            if path == "/api/v1/agents":
                return {"items": [
                    {"id": "agent-dev", "name": "backend", "role": "worker"},
                    {"id": "agent-other", "name": "agent-dev", "role": "worker"},
                ]}
            self.fail(f"unexpected API request: {method} {path}")

        commands.api_json = fake_api_json
        commands.init_registration = lambda domain: initialization.append(domain)
        try:
            args = Namespace(agent="agent-dev", domain="feishu", role="worker")
            with self.assertRaisesRegex(RuntimeError, "reference .* matched multiple agents"):
                commands.cmd_start(args)
        finally:
            commands.api_json = original_api_json
            commands.init_registration = original_init_registration

        self.assertEqual(initialization, [])

    def test_configure_worker_skips_admin_from_registration_open_id(self):
        calls = []
        original_csgclaw_cli_json = csgclaw.csgclaw_cli_json

        def fake_csgclaw_cli_json(args, cli_args, input_text=None):
            calls.append((cli_args, input_text))
            return {
                "participant_id": "dev",
                "agent_id": "u-dev",
                "restart_status": "worker_recreated",
            }

        csgclaw.csgclaw_cli_json = fake_csgclaw_cli_json
        try:
            response = csgclaw.configure_csgclaw(
                Namespace(role="worker", recreate="auto"),
                {"agent_id": "u-dev", "role": "worker"},
                {"app_id": "cli_dev", "app_secret": "secret-value", "open_id": "ou_admin"},
            )
        finally:
            csgclaw.csgclaw_cli_json = original_csgclaw_cli_json

        self.assertNotIn("admin_bind", response)
        self.assertNotIn("admin_open_id", response)
        self.assertFalse(any("--feishu-kind" in call[0] and "human" in call[0] for call in calls))
        self.assertTrue(any("--restart" in call[0] for call in calls))
        self.assertTrue(any(call[1] == "secret-value" for call in calls))

    def test_configure_manager_passes_admin_name_from_registration(self):
        calls = []
        api_calls = []
        original_csgclaw_cli_json = csgclaw.csgclaw_cli_json
        original_api_json = csgclaw.api_json

        def fake_csgclaw_cli_json(args, cli_args, input_text=None):
            calls.append((cli_args, input_text))
            if "--feishu-kind" in cli_args and cli_args[cli_args.index("--feishu-kind") + 1] == "human":
                return {"participant_id": "admin", "config_saved": True}
            return {
                "participant_id": "manager",
                "agent_id": "u-manager",
                "restart_status": "restart_skipped",
            }

        def fake_api_json(args, method, path, body=None):
            api_calls.append((method, path, body))
            return {"id": "u-manager", "status": "running"}

        csgclaw.csgclaw_cli_json = fake_csgclaw_cli_json
        csgclaw.api_json = fake_api_json
        try:
            response = csgclaw.configure_csgclaw(
                Namespace(role="manager", recreate="auto"),
                {"agent_id": "u-manager", "role": "manager"},
                {"app_id": "cli_dev", "app_secret": "secret-value", "open_id": "ou_admin", "name": "龙韵"},
            )
        finally:
            csgclaw.csgclaw_cli_json = original_csgclaw_cli_json
            csgclaw.api_json = original_api_json

        admin_bind = next(call[0] for call in calls if "--feishu-kind" in call[0] and "human" in call[0])
        self.assertIn("--name", admin_bind)
        self.assertEqual(admin_bind[admin_bind.index("--name") + 1], "龙韵")
        self.assertEqual(response["admin_bind"]["participant_id"], "admin")
        self.assertEqual(response["binding_activation"]["status"], "running")
        self.assertEqual(api_calls, [("POST", "/api/v1/agents/u-manager/bindings:apply?channel=feishu", None)])
        self.assertEqual(response["admin_open_id"], "ou_admin")
        self.assertEqual(response["admin_open_id_source"], "registration")

    def test_manager_finalize_returns_reconciled_binding_without_action_card(self):
        originals = {
            "load_state": commands.load_state,
            "poll_until_success": commands.poll_until_success,
            "configure_csgclaw": commands.configure_csgclaw,
            "delete_state": commands.delete_state,
        }
        commands.load_state = lambda args: {
            "registration_id": "reg-1",
            "agent_id": "u-manager",
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
            "admin_open_id": "ou_example",
            "bot_bind": {
                "participant_id": "manager",
                "agent_id": "u-manager",
                "restart_status": "restart_skipped",
            },
            "binding_activation": {"id": "u-manager", "status": "running"},
        }
        commands.delete_state = lambda args, registration_id: None
        try:
            args = Namespace(
                registration_id="reg-1",
                timeout=1,
                no_configure=False,
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
        self.assertEqual(payload["status"], "configured")
        self.assertEqual(payload["agent_id"], "u-manager")
        self.assertEqual(payload["config"]["bot_bind"]["participant_id"], "manager")
        self.assertEqual(payload["activation"]["status"], "running")
        self.assertNotIn("type", payload)
        self.assertNotIn("actions", payload)
        self.assertEqual(payload["app_secret"], "present")
        self.assertEqual(payload["manager_group_permission_app_id"], "cli_example")
        self.assertIn("/app/cli_example/auth", payload["manager_group_permission_url"])
        self.assertIn("im:chat.members:write_only", payload["manager_group_permission_url"])
        self.assertNotIn("fallback", payload)
        self.assertNotIn("non_web_instruction", payload)
        self.assertNotIn("render_target", payload)


if __name__ == "__main__":
    unittest.main()
