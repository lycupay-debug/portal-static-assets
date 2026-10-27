from __future__ import annotations

import importlib.util
import sys
import unittest
from pathlib import Path

SCRIPT = Path(__file__).with_name("go-ssh-mcp-service-log-sync.py")
spec = importlib.util.spec_from_file_location("go_ssh_mcp_service_log_sync", SCRIPT)
sync = importlib.util.module_from_spec(spec)
assert spec and spec.loader
sys.modules[spec.name] = sync
spec.loader.exec_module(sync)


class ServiceLogSyncTests(unittest.TestCase):
    def test_projects_only_sanitized_allowlisted_fields(self):
        record = sync.project({
            "operation": "database.query",
            "timestamp": "2026-10-28T00:00:00+00:00",
            "success": True,
            "duration_ms": 42,
            "returncode": 0,
            "sql": "SELECT secret FROM users",
            "stdout": "private output",
            "stderr": "private error",
            "token": "never-copy-this",
            "username": "operator",
        })
        self.assertIsNotNone(record)
        self.assertEqual(record["action"], "tool.operation.completed")
        self.assertEqual(record["outcome"], "success")
        self.assertEqual(record["details"], {
            "tool_source": "Go SSH MCP",
            "operation": "database.query",
            "duration_ms": 42,
            "returncode": 0,
        })
        self.assertNotIn("secret", str(record))
        self.assertNotIn("private output", str(record))
        self.assertNotIn("never-copy-this", str(record))

    def test_rejects_non_string_and_unknown_operations(self):
        for operation in (None, [], {}, 123, "unknown.operation"):
            with self.subTest(operation=operation):
                self.assertIsNone(sync.project({
                    "operation": operation,
                    "timestamp": "2026-10-28T00:00:00+00:00",
                }))

    def test_boolean_returncode_is_not_treated_as_an_exit_code(self):
        record = sync.project({
            "operation": "shell.exec",
            "timestamp": "2026-10-28T00:00:00+00:00",
            "returncode": True,
        })
        self.assertIsNotNone(record)
        self.assertEqual(record["outcome"], "success")
        self.assertNotIn("returncode", record["details"])

    def test_timeout_takes_precedence_over_nonzero_returncode(self):
        record = sync.project({
            "operation": "execute_parallel",
            "timestamp": "2026-10-28T00:00:00+00:00",
            "timed_out": True,
            "returncode": 124,
        })
        self.assertEqual(record["outcome"], "timeout")
        self.assertTrue(record["details"]["timed_out"])

    def test_timestamps_require_timezone_and_duration_excludes_boolean(self):
        self.assertIsNone(sync.parse_time("2026-10-28T00:00:00"))
        self.assertIsNone(sync.parse_time("not-a-time"))
        record = sync.project({
            "operation": "file.read",
            "timestamp": "2026-10-28T00:00:00Z",
            "duration_ms": True,
        })
        self.assertIsNotNone(record)
        self.assertNotIn("duration_ms", record["details"])


if __name__ == "__main__":
    unittest.main()
