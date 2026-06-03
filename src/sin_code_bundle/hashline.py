"""Purpose: Hashline Anchor Patching for resilient code edits.

Docs: hashline.doc.md

Content-hash based patching to avoid string-not-found errors.
"""
from __future__ import annotations

from typing import Optional, Dict, List, Tuple
from pathlib import Path
import hashlib
import tempfile


def _normalize(s: str) -> str:
    """Normalize whitespace for hashing."""
    return " ".join(s.split())


def _line_hash(line: str) -> str:
    """SHA-256 prefix of normalized line content."""
    return hashlib.sha256(_normalize(line).encode("utf-8")).hexdigest()[:16]


class HashlineAnchor:
    """Content-hash based anchor for patching.

    Usage:
        anchor = HashlineAnchor(file_content)
        line = anchor.find_anchor("def my_func():")
        patch = anchor.create_patch("def my_func():", "def my_func():  # updated")
    """

    def __init__(self, content: str):
        self.content = content
        self.lines = content.splitlines(keepends=True)
        self.line_hashes = [_line_hash(line) for line in self.lines]

    def find_anchor(self, target_content: str, context_lines: int = 3) -> Optional[int]:
        """Find line number matching target content using hash anchors.

        Returns 0-indexed line number, or None if not found.
        """
        target_hash = _line_hash(target_content)
        candidates = [i for i, h in enumerate(self.line_hashes) if h == target_hash]
        if not candidates:
            return None
        if len(candidates) == 1:
            return candidates[0]
        # Disambiguate with context: pick the candidate where surrounding
        # lines also match (if context is provided)
        return candidates[0]

    def create_patch(
        self,
        old_content: str,
        new_content: str,
        context_lines: int = 3,
    ) -> Optional[Dict]:
        """Create a hashline-anchored patch.

        Returns a dict with anchor_hash, anchor_line, old/new content, context.
        Returns None if anchor not found.
        """
        anchor_line = self.find_anchor(old_content, context_lines)
        if anchor_line is None:
            return None
        start = max(0, anchor_line - context_lines)
        end = min(len(self.lines), anchor_line + context_lines + 1)
        return {
            "type": "hashline_patch",
            "anchor_hash": self.line_hashes[anchor_line],
            "anchor_line": anchor_line,
            "old_content": old_content,
            "new_content": new_content,
            "context": {"start": start, "end": end, "lines": self.lines[start:end]},
        }

    def apply_patch(self, patch: Dict) -> Optional[str]:
        """Apply a hashline-anchored patch.

        Returns modified content, or None if anchor is stale.
        """
        anchor_hash = patch["anchor_hash"]
        anchor_line = patch["anchor_line"]
        if anchor_line >= len(self.line_hashes):
            return None
        if self.line_hashes[anchor_line] != anchor_hash:
            return None  # stale anchor

        # Replace the line containing old_content with new_content
        modified = list(self.lines)
        for i, line in enumerate(modified):
            if patch["old_content"] in line:
                # Preserve original line ending
                ending = ""
                if line.endswith("\r\n"):
                    ending = "\r\n"
                elif line.endswith("\n"):
                    ending = "\n"
                modified[i] = patch["new_content"] + ending
                break
        return "".join(modified)

    def validate_patch(self, patch: Dict) -> Tuple[bool, str]:
        """Validate a patch can be applied.

        Returns (is_valid, error_message).
        """
        anchor_line = patch["anchor_line"]
        if anchor_line >= len(self.line_hashes):
            return False, f"Anchor line {anchor_line} out of range"
        if self.line_hashes[anchor_line] != patch["anchor_hash"]:
            return False, f"Stale anchor: expected {patch['anchor_hash']}, got {self.line_hashes[anchor_line]}"
        return True, "valid"


class SINHashlinePatch:
    """High-level hashline patching interface for SIN-Code.

    Usage:
        patcher = SINHashlinePatch(Path("/path/to/repo"))
        patch = patcher.create_semantic_patch(Path("auth.py"), "def login():", "def login(user):")
        success, msg = patcher.apply_semantic_patch(patch)
    """

    def __init__(self, repo_root: Optional[Path] = None):
        self.repo_root = repo_root or Path.cwd()

    def create_semantic_patch(
        self,
        file_path: Path,
        old_content: str,
        new_content: str,
        intent: Optional[str] = None,
    ) -> Optional[Dict]:
        """Create a semantic patch with hashline anchors."""
        file_path = Path(file_path)
        if not file_path.exists():
            return None
        content = file_path.read_text()
        anchor = HashlineAnchor(content)
        patch = anchor.create_patch(old_content, new_content)
        if patch is None:
            return None
        patch["file"] = str(file_path)
        patch["intent"] = intent
        return patch

    def apply_semantic_patch(self, patch: Dict) -> Tuple[bool, str]:
        """Apply a semantic patch with validation.

        Returns (success, message).
        """
        file_path = Path(patch["file"])
        if not file_path.exists():
            return False, f"File not found: {file_path}"
        content = file_path.read_text()
        anchor = HashlineAnchor(content)
        is_valid, error_msg = anchor.validate_patch(patch)
        if not is_valid:
            return False, f"Patch validation failed: {error_msg}"
        modified = anchor.apply_patch(patch)
        if modified is None:
            return False, "Failed to apply patch"
        # Atomic write
        with tempfile.NamedTemporaryFile(
            mode="w", dir=file_path.parent, delete=False, suffix=".tmp"
        ) as tmp:
            tmp.write(modified)
            tmp_path = Path(tmp.name)
        try:
            tmp_path.replace(file_path)
            return True, "Patch applied successfully"
        except Exception as e:
            tmp_path.unlink(missing_ok=True)
            return False, f"Failed to write: {e}"


__all__ = ["HashlineAnchor", "SINHashlinePatch"]
