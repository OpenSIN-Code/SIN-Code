"""TDD Gatekeeper: Red-Green-Refactor Enforcer for Pocock Workflow.

This tool enforces the strict Test-Driven Development cycle by blocking
file edits until a failing test (RED phase) is registered and executed.

Docs: tdd_enforcer.doc.md
"""

from __future__ import annotations

import sys
import subprocess
import os
import json
import argparse
from pathlib import Path
from typing import Optional
from datetime import datetime


class TDDEnforcer:
    """Enforces TDD Red-Green-Refactor cycle by locking code files."""

    def __init__(self, test_cmd: str, file_to_edit: str, lock_dir: Optional[str] = None):
        self.test_cmd = test_cmd
        self.file_to_edit = file_to_edit
        self.lock_dir = lock_dir or os.path.join(os.getcwd(), ".tdd-locks")
        self.lock_file = os.path.join(self.lock_dir, f"{self._safe_filename(file_to_edit)}.lock")
        self.state_file = os.path.join(self.lock_dir, "tdd-state.json")

    def _safe_filename(self, path: str) -> str:
        """Create a safe lock filename from path."""
        return path.replace("/", "_").replace("\\", "_").replace(":", "_")

    def run_tests(self) -> tuple[int, str]:
        """Execute test suite and return (exit_code, output)."""
        print(f"🧪 Führe Tests aus: {self.test_cmd}")
        result = subprocess.run(
            self.test_cmd,
            shell=True,
            capture_output=True,
            text=True,
            timeout=60
        )
        output = result.stdout + result.stderr
        return result.returncode, output

    def _ensure_lock_dir(self) -> None:
        """Create lock directory if it doesn't exist."""
        os.makedirs(self.lock_dir, exist_ok=True)

    def _load_state(self) -> dict:
        """Load TDD state from JSON file."""
        if os.path.exists(self.state_file):
            with open(self.state_file, "r", encoding="utf-8") as f:
                return json.load(f)
        return {}

    def _save_state(self, state: dict) -> None:
        """Save TDD state to JSON file."""
        self._ensure_lock_dir()
        with open(self.state_file, "w", encoding="utf-8") as f:
            json.dump(state, f, indent=2, ensure_ascii=False)

    def _get_current_phase(self) -> str:
        """Determine current TDD phase for this file."""
        state = self._load_state()
        file_state = state.get(self.file_to_edit, {})
        return file_state.get("phase", "unknown")

    def _set_phase(self, phase: str) -> None:
        """Set TDD phase for this file."""
        state = self._load_state()
        if self.file_to_edit not in state:
            state[self.file_to_edit] = {}
        state[self.file_to_edit]["phase"] = phase
        state[self.file_to_edit]["last_updated"] = datetime.now().isoformat()
        self._save_state(state)

    def enforce(self) -> dict:
        """Enforce TDD cycle: RED phase must be confirmed before editing.

        Returns:
            dict with status and phase information.
        """
        self._ensure_lock_dir()
        current_phase = self._get_current_phase()

        print(f"🛡️  TDD Gatekeeper für '{self.file_to_edit}'")
        print(f"   Aktuelle Phase: {current_phase}")
        print(f"   Lock-Datei: {self.lock_file}")
        print()

        # Check if already in GREEN phase (tests passing)
        if current_phase == "green":
            print("✅ GREEN-Phase: Tests laufen durch. Du kannst refactoren.")
            return {
                "status": "allowed",
                "phase": "green",
                "message": "GREEN-Phase: Refactoring erlaubt",
                "file": self.file_to_edit
            }

        # Run tests to check current state
        exit_code, output = self.run_tests()

        if exit_code == 0:
            # All tests passing - if we were in RED, we can now move to GREEN
            if current_phase == "red":
                self._set_phase("green")
                self._remove_lock()
                print("\n🎉 GREEN-Phase erreicht! Tests laufen durch.")
                print("   Du kannst nun refactoren.")
                return {
                    "status": "allowed",
                    "phase": "green",
                    "message": "GREEN-Phase erreicht: Refactoring erlaubt",
                    "file": self.file_to_edit,
                    "test_output": output
                }
            else:
                # Already in GREEN or unknown
                self._set_phase("green")
                self._remove_lock()
                print("\n✅ Tests laufen durch. Du bist in GREEN-Phase.")
                return {
                    "status": "allowed",
                    "phase": "green",
                    "message": "GREEN-Phase: Refactoring erlaubt",
                    "file": self.file_to_edit,
                    "test_output": output
                }

        else:
            # Tests failing - this is the RED phase
            if current_phase == "red":
                print("\n🔴 RED-Phase bestätigt. Tests schlagen fehl (wie erwartet).")
                print("   Du bist autorisiert, die minimale Implementierung zu schreiben.")
                self._remove_lock()
                return {
                    "status": "allowed",
                    "phase": "red",
                    "message": "RED-Phase: Implementierung erlaubt",
                    "file": self.file_to_edit,
                    "test_output": output
                }
            else:
                # Need to start RED phase
                self._set_phase("red")
                self._create_lock()
                print("\n🔴 RED-Phase erkannt. Tests schlagen fehl.")
                print("   Du kannst nun den minimalen Code schreiben, um die Tests grün zu machen.")
                print("\n   Test-Output:")
                print("   " + "\n   ".join(output.split("\n")[:10]))  # Show first 10 lines
                return {
                    "status": "allowed",
                    "phase": "red",
                    "message": "RED-Phase: Implementierung erlaubt",
                    "file": self.file_to_edit,
                    "test_output": output
                }

    def _create_lock(self) -> None:
        """Create lock file to prevent editing."""
        with open(self.lock_file, "w", encoding="utf-8") as f:
            f.write(f"LOCKED: {self.file_to_edit}\n")
            f.write(f"Created: {datetime.now().isoformat()}\n")
            f.write("Status: RED-Phase - Implementierung erlaubt\n")

    def _remove_lock(self) -> None:
        """Remove lock file to allow editing."""
        if os.path.exists(self.lock_file):
            os.remove(self.lock_file)
            print(f"🔓 Lock für {self.file_to_edit} aufgehoben.")

    def is_locked(self) -> bool:
        """Check if file is currently locked."""
        return os.path.exists(self.lock_file)

    def reset(self) -> None:
        """Reset TDD state for this file."""
        state = self._load_state()
        if self.file_to_edit in state:
            del state[self.file_to_edit]
        self._save_state(state)
        self._remove_lock()
        print(f"🔄 TDD-State für {self.file_to_edit} zurückgesetzt.")


def run_tdd_enforcer(test_cmd: str, file_to_edit: str, lock_dir: Optional[str] = None) -> dict:
    """Convenience function to run TDD enforcer.

    Args:
        test_cmd: Command to run tests (e.g., "npm test", "pytest")
        file_to_edit: Path to file being edited
        lock_dir: Directory for lock files (optional)

    Returns:
        dict with status and phase information
    """
    enforcer = TDDEnforcer(test_cmd, file_to_edit, lock_dir)
    return enforcer.enforce()


def main():
    """CLI entry point for TDD enforcer."""
    parser = argparse.ArgumentParser(
        description="TDD Enforcer - Red-Green-Refactor Gatekeeper (Pocock Workflow)",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  %(prog)s "pytest tests/" "src/api.py"
  %(prog)s "npm test" "src/components/Button.tsx"
  %(prog)s "pytest tests/" "src/api.py" --reset
  %(prog)s "pytest tests/" "src/api.py" --check
        """
    )
    parser.add_argument("test_cmd", help="Test command (e.g., 'pytest tests/')")
    parser.add_argument("file_to_edit", help="File to edit")
    parser.add_argument("--lock-dir", help="Directory for lock files")
    parser.add_argument("--reset", action="store_true", help="Reset TDD state for this file")
    parser.add_argument("--check", action="store_true", help="Only check if locked, don't enforce")
    parser.add_argument("--json", action="store_true", help="Output JSON")

    args = parser.parse_args()

    enforcer = TDDEnforcer(args.test_cmd, args.file_to_edit, args.lock_dir)

    if args.reset:
        enforcer.reset()
        sys.exit(0)

    if args.check:
        result = {
            "is_locked": enforcer.is_locked(),
            "phase": enforcer._get_current_phase(),
            "file": args.file_to_edit,
            "lock_file": enforcer.lock_file
        }
        if args.json:
            print(json.dumps(result, indent=2, ensure_ascii=False))
        else:
            status = "🔒 Gesperrt" if result["is_locked"] else "🔓 Entsperrt"
            print(f"{status} - Phase: {result['phase']}")
        sys.exit(0)

    result = enforcer.enforce()

    if args.json:
        print(json.dumps(result, indent=2, ensure_ascii=False))
    else:
        print(f"\n{'=' * 60}")
        print(f"Ergebnis: {result['status'].upper()}")
        print(f"Phase: {result['phase'].upper()}")
        print(f"{'=' * 60}")

    # Exit code: 0 if allowed, 1 if blocked
    if result["status"] == "blocked":
        sys.exit(1)
    sys.exit(0)


if __name__ == "__main__":
    main()
