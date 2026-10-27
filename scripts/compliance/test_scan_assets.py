from __future__ import annotations

import hashlib
import importlib.util
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).with_name("scan_assets.py")
spec = importlib.util.spec_from_file_location("scan_assets", SCRIPT)
scan_assets = importlib.util.module_from_spec(spec)
assert spec and spec.loader
sys.modules[spec.name] = scan_assets
spec.loader.exec_module(scan_assets)


class ScanAssetsTests(unittest.TestCase):
    def test_sql_values_are_escaped_and_empty_scans_produce_valid_source(self):
        self.assertEqual(scan_assets.sqlq("O'Reilly"), "'O''Reilly'")
        self.assertEqual(
            scan_assets.current_asset_source([]),
            "(SELECT NULL::text AS asset_path, NULL::text AS sha256 WHERE FALSE)",
        )
        self.assertIn("VALUES", scan_assets.current_asset_source([
            {"asset_path": "frontend/src/a.ts", "sha256": "abc"},
        ]))

    def test_package_lock_paths_cannot_escape_frontend_root(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp) / "frontend"
            root.mkdir()
            safe = scan_assets.safe_package_directory(root, "node_modules/demo")
            self.assertEqual(safe, root / "node_modules/demo")
            for unsafe in ("../outside", "/etc/passwd", "node_modules/../../outside", "node_modules\\demo"):
                with self.subTest(path=unsafe):
                    with self.assertRaises(ValueError):
                        scan_assets.safe_package_directory(root, unsafe)

    def test_symlink_targets_outside_scan_root_are_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp) / "scan-root"
            outside = Path(temp) / "outside.svg"
            root.mkdir()
            outside.write_text("private asset bytes", encoding="utf-8")
            link = root / "linked.svg"
            try:
                link.symlink_to(outside)
            except OSError as exc:
                self.skipTest(f"symlinks unavailable: {exc}")
            self.assertFalse(scan_assets.path_is_within_root(root, link))
            self.assertTrue(scan_assets.path_is_within_root(root, root / "new-file.svg"))

    def test_license_symlink_outside_package_is_not_accepted_as_evidence(self):
        with tempfile.TemporaryDirectory() as temp:
            package = Path(temp) / "frontend/node_modules/demo"
            package.mkdir(parents=True)
            outside = Path(temp) / "outside-license.txt"
            outside.write_text("must not be read as package license", encoding="utf-8")
            link = package / "LICENSE"
            try:
                link.symlink_to(outside)
            except OSError as exc:
                self.skipTest(f"symlinks unavailable: {exc}")
            self.assertIsNone(scan_assets.find_license_file(package))

    def test_symlink_loops_are_rejected_without_crashing(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp) / "scan-root"
            root.mkdir()
            first, second = root / "first", root / "second"
            try:
                first.symlink_to(second)
                second.symlink_to(first)
            except OSError as exc:
                self.skipTest(f"symlinks unavailable: {exc}")
            self.assertFalse(scan_assets.path_is_within_root(root, first))
            self.assertIsNone(scan_assets.find_license_file(root))

    def test_end_to_end_inventory_records_escaping_symlink_without_reading_target(self):
        with tempfile.TemporaryDirectory() as temp:
            repo = Path(temp) / "repo"
            frontend = repo / "frontend"
            (frontend / "src").mkdir(parents=True)
            (frontend / "public").mkdir()
            (frontend / "node_modules/demo").mkdir(parents=True)
            (frontend / "src/index.ts").write_text("export const ok = true;", encoding="utf-8")
            (frontend / "public/THIRD_PARTY_NOTICES.txt").write_text("Third-party notices", encoding="utf-8")
            (frontend / "node_modules/demo/LICENSE").write_text("MIT License", encoding="utf-8")
            (frontend / "package-lock.json").write_text(json.dumps({
                "name": "fixture",
                "lockfileVersion": 3,
                "packages": {
                    "": {"dependencies": {"demo": "1.0.0"}, "devDependencies": {"ignored": "1.0.0"}},
                    "node_modules/demo": {"version": "1.0.0", "license": "MIT"},
                    "node_modules/ignored": {"version": "1.0.0", "license": "GPL-3.0-only"},
                },
            }), encoding="utf-8")
            outside = Path(temp) / "outside.svg"
            outside.write_text("must not be hashed", encoding="utf-8")
            try:
                (frontend / "src/external.svg").symlink_to(outside)
            except OSError as exc:
                self.skipTest(f"symlinks unavailable: {exc}")

            subprocess.run(["git", "init", "-q", str(repo)], check=True)
            subprocess.run(["git", "-C", str(repo), "config", "user.email", "test@example.invalid"], check=True)
            subprocess.run(["git", "-C", str(repo), "config", "user.name", "Test"], check=True)
            subprocess.run(["git", "-C", str(repo), "add", "frontend"], check=True)
            subprocess.run(["git", "-C", str(repo), "commit", "-qm", "fixture"], check=True)
            # Deliberately scan a dirty worktree to verify that the report records
            # the commit and the dirty state separately.
            (frontend / "src/index.ts").write_text("export const ok = false;", encoding="utf-8")

            output = Path(temp) / "reports"
            run = subprocess.run([
                sys.executable, str(SCRIPT), "--repo", str(repo),
                "--release", str(Path(temp) / "no-release"), "--out", str(output),
            ], check=False, capture_output=True, text=True)
            self.assertEqual(run.returncode, 0, run.stderr)
            inventory = json.loads((output / "asset-inventory.json").read_text(encoding="utf-8"))
            escape = next(a for a in inventory["assets"] if a["asset_path"] == "frontend/src/external.svg")
            self.assertIn("escapes the configured scan root", escape["metadata"]["scan_error"])
            self.assertEqual(escape["sha256"], "")
            self.assertTrue(inventory["source_worktree_dirty"])
            self.assertTrue(inventory["source_revision"])
            dependency_names = {a["asset_name"] for a in inventory["assets"] if a["asset_type"] == "software_dependency"}
            self.assertEqual(dependency_names, {"demo@1.0.0"})
            seed_sql = (output / "seed-compliance-inventory.sql").read_text(encoding="utf-8")
            self.assertIn("completed_with_warnings", seed_sql)
            self.assertIn("worktree_dirty=true", seed_sql)
            self.assertNotIn(hashlib.sha256(b"must not be hashed").hexdigest(), seed_sql)

    def test_empty_inventory_source_is_valid_select(self):
        self.assertEqual(
            scan_assets.current_asset_source([]),
            "(SELECT NULL::text AS asset_path, NULL::text AS sha256 WHERE FALSE)",
        )

    def test_empty_inventory_does_not_generate_mass_deactivation(self):
        with tempfile.TemporaryDirectory() as temp:
            repo = Path(temp) / "repo"
            frontend = repo / "frontend"
            (frontend / "src").mkdir(parents=True)
            (frontend / "public").mkdir()
            (frontend / "package-lock.json").write_text(json.dumps({
                "name": "empty-fixture", "lockfileVersion": 3, "packages": {"": {}},
            }), encoding="utf-8")
            subprocess.run(["git", "init", "-q", str(repo)], check=True)
            subprocess.run(["git", "-C", str(repo), "config", "user.email", "test@example.invalid"], check=True)
            subprocess.run(["git", "-C", str(repo), "config", "user.name", "Test"], check=True)
            subprocess.run(["git", "-C", str(repo), "add", "frontend"], check=True)
            subprocess.run(["git", "-C", str(repo), "commit", "-qm", "empty fixture"], check=True)
            output = Path(temp) / "reports"
            run = subprocess.run([
                sys.executable, str(SCRIPT), "--repo", str(repo),
                "--release", str(Path(temp) / "no-release"), "--out", str(output),
            ], check=False, capture_output=True, text=True)
            self.assertEqual(run.returncode, 0, run.stderr)
            sql = (output / "seed-compliance-inventory.sql").read_text(encoding="utf-8")
            self.assertIn("Empty inventory: existing assets are left unchanged", sql)
            self.assertNotIn("UPDATE portal.compliance_assets old SET published_or_active=FALSE", sql)


if __name__ == "__main__":
    unittest.main()
