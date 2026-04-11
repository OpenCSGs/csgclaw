import importlib.util
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).resolve().parent / "manager_worker_api.py"
SPEC = importlib.util.spec_from_file_location("manager_worker_api", MODULE_PATH)
if SPEC is None or SPEC.loader is None:
    raise RuntimeError(f"Unable to load module from {MODULE_PATH}")
manager_worker_api = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(manager_worker_api)


BOT_ID = "u-manager"
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
            {"id": "u-manager", "handle": "manager", "name": "manager"},
            {"id": "u-ux", "handle": "ux", "name": "ux"},
            {"id": "u-dev", "handle": "dev", "name": "dev"},
            {"id": "u-qa", "handle": "qa", "name": "qa"},
        ],
        "rooms": [
            {
                "id": ROOM_ID,
                "participants": ["u-admin", "u-manager", "u-ux", "u-dev", "u-qa"],
            }
        ],
    }


def dispatch_message(task):
    return manager_worker_api.build_tracking_message(task, None, TODO_PATH)


class TrackingDecisionTests(unittest.TestCase):
    def decide(self, tasks, messages, bootstrap=None):
        if bootstrap is None:
            bootstrap = make_bootstrap()
        return manager_worker_api.decide_tracking_action(
            tasks,
            messages,
            bootstrap,
            bot_id=BOT_ID,
            room_id=ROOM_ID,
            mention=None,
            todo_path=TODO_PATH,
            retry_in_seconds=2.0,
        )

    def test_first_task_dispatches_immediately(self):
        task1 = make_task(1, "ux", passes=False)

        decision = self.decide([task1], [])

        self.assertEqual(decision["kind"], "dispatch")
        self.assertEqual(decision["task"]["id"], 1)
        self.assertEqual(decision["text"], dispatch_message(task1))

    def test_waits_for_task_passes_when_current_task_already_dispatched(self):
        task1 = make_task(1, "ux", passes=False)
        messages = [
            make_message(BOT_ID, dispatch_message(task1), "2026-04-10T08:26:40Z"),
        ]

        decision = self.decide([task1, make_task(2, "dev", passes=False)], messages)

        self.assertEqual(decision["kind"], "wait")
        self.assertEqual(decision["output"]["event"], "waiting-for-task-passes")
        self.assertEqual(decision["output"]["task_id"], 1)

    def test_waits_for_assignee_reply_after_previous_task_passes(self):
        task1 = make_task(1, "ux", passes=True)
        task2 = make_task(2, "dev", passes=False)
        messages = [
            make_message(BOT_ID, dispatch_message(task1), "2026-04-10T08:26:40Z"),
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
            make_message(BOT_ID, dispatch_message(task1), "2026-04-10T08:26:40Z"),
            make_message("u-ux", "🔧 `read_file`\n```\n{\"path\":\"/tmp/todo.json\"}\n```", "2026-04-10T08:26:44Z"),
        ]

        decision = self.decide([task1, task2], messages)

        self.assertEqual(decision["kind"], "wait")
        self.assertEqual(decision["output"]["event"], "waiting-for-assignee-reply")

    def test_dispatches_next_task_after_human_reply_and_pass(self):
        task1 = make_task(1, "ux", passes=True)
        task2 = make_task(2, "dev", passes=False)
        messages = [
            make_message(BOT_ID, dispatch_message(task1), "2026-04-10T08:26:40Z"),
            make_message("u-ux", "任务1已完成，设计文档已经交付。", "2026-04-10T08:32:49Z"),
        ]

        decision = self.decide([task1, task2], messages)

        self.assertEqual(decision["kind"], "dispatch")
        self.assertEqual(decision["task"]["id"], 2)
        self.assertEqual(decision["text"], dispatch_message(task2))

    def test_dispatches_when_reply_exists_before_current_poll_once_pass_is_true(self):
        task1 = make_task(1, "ux", passes=True)
        task2 = make_task(2, "dev", passes=False)
        messages = [
            make_message(BOT_ID, dispatch_message(task1), "2026-04-10T08:26:40Z"),
            make_message("u-ux", "设计工作完成，等待下一步。", "2026-04-10T08:27:10Z"),
        ]

        decision = self.decide([task1, task2], messages)

        self.assertEqual(decision["kind"], "dispatch")
        self.assertEqual(decision["task"]["id"], 2)

    def test_unresolved_assignee_raises_clear_error(self):
        task1 = make_task(1, "ghost", passes=True)
        task2 = make_task(2, "dev", passes=False)
        messages = [
            make_message(BOT_ID, dispatch_message(task1), "2026-04-10T08:26:40Z"),
        ]

        with self.assertRaises(manager_worker_api.TrackingError) as ctx:
            self.decide([task1, task2], messages)

        self.assertIn('Task assignee "ghost"', str(ctx.exception))
        self.assertIn(ROOM_ID, str(ctx.exception))


if __name__ == "__main__":
    unittest.main()
