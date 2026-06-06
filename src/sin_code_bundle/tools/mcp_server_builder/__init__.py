"""
Purpose: SIN-MCP-Server-Builder Skill — meta-skill that scaffolds new MCP servers.
Docs: __init__.doc.md
"""

from .auditor import Auditor
from .publisher import Publisher
from .registrar import Registrar
from .scaffolder import Scaffolder, ScaffoldSpec
from .templates import TemplateEngine, list_templates
from .test_gen import TestGenerator
from .tool_adder import ToolAdder, ToolSpec
from .validator import ValidationResult, Validator

__all__ = [
    "Scaffolder",
    "ScaffoldSpec",
    "TemplateEngine",
    "list_templates",
    "ToolAdder",
    "ToolSpec",
    "TestGenerator",
    "Registrar",
    "Validator",
    "ValidationResult",
    "Publisher",
    "Auditor",
]
__version__ = "0.1.0"
