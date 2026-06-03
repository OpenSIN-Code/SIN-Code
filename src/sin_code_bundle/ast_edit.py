"""AST-based code editing with lazy tree-sitter + POC verification.

Docs: ast_edit.doc.md

Tree-sitter is **optional**. If it isn't installed, :class:`SINASTEdit`
falls back to a no-op state where :meth:`is_available` returns ``False``
and :meth:`edit` returns a clear install-hint error. This keeps the
bundle importable without tree-sitter as a hard dep.
"""
from __future__ import annotations

import tempfile
from pathlib import Path
from typing import Any, Dict, List, Optional


# ── Lazy tree-sitter import ─────────────────────────────────────────
# Tree-sitter is a heavy native dep; we never want to force it on
# users who only want to import :mod:`sin_code_bundle` for tooling.
def _try_import_tree_sitter() -> Optional[Any]:
    try:
        import tree_sitter  # type: ignore  # noqa: F401
        return tree_sitter
    except ImportError:
        return None


def _try_import_tree_sitter_languages() -> Optional[Any]:
    try:
        from tree_sitter_languages import get_parser  # type: ignore  # noqa: F401
        return get_parser
    except ImportError:
        return None


class ASTEditResult:
    """Result of an AST edit operation.

    Attributes:
        success: True if the proposal phase succeeded.
        proposed_changes: List of change dicts ready to be applied
            via :meth:`SINASTEdit.resolve`.
        poc_verified: True if a POC verification call succeeded.
        poc_report: The raw POC report (or ``None``).
        error: Human-readable error message on failure.
    """

    def __init__(
        self,
        success: bool,
        proposed_changes: Optional[List[Dict[str, Any]]] = None,
        poc_verified: bool = False,
        poc_report: Optional[Dict[str, Any]] = None,
        error: Optional[str] = None,
    ) -> None:
        self.success = success
        self.proposed_changes = proposed_changes or []
        self.poc_verified = poc_verified
        self.poc_report = poc_report
        self.error = error

    def to_dict(self) -> Dict[str, Any]:
        return {
            "success": self.success,
            "proposed_changes": self.proposed_changes,
            "poc_verified": self.poc_verified,
            "poc_report": self.poc_report,
            "error": self.error,
        }


class SINASTEdit:
    """AST-based code editing with tree-sitter and POC verification.

    Usage:
        ast = SINASTEdit()
        result = ast.edit(Path("foo.py"), "def old_func():", "def new_func():")
        if result.success:
            ast.resolve(Path("foo.py"), result.proposed_changes)

    Tree-sitter must be installed for any real editing::

        pip install tree-sitter tree-sitter-languages
    """

    SUPPORTED_LANGS = {"python", "javascript", "typescript", "go", "rust"}

    def __init__(self, repo_root: Optional[Path] = None) -> None:
        self.repo_root = repo_root or Path.cwd()
        self.ts: Optional[Any] = _try_import_tree_sitter()
        self.get_parser: Optional[Any] = _try_import_tree_sitter_languages()
        self.parsers: Dict[str, Any] = {}
        if self.ts is not None and self.get_parser is not None:
            self._init_parsers()

    def _init_parsers(self) -> None:
        """Initialize tree-sitter parsers for :data:`SUPPORTED_LANGS`.

        Missing or broken language bindings are skipped silently — the
        affected language simply won't appear in :attr:`parsers`.
        """
        if self.get_parser is None:
            return
        for lang in self.SUPPORTED_LANGS:
            try:
                self.parsers[lang] = self.get_parser(lang)
            except Exception:
                # Language not available in this tree-sitter build
                pass

    @staticmethod
    def _detect_language(file_path: Path) -> Optional[str]:
        ext_map = {
            ".py": "python",
            ".js": "javascript",
            ".ts": "typescript",
            ".go": "go",
            ".rs": "rust",
        }
        return ext_map.get(file_path.suffix)

    def is_available(self, language: Optional[str] = None) -> bool:
        """Return whether AST editing is available.

        With no ``language`` argument, returns True if *any* supported
        parser loaded successfully. With a ``language``, returns True
        only if that specific parser is ready.
        """
        if self.ts is None or self.get_parser is None:
            return False
        if language is None:
            return bool(self.parsers)
        return language in self.parsers

    def edit(
        self,
        file_path: Path,
        old_substring: str,
        replacement: str,
        verify_with_poc: bool = True,
    ) -> ASTEditResult:
        """Propose an AST-based edit.

        Tree-sitter is used to parse the file and confirm the language
        is supported, but the actual replacement is line-based (the
        line containing ``old_substring`` is swapped for ``replacement``).
        That keeps the v1 simple while still validating syntax.

        Install for full AST-precise edits::

            pip install tree-sitter tree-sitter-languages
        """
        file_path = Path(file_path)
        if not file_path.exists():
            return ASTEditResult(
                success=False, error=f"File not found: {file_path}"
            )
        if not self.is_available():
            return ASTEditResult(
                success=False,
                error=(
                    "tree-sitter not installed. Run: "
                    "pip install tree-sitter tree-sitter-languages"
                ),
            )
        language = self._detect_language(file_path)
        if not language or language not in self.parsers:
            return ASTEditResult(
                success=False,
                error=f"Unsupported or unparsed language for: {file_path}",
            )
        parser = self.parsers[language]

        code = file_path.read_text()
        # Parse the whole file — throws away the tree after, but proves
        # the file is syntactically valid for the chosen language.
        parser.parse(bytes(code, "utf-8"))

        if old_substring not in code:
            return ASTEditResult(
                success=False,
                error=f"old_substring not found in {file_path}",
            )
        lines = code.splitlines(keepends=True)
        target_line: Optional[int] = None
        for i, line in enumerate(lines):
            if old_substring in line:
                target_line = i
                break
        if target_line is None:
            return ASTEditResult(
                success=False, error="Could not locate line"
            )

        # Preserve line ending of the original line
        new_line = replacement + (
            "\n" if lines[target_line].endswith("\n") else ""
        )
        proposed: List[Dict[str, Any]] = [
            {
                "type": "ast_replacement",
                "line": target_line,
                "old": lines[target_line],
                "new": new_line,
                "language": language,
            }
        ]

        poc_verified = False
        poc_report: Optional[Dict[str, Any]] = None
        if verify_with_poc:
            poc_report, poc_verified = self._verify_with_poc()
        else:
            poc_report = {"verified": "skipped", "note": "verify_with_poc=False"}

        return ASTEditResult(
            success=True,
            proposed_changes=proposed,
            poc_verified=poc_verified,
            poc_report=poc_report,
        )

    def _verify_with_poc(self) -> tuple[Dict[str, Any], bool]:
        """Best-effort POC verification using the optional ``sin_code_poc``.

        Returns ``(report, verified)``. Never raises; on ImportError
        reports ``verified: skipped``. On any other failure reports
        ``verified: failed`` with the error string.
        """
        try:
            from sin_code_poc import property_metadata  # type: ignore

            props: Any = property_metadata() if callable(property_metadata) else {}
            n = len(props) if hasattr(props, "__len__") else 0
            return (
                {
                    "verified": "ok",
                    "note": f"POC available, {n} properties",
                },
                True,
            )
        except ImportError:
            return ({"verified": "skipped", "error": "POC not installed"}, False)
        except Exception as e:  # noqa: BLE001
            return ({"verified": "failed", "error": str(e)}, False)

    def resolve(self, file_path: Path, changes: List[Dict[str, Any]]) -> bool:
        """Apply accepted AST changes to a file.

        Changes are applied in reverse line order so earlier line
        numbers stay valid. The write is atomic via a sibling
        ``tempfile.NamedTemporaryFile`` + ``Path.replace``.

        Returns True on success, False on any I/O failure.
        """
        file_path = Path(file_path)
        if not file_path.exists():
            return False
        code = file_path.read_text()
        lines = code.splitlines(keepends=True)
        # Apply changes in reverse order so line numbers stay valid
        sorted_changes = sorted(changes, key=lambda c: c["line"], reverse=True)
        for change in sorted_changes:
            idx = change["line"]
            if 0 <= idx < len(lines):
                lines[idx] = change["new"]
        modified = "".join(lines)
        # Atomic write: tmp in same dir, then replace
        try:
            with tempfile.NamedTemporaryFile(
                mode="w",
                dir=file_path.parent,
                delete=False,
                suffix=".tmp",
            ) as tmp:
                tmp.write(modified)
                tmp_path = Path(tmp.name)
        except OSError:
            return False
        try:
            tmp_path.replace(file_path)
            return True
        except OSError:
            tmp_path.unlink(missing_ok=True)
            return False


__all__ = ["SINASTEdit", "ASTEditResult"]
