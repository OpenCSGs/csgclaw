import importlib.util
import json
import tempfile
import unittest
from argparse import Namespace
from pathlib import Path


MODULE_PATH = Path(__file__).resolve().parent / "manager_worker_api.py"
SPEC = importlib.util.spec_from_file_location("manager_worker_api", MODULE_PATH)
if SPEC is None or SPEC.loader is None:
    raise RuntimeError(f"Unable to load module from {MODULE_PATH}")
manager_worker_api = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(manager_worker_api)


PARTICIPANT_ID = "u-manager"
ROOM_ID = "room-123"
TODO_PATH = "/tmp/project/todo.json"


def make_task(task_id, assignee, *, passes):
    return {
        "id": task_id,
        "assignee": assignee,
        "category": "feature",
        "description": f"task {task_id}",
        "steps": ["do the work"],
        "passes": passes,
        "progress_note": "",
    }


def make_message(sender_id, content, created_at):
    return {
        "id": f"msg-{sender_id}-{created_at}",
        "sender_id": sender_id,
        "content": content,
        "created_at": created_at,
    }


def make_bootstrap():
    return {
        "current_user_id": "u-admin",
        "users": [
            {"id": "u-manager", "name": "manager"},
            {"id": "u-ux", "name": "ux"},
            {"id": "u-dev", "name": "dev"},
            {"id": "u-qa", "name": "qa"},
        ],
        "rooms": [
            {
                "id": ROOM_ID,
                "members": ["u-admin", "u-manager", "u-ux", "u-dev", "u-qa"],
            }
        ],
    }


def dispatch_message(task):
    return manager_worker_api.build_tracking_message(task, TODO_PATH)


class TrackingDecisionTests(unittest.TestCase):
    def decide(self, tasks, messages, bootstrap=None):
        if bootstrap is None:
            bootstrap = make_bootstrap()
        return manager_worker_api.decide_tracking_action(
            tasks,
            messages,
            bootstrap,
            participant_id=PARTICIPANT_ID,
            room_id=ROOM_ID,
            todo_path=TODO_PATH,
            retry_in_seconds=2.0,
        )

    def test_first_task_dispatches_immediately(self):
        task1 = make_task(1, "ux", passes=False)

        decision = self.decide([task1], [])

        self.assertEqual(decision["kind"], "dispatch")
        self.assertEqual(decision["task"]["id"], 1)
        self.assertEqual(decision["text"], dispatch_message(task1))
        self.assertEqual(decision["mention_id"], "u-ux")

    def test_waits_for_task_passes_when_current_task_already_dispatched(self):
        task1 = make_task(1, "ux", passes=False)
        messages = [
            make_message(PARTICIPANT_ID, dispatch_message(task1), "2026-04-10T08:26:40Z"),
        ]

        decision = self.decide([task1, make_task(2, "dev", passes=False)], messages)

        self.assertEqual(decision["kind"], "wait")
        self.assertEqual(decision["output"]["event"], "waiting-for-task-passes")
        self.assertEqual(decision["output"]["task_id"], 1)

    def test_waits_for_task_passes_when_csgclaw_message_has_mention_prefix(self):
        task1 = make_task(1, "dev", passes=False)
        messages = [
            make_message(PARTICIPANT_ID, f"@dev {dispatch_message(task1)}", "2026-04-10T08:26:40Z"),
        ]

        decision = self.decide([task1, make_task(2, "qa", passes=False)], messages)

        self.assertEqual(decision["kind"], "wait")
        self.assertEqual(decision["output"]["event"], "waiting-for-task-passes")
        self.assertEqual(decision["output"]["task_id"], 1)

    def test_waits_for_assignee_reply_after_previous_task_passes(self):
        task1 = make_task(1, "ux", passes=True)
        task2 = make_task(2, "dev", passes=False)
        messages = [
            make_message(PARTICIPANT_ID, dispatch_message(task1), "2026-04-10T08:26:40Z"),
        ]

        decision = self.decide([task1, task2], messages)

        self.assertEqual(decision["kind"], "wait")
        self.assertEqual(decision["output"]["event"], "waiting-for-assignee-reply")
        self.assertEqual(decision["output"]["task_id"], 1)
        self.assertEqual(decision["output"]["pending_task_id"], 2)

    def test_tool_trace_does_not_count_as_assignee_reply(self):
        task1 = make_task(1, "ux", passes=True)
        task2 = make_task(2, "dev", passes=False)
        messages = [
            make_message(PARTICIPANT_ID, dispatch_message(task1), "2026-04-10T08:26:40Z"),
            make_message("u-ux", "🔧 `read_file`\n```\n{\"path\":\"/tmp/todo.json\"}\n```", "2026-04-10T08:26:44Z"),
        ]

        decision = self.decide([task1, task2], messages)

        self.assertEqual(decision["kind"], "wait")
        self.assertEqual(decision["output"]["event"], "waiting-for-assignee-reply")

    def test_dispatches_next_task_after_human_reply_and_pass(self):
        task1 = make_task(1, "ux", passes=True)
        task2 = make_task(2, "dev", passes=False)
        messages = [
            make_message(PARTICIPANT_ID, dispatch_message(task1), "2026-04-10T08:26:40Z"),
            make_message("u-ux", "任务1已完成，设计文档已经交付。", "2026-04-10T08:32:49Z"),
        ]

        decision = self.decide([task1, task2], messages)

        self.assertEqual(decision["kind"], "dispatch")
        self.assertEqual(decision["task"]["id"], 2)
        self.assertEqual(decision["text"], dispatch_message(task2))
        self.assertEqual(decision["mention_id"], "u-dev")

    def test_dispatches_when_reply_exists_before_current_poll_once_pass_is_true(self):
        task1 = make_task(1, "ux", passes=True)
        task2 = make_task(2, "dev", passes=False)
        messages = [
            make_message(PARTICIPANT_ID, dispatch_message(task1), "2026-04-10T08:26:40Z"),
            make_message("u-ux", "设计工作完成，等待下一步。", "2026-04-10T08:27:10Z"),
        ]

        decision = self.decide([task1, task2], messages)

        self.assertEqual(decision["kind"], "dispatch")
        self.assertEqual(decision["task"]["id"], 2)
        self.assertEqual(decision["mention_id"], "u-dev")

    def test_unresolved_assignee_raises_clear_error(self):
        task1 = make_task(1, "ghost", passes=True)
        task2 = make_task(2, "dev", passes=False)
        messages = [
            make_message(PARTICIPANT_ID, dispatch_message(task1), "2026-04-10T08:26:40Z"),
        ]

        with self.assertRaises(manager_worker_api.TrackingError) as ctx:
            self.decide([task1, task2], messages)

        self.assertIn('Task assignee "ghost"', str(ctx.exception))
        self.assertIn(ROOM_ID, str(ctx.exception))


class SimpleTrackingDecisionTests(unittest.TestCase):
    def decide(self, tasks, previous_pending_index=None):
        return manager_worker_api.decide_simple_tracking_action(
            tasks,
            room_id=ROOM_ID,
            todo_path=TODO_PATH,
            retry_in_seconds=2.0,
            previous_pending_index=previous_pending_index,
        )

    def test_first_task_dispatches_without_delay(self):
        task1 = make_task(1, "ux", passes=False)

        decision = self.decide([task1])

        self.assertEqual(decision["kind"], "dispatch")
        self.assertEqual(decision["task"]["id"], 1)
        self.assertEqual(decision["mention_id"], "u-ux")
        self.assertEqual(decision["delay_seconds"], 0.0)
        self.assertEqual(decision["pending_index"], 0)

    def test_next_task_dispatches_with_delay_after_previous_task_passes(self):
        task1 = make_task(1, "ux", passes=True)
        task2 = make_task(2, "dev", passes=False)

        decision = self.decide([task1, task2], previous_pending_index=0)

        self.assertEqual(decision["kind"], "dispatch")
        self.assertEqual(decision["task"]["id"], 2)
        self.assertEqual(decision["mention_id"], "u-dev")
        self.assertEqual(decision["delay_seconds"], manager_worker_api.FEISHU_DISPATCH_DELAY_SECONDS)
        self.assertEqual(decision["pending_index"], 1)

    def test_same_pending_index_waits_for_passes(self):
        task1 = make_task(1, "ux", passes=True)
        task2 = make_task(2, "dev", passes=False)

        decision = self.decide([task1, task2], previous_pending_index=1)

        self.assertEqual(decision["kind"], "wait")
        self.assertEqual(decision["output"]["event"], "waiting-for-task-passes")
        self.assertEqual(decision["output"]["task_id"], 2)

    def test_explicit_mention_id_is_used(self):
        task1 = make_task(1, "ux", passes=False)
        task1["mention_id"] = "custom-worker"

        decision = self.decide([task1])

        self.assertEqual(decision["kind"], "dispatch")
        self.assertEqual(decision["mention_id"], "custom-worker")

    def test_assignee_id_is_used_for_simple_dispatch(self):
        task1 = make_task(1, "ux", passes=False)
        task1["assignee_id"] = "u-explicit"

        decision = self.decide([task1])

        self.assertEqual(decision["kind"], "dispatch")
        self.assertEqual(decision["mention_id"], "u-explicit")


class CSGClawAPITests(unittest.TestCase):
    def test_dry_run_send_participant_message_uses_cli_message_create_with_mention(self):
        api = manager_worker_api.CSGClawAPI(
            base_url="http://example.test",
            token=None,
            timeout=30,
            dry_run=True,
        )

        result = api.send_participant_message("feishu", ROOM_ID, PARTICIPANT_ID, "u-dev", "hello")

        self.assertEqual(
            result["command"],
            [
                "csgclaw-cli",
                "--endpoint",
                "http://example.test",
                "--output",
                "json",
                "message",
                "create",
                "--channel",
                "feishu",
                "--room-id",
                ROOM_ID,
                "--sender-id",
                PARTICIPANT_ID,
                "--mention-id",
                "u-dev",
                "--content",
                "hello",
            ],
        )

    def test_list_messages_uses_channel_specific_path(self):
        api = manager_worker_api.CSGClawAPI(
            base_url="http://example.test",
            token=None,
            timeout=30,
            dry_run=True,
        )

        self.assertEqual(api.list_messages("feishu", "oc_alpha"), [])


class RunTrackingTests(unittest.TestCase):
    def test_missing_todo_file_logs_retry_without_crashing(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            todo_path = str(Path(temp_dir) / "missing.json")
            log_path = manager_worker_api.get_tracking_state_paths(todo_path)[1]
            args = Namespace(
                base_url="http://example.test",
                token=None,
                timeout=30,
                dry_run=True,
                channel="csgclaw",
                room_id=ROOM_ID,
                participant_id=PARTICIPANT_ID,
                todo_path=todo_path,
                interval=0.01,
                once=False,
            )

            sleep_calls = {"count": 0}
            original_sleep = manager_worker_api.time.sleep

            def fake_sleep(_seconds):
                sleep_calls["count"] += 1
                raise KeyboardInterrupt

            manager_worker_api.time.sleep = fake_sleep
            try:
                with self.assertRaises(KeyboardInterrupt):
                    manager_worker_api.cmd_run_tracking(args)
            finally:
                manager_worker_api.time.sleep = original_sleep

            self.assertEqual(sleep_calls["count"], 1)
            log_lines = log_path.read_text(encoding="utf-8").strip().splitlines()
            events = [json.loads(line)["event"] for line in log_lines]
            self.assertIn("tracking-run-started", events)
            self.assertIn("tracking-loop-start", events)
            self.assertIn("tracking-read-retry", events)


if __name__ == "__main__":
    unittest.main()
