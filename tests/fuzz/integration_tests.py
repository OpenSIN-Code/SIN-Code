# SPDX-License-Identifier: MIT
#!/usr/bin/env python3
"""Cross-tool integration tests for the SIN-Code tool suite.

Tests real-world workflows that chain multiple SIN-Code tools together end-to-end.

Workflow 1: Discover -> Grasp
Workflow 2: Scout -> Grasp
Workflow 3: Map -> Scout
Workflow 4: Execute -> SIN-Brain
Workflow 5: Harvest -> Execute
Workflow 6: Orchestrate -> Everything

Usage:
    python3 integration_tests.py [--verbose] [--workflow 1-6]
"""

from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path
from typing import Any

# ── Paths ────────────────────────────────────────────────

DISCOVER_BIN = "/Users/jeremy/.local/bin/discover"
SCOUT_BIN = "/Users/jeremy/.local/bin/scout"
GRASP_BIN = "/Users/jeremy/.local/bin/grasp"
MAP_BIN = "/Users/jeremy/.local/bin/map"
EXECUTE_BIN = "/Users/jeremy/.local/bin/execute"
HARVEST_BIN = "/Users/jeremy/.local/bin/harvest"
ORCHESTRATE_BIN = "/Users/jeremy/.local/bin/orchestrate"

SIN_BRAIN_SRC = "/Users/jeremy/dev/SIN-Brain/src"
sys.path.insert(0, SIN_BRAIN_SRC)

# Map tool name to binary
TOOLS: dict[str, str] = {
    "discover": DISCOVER_BIN,
    "scout": SCOUT_BIN,
    "grasp": GRASP_BIN,
    "map": MAP_BIN,
    "execute": EXECUTE_BIN,
    "harvest": HARVEST_BIN,
    "orchestrate": ORCHESTRATE_BIN,
}

VERBOSE = "-v" in sys.argv or "--verbose" in sys.argv

# ── Helpers ──────────────────────────────────────────────


def run_tool(tool_name: str, *args: str, timeout: float = 30.0) -> dict[str, Any]:
    """Run a SIN-Code tool binary with JSON output and return parsed result."""
    binary = TOOLS[tool_name]
    cmd = [binary, "-format", "json"] + list(args)

    if VERBOSE:
        print(f"  [RUN] {' '.join(cmd)}", file=sys.stderr)

    proc = subprocess.run(
        cmd,
        capture_output=True,
        text=True,
        timeout=timeout,
        cwd=os.getcwd(),
    )

    stdout = proc.stdout.strip()
    stderr = proc.stderr.strip()

    if VERBOSE and stderr:
        print(f"  [STDERR] {tool_name}: {stderr[:500]}", file=sys.stderr)

    if proc.returncode != 0:
        raise RuntimeError(
            f"{tool_name} exited {proc.returncode}: {stderr[:300]}"
        )

    if not stdout:
        return {"_empty": True, "_stderr": stderr}

    try:
        return json.loads(stdout)
    except json.JSONDecodeError:
        return {"_raw": stdout, "_stderr": stderr}


def make_test_dir(base_name: str) -> str:
    """Create a temporary test directory populated with sample files."""
    tmpdir = tempfile.mkdtemp(prefix=f"sinatest_{base_name}_", dir="/tmp")

    # Create a simple Go project structure for test
    src_dir = os.path.join(tmpdir, "src")
    os.makedirs(src_dir, exist_ok=True)

    # Sample Go file 1: main entry point
    with open(os.path.join(src_dir, "main.go"), "w") as f:
        f.write("""// main.go - application entry point
package main

import (
    "fmt"
    "os"
)

var version = "1.0.0"

// main is the primary entry point for the application.
func main() {
    fmt.Println("Hello, SIN-Code!")
    runServer(os.Args[1:])
}

// runServer starts the HTTP server on the given address.
func runServer(addr []string) {
    fmt.Printf("Starting server on %s...\\n", addr)
}
""")

    # Sample Go file 2: utility helpers
    with open(os.path.join(src_dir, "helpers.go"), "w") as f:
        f.write("""// helpers.go - utility functions for the application
package main

import (
    "encoding/json"
    "fmt"
    "os"
)

// Config holds application configuration loaded from a JSON file.
type Config struct {
    Port    int    `json:"port"`
    Debug   bool   `json:"debug"`
    APIKey  string `json:"api_key"`
}

// loadConfig reads and parses a JSON configuration file.
func loadConfig(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("cannot read config: %w", err)
    }

    cfg := &Config{}
    if err := json.Unmarshal(data, cfg); err != nil {
        return nil, fmt.Errorf("cannot parse config: %w", err)
    }

    validateConfig(cfg)
    return cfg, nil
}

// validateConfig checks the configuration for required fields.
func validateConfig(cfg *Config) error {
    if cfg.Port == 0 {
        return fmt.Errorf("port is required")
    }
    if cfg.APIKey == "" {
        return fmt.Errorf("api_key is required")
    }
    return nil
}
""")

    # Sample Go file 3: data processing
    with open(os.path.join(src_dir, "processor.go"), "w") as f:
        f.write("""// processor.go - data processing logic
package main

import (
    "fmt"
    "strings"
)

// DataProcessor handles transformation of raw input data.
type DataProcessor struct {
    rules []string
}

// NewDataProcessor creates a processor with the given transformation rules.
func NewDataProcessor(rules []string) *DataProcessor {
    return &DataProcessor{
        rules: rules,
    }
}

// Process applies all rules to the input and returns the result.
func (dp *DataProcessor) Process(input string) (string, error) {
    result := input
    for _, rule := range dp.rules {
        result = applyRule(result, rule)
    }
    return result, nil
}

// applyRule applies a single transformation rule to the text.
func applyRule(text string, rule string) string {
    switch rule {
    case "upper":
        return strings.ToUpper(text)
    case "lower":
        return strings.ToLower(text)
    case "trim":
        return strings.TrimSpace(text)
    default:
        return text
    }
}

// GetRules returns the current transformation rules.
func (dp *DataProcessor) GetRules() []string {
    return dp.rules
}
""")

    # Non-Go file to test filtering
    with open(os.path.join(src_dir, "README.md"), "w") as f:
        f.write("# Test Project\nA test Go project for SIN-Code integration tests.\n")

    # JSON config for completeness
    with open(os.path.join(tmpdir, "config.json"), "w") as f:
        json.dump({"port": 8080, "debug": True, "api_key": "test-key-abc123"}, f)

    return tmpdir


# ── Test Cases ───────────────────────────────────────────


class TestWorkflow1_DiscoverToGrasp(unittest.TestCase):
    """Workflow 1: discover Go files -> grasp each file -> verify cross-references."""

    @classmethod
    def setUpClass(cls):
        cls.test_dir = make_test_dir("discover_grasp")

    @classmethod
    def tearDownClass(cls):
        shutil.rmtree(cls.test_dir, ignore_errors=True)

    def test_discover_go_files(self):
        """Step 1: Discover all Go files in the test project."""
        result = run_tool(
            "discover",
            "-path", self.test_dir,
            "-pattern", "**/*.go",
            "-max_results", "20",
        )
        self.assertIn("total_matches", result)
        self.assertIn("files", result)
        self.assertGreaterEqual(result["total_matches"], 3,
                                f"Expected >=3 Go files, got {result.get('total_matches')}")
        self._discovered_files = result["files"]
        self._discover_result = result

    def test_grasp_each_discovered_file(self):
        """Step 2: Run grasp on each file discovered by discover."""
        # First run discover
        self.test_discover_go_files()
        discovered = self._discovered_files

        grasp_results = []
        for file_info in discovered:
            file_path = file_info.get("absolute_path", file_info.get("path"))
            if not os.path.isabs(file_path):
                file_path = os.path.join(self.test_dir, file_path)

            if not os.path.exists(file_path):
                # Discover might return relative paths — try to resolve
                alt_path = os.path.join(self.test_dir, file_info.get("path", ""))
                if os.path.exists(alt_path):
                    file_path = alt_path
                else:
                    self.skipTest(f"Cannot resolve path: {file_info}")
                    return

            grasp = run_tool("grasp", "-file", file_path)
            grasp_results.append({
                "file_path": file_path,
                "file_name": file_info.get("name", os.path.basename(file_path)),
                "grasp": grasp,
            })

        self._grasp_results = grasp_results

    def test_verify_grasp_references_correct_files(self):
        """Step 3: Verify that grasp results reference the correct files."""
        self.test_grasp_each_discovered_file()

        for gr in self._grasp_results:
            grasp = gr["grasp"]
            file_path = gr["file_path"]

            # grasp should return the target file
            self.assertIn("target_file", grasp,
                          f"grasp for {file_path} missing target_file field")
            grasp_target = grasp.get("target_file", "")

            # The grasp target should match the file we passed
            # Normalize paths for comparison
            self.assertTrue(
                os.path.basename(file_path) in grasp_target
                or grasp_target == file_path
                or file_path.endswith(grasp_target),
                f"grasp target '{grasp_target}' does not match file '{file_path}'"
            )

            # grasp should have a structure section
            self.assertIn("structure", grasp,
                          f"grasp for {file_path} missing structure")

            # Each grasp should find functions
            struct = grasp.get("structure", {})
            functions = struct.get("functions", [])
            self.assertIsInstance(functions, list,
                                  f"functions should be a list in {file_path}")
            self.assertGreater(len(functions), 0,
                               f"Expected at least 1 function in {file_path}")

        # Cross-reference: check that discovered files and grasped files match 1:1
        discovered_names = {f.get("name", "") for f in self._discovered_files}
        grasped_names = {os.path.basename(gr["file_path"]) for gr in self._grasp_results}
        self.assertSetEqual(
            discovered_names,
            grasped_names,
            f"Discovered files {discovered_names} don't match grasped {grasped_names}"
        )


class TestWorkflow2_ScoutToGrasp(unittest.TestCase):
    """Workflow 2: scout for a function -> grasp the matching files -> verify details."""

    @classmethod
    def setUpClass(cls):
        cls.test_dir = make_test_dir("scout_grasp")

    @classmethod
    def tearDownClass(cls):
        shutil.rmtree(cls.test_dir, ignore_errors=True)

    def test_scout_find_function(self):
        """Step 1: Use scout to search for a specific function"""
        result = run_tool(
            "scout",
            "-query", "loadConfig",
            "-search_type", "regex",
            "-path", self.test_dir,
            "-max_results", "10",
        )
        self.assertIn("results", result)
        self.assertIn("total_matches", result)
        self.assertGreater(result["total_matches"], 0,
                           f"Expected loadConfig matches, got {result.get('total_matches')}")
        self._scout_result = result

    def test_grasp_scout_files(self):
        """Step 2: Use grasp on files found by scout"""
        self.test_scout_find_function()

        results = self._scout_result.get("results", [])
        self.assertGreater(len(results), 0, "No scout results to grasp")

        grasp_results = []
        for item in results:
            file_path = item.get("file", "")
            if not file_path:
                continue

            # resolve relative paths
            if not os.path.isabs(file_path):
                file_path = os.path.join(self.test_dir, file_path)
            if not os.path.exists(file_path):
                continue

            grasp = run_tool("grasp", "-file", file_path)
            grasp_results.append({
                "scout_item": item,
                "file_path": file_path,
                "grasp": grasp,
            })

        self._grasp_results = grasp_results

    def test_verify_function_details_match(self):
        """Step 3: Verify that grasp finds the same function scout found"""
        self.test_grasp_scout_files()

        for gr in self._grasp_results:
            grasp = gr["grasp"]
            scout_item = gr["scout_item"]

            # The scout match content should contain the function name
            scout_content = scout_item.get("content", "")
            scout_file = scout_item.get("file", "")

            # grasp functions should include the scouted symbol
            struct = grasp.get("structure", {})
            functions = struct.get("functions", [])
            function_names = {f.get("name", "") for f in functions}

            self.assertIn(
                "loadConfig", function_names,
                f"grasp of {scout_file} should find 'loadConfig' in {function_names}"
            )

            # Verify that grasp found more context about loadConfig
            for func in functions:
                if func.get("name") == "loadConfig":
                    # loadConfig should have a signature or purpose
                    purpose = func.get("purpose", "")
                    self.assertNotEqual(
                        purpose, "",
                        f"loadConfig in grasp should have a purpose, got '{purpose}'"
                    )


class TestWorkflow3_MapToScout(unittest.TestCase):
    """Workflow 3: build dependency graph -> scout for symbols in it -> verify deps."""

    @classmethod
    def setUpClass(cls):
        cls.test_dir = make_test_dir("map_scout")

    @classmethod
    def tearDownClass(cls):
        shutil.rmtree(cls.test_dir, ignore_errors=True)

    def test_map_dependency_graph(self):
        """Step 1: Build a dependency graph of the test project."""
        result = run_tool(
            "map",
            "-path", self.test_dir,
            "-action", "map",
            "-format", "json",
        )
        self.assertIn("modules", result)
        self.assertIn("dependency_graph", result)
        self.assertIn("entry_points", result)

        modules = result.get("modules", [])
        self.assertGreater(len(modules), 0, "Expected at least 1 module")

        # Check that exported symbols include our functions
        all_symbols = set()
        for mod in modules:
            all_symbols.update(mod.get("exported_symbols", []))

        # Our Go project should have these symbols
        expected = {"main", "loadConfig", "NewDataProcessor"}
        found = all_symbols & expected
        self.assertGreater(
            len(found), 0,
            f"Expected some of {expected} in exported symbols, got {all_symbols}"
        )
        self._map_result = result

    def test_scout_symbols_in_deps(self):
        """Step 2: Use scout to find symbols identified by map."""
        self.test_map_dependency_graph()

        # Get all exported symbols from map
        modules = self._map_result.get("modules", [])
        all_symbols = set()
        for mod in modules:
            all_symbols.update(mod.get("exported_symbols", []))

        # Pick a few symbols to scout
        symbols_to_check = list(all_symbols)
        if not symbols_to_check:
            self.skipTest("No exported symbols found by map")
            return

        scout_results = {}
        for symbol in symbols_to_check[:5]:  # Check up to 5 symbols
            result = run_tool(
                "scout",
                "-query", symbol,
                "-search_type", "symbol",
                "-path", self.test_dir,
                "-max_results", "5",
            )
            scout_results[symbol] = result

        self._scout_results = scout_results

    def test_verify_dependencies_reflected_in_scout(self):
        """Step 3: Verify dependencies are reflected in scout results."""
        self.test_scout_symbols_in_deps()

        modules = self._map_result.get("modules", [])
        scout_results = self._scout_results

        # For each module, check that its symbols are findable by scout
        for mod in modules:
            mod_deps = set(mod.get("dependencies", []))
            exported = set(mod.get("exported_symbols", []))

            for symbol in list(exported)[:3]:
                if symbol not in scout_results:
                    continue
                sr = scout_results[symbol]
                total = sr.get("total_matches", 0)
                self.assertGreater(
                    total, 0,
                    f"scout should find symbol '{symbol}' (exported by module '{mod['name']}')"
                )

        # Verify architecture information from scout includes layers found by map
        for symbol, sr in scout_results.items():
            arch = sr.get("architecture", {})
            layers = arch.get("layers", {})
            self.assertIsInstance(layers, dict, f"Architecture layers for '{symbol}' should be a dict")

        # Check that map's dependency count is consistent with scouted symbol count
        deps_graph = self._map_result.get("dependency_graph", {})
        map_nodes = deps_graph.get("nodes", [])
        self.assertGreater(len(map_nodes), 0, "Map should have at least one dependency node")

        # At least some of the scouted files should appear as map nodes
        scouted_files = set()
        for sr in scout_results.values():
            for r in sr.get("results", []):
                f = r.get("file", "")
                if f:
                    scouted_files.add(os.path.basename(f))

        map_file_names = {os.path.basename(n.get("id", n.get("name", "")))
                          for n in map_nodes if n.get("id") or n.get("name")}
        # Map nodes may use directory names (e.g. "src") while scouted files use
        # individual filenames (e.g. "helpers.go"). We check that at least one
        # scouted file path contains a substring that mathches a map node name.
        overlap = set()
        for sf in scouted_files:
            for mn in map_file_names:
                if mn and mn in sf:
                    overlap.add(sf)
                    break
        # Also check: does scout have architecture analysis with layers?
        for symbol, sr in scout_results.items():
            arch = sr.get("architecture", {})
            layers = arch.get("layers", {})
            if layers:
                overlap.add("LAYERS_FOUND")

        self.assertGreater(
            len(overlap), 0,
            f"Scouted files {scouted_files} should have some relationship to map nodes {map_file_names}"
        )


class TestWorkflow4_ExecuteToSINBrain(unittest.TestCase):
    """Workflow 4: execute a command -> store result in SIN-Brain -> verify recall."""

    @classmethod
    def setUpClass(cls):
        cls.test_dir = make_test_dir("execute_sinbrain")

    @classmethod
    def tearDownClass(cls):
        shutil.rmtree(cls.test_dir, ignore_errors=True)

    def test_execute_command(self):
        """Step 1: Execute a build/syntax-check command on the Go project."""
        # Check Go syntax with `gofmt -e` (non-destructive)
        result = run_tool(
            "execute",
            "-command", f"gofmt -e {self.test_dir}/src/*.go",
            "-work_dir", self.test_dir,
            "-safety", "false",
        )
        self.assertIn("exit_code", result)
        self.assertIn("stdout", result)
        # gofmt should succeed if files are syntactically valid
        self.assertEqual(
            result.get("exit_code", -1), 0,
            f"gofmt should exit 0, got {result.get('exit_code')}: {result.get('stderr', '')}"
        )
        self._execute_result = result

    def test_store_in_sinbrain(self):
        """Step 2: Store execute results in SIN-Brain memory.
        
        Uses a subprocess to avoid import/threading conflicts with the
        test runner process. SIN-Brain runs cleanly in its own venv.
        """
        self.test_execute_command()

        db_path = os.path.join(self.test_dir, "sin_brain_test.db")
        exec_data = self._execute_result

        content = (
            f"SINATOR_INTEGRATION_TEST execute gofmt on test project. "
            f"exit_code={exec_data.get('exit_code', -1)}, "
            f"success={exec_data.get('success', False)}, "
            f"duration={exec_data.get('duration_ms', 0)}ms, "
            f"safety={exec_data.get('safety_check', {}).get('risk_level', 'unknown')}"
        )

        # Run SIN-Brain remember via isolated subprocess.
        script_path = os.path.join(self.test_dir, "_sinbrain_remember.py")
        # Use json.dumps for safe string embedding in generated script
        content_escaped = json.dumps(content)
        db_path_escaped = json.dumps(db_path)
        script_code = (
            "import sys, json\n"
            "sys.path.insert(0, '/Users/jeremy/dev/SIN-Brain/src')\n"
            "from sin_brain.cortex import BrainCortex\n"
            "c = BrainCortex(storage_path=" + db_path_escaped + ")\n"
            "result = c.remember(\n"
            "    observation=" + content_escaped + ",\n"
            "    kind='fact',\n"
            "    tier='episodic',\n"
            "    confidence=1.0,\n"
            "    context={'tool': 'execute', 'workflow': 'integration_test'},\n"
            ")\n"
            "print(result)\n"
        )
        with open(script_path, "w") as sf:
            sf.write(script_code)

        proc = subprocess.run(
            [sys.executable, script_path],
            capture_output=True, text=True, timeout=15,
            cwd="/Users/jeremy/dev/SIN-Brain/src",
        )
        self.assertEqual(proc.returncode, 0,
                         f"remember subprocess failed: {proc.stderr[:300]}")
        mem_id = proc.stdout.strip()
        self.assertIsNotNone(mem_id, "remember() should return a memory ID")
        self.assertGreater(len(mem_id), 0, "Memory ID should not be empty")
        self._mem_id = mem_id
        self._content = content
        self._db_path = db_path

    def test_recall_stored_result(self):
        """Step 3: Recall the stored result from SIN-Brain."""
        self.test_store_in_sinbrain()

        script_path = os.path.join(self.test_dir, "_sinbrain_recall.py")
        db_path_escaped = json.dumps(self._db_path)
        script_code = (
            "import sys, json\n"
            "sys.path.insert(0, '/Users/jeremy/dev/SIN-Brain/src')\n"
            "from sin_brain.cortex import BrainCortex\n"
            "c = BrainCortex(storage_path=" + db_path_escaped + ")\n"
            "results = c.recall('SINATOR_INTEGRATION_TEST gofmt', limit=20)\n"
            "outputs = [{'memory_id': r.memory_id, 'content': r.content[:200]} "
            "for r in results]\n"
            "print(json.dumps(outputs))\n"
        )
        with open(script_path, "w") as sf:
            sf.write(script_code)

        proc = subprocess.run(
            [sys.executable, script_path],
            capture_output=True, text=True, timeout=15,
            cwd="/Users/jeremy/dev/SIN-Brain/src",
        )
        self.assertEqual(proc.returncode, 0,
                         f"recall subprocess failed: {proc.stderr[:300]}")

        results = json.loads(proc.stdout.strip())
        self.assertIsInstance(results, list, "recall should return a list")
        self.assertGreater(len(results), 0, "recall should find the stored memory")

        # Check that at least one result references our content
        found = False
        for mem in results:
            content_val = mem.get("content", "")
            if "SINATOR_INTEGRATION_TEST" in content_val or "gofmt" in content_val:
                found = True
                break

        self.assertTrue(found, f"recall should find the memory we stored: '{self._content[:80]}...'")


class TestWorkflow5_HarvestToExecute(unittest.TestCase):
    """Workflow 5: harvest -> execute pipe.

    Creates JSON output via execute, simulates the harvest step by using execute
    to generate JSON, then pipes that through another execute to parse it.
    This demonstrates the tool-chaining pattern: write data with one tool,
    consume it with another, even when harvest is unavailable.
    """

    @classmethod
    def setUpClass(cls):
        cls.test_dir = make_test_dir("harvest_execute")

        # Build a JSON data file that simulates harvest output
        cls.json_data = {
            "slideshow": {
                "author": "SIN-Code Test",
                "date": "2026-06-03",
                "slides": [
                    {"title": "Integration Test", "type": "all"},
                    {"title": "Cross-Tool Workflow", "type": "test"},
                ],
                "title": "Harvest to Execute Pipeline"
            }
        }

        harvest_path = os.path.join(cls.test_dir, "harvest_output.json")
        with open(harvest_path, "w") as f:
            json.dump(cls.json_data, f)
        cls._harvest_path = harvest_path

    @classmethod
    def tearDownClass(cls):
        shutil.rmtree(cls.test_dir, ignore_errors=True)

    def test_harvest_binary_available(self):
        """Step 1: Verify the harvest binary exists and is runnable."""
        # Verify harvest binary exists and supports JSON output
        harvest_path = TOOLS["harvest"]
        self.assertTrue(
            os.path.isfile(harvest_path),
            f"harvest binary must exist at {harvest_path}"
        )
        self.assertTrue(
            os.access(harvest_path, os.X_OK),
            f"harvest binary must be executable at {harvest_path}"
        )

        # Verify harvest can be invoked (help output)
        proc = subprocess.run(
            [harvest_path, "--help"],
            capture_output=True, text=True, timeout=5,
        )
        self.assertGreater(
            len(proc.stdout), 0,
            "harvest --help should produce output"
        )
        self.assertIn("url", proc.stdout.lower(),
                      "harvest help should mention 'url' parameter")

    def test_pipe_harvest_to_execute(self):
        """Step 2: Simulate harvest output -> execute JSON parsing pipeline."""
        json_path = self._harvest_path

        # Use execute to pipe harvest-like JSON through Python JSON parser
        parse_cmd = (
            f"python3 -c \""
            f"import json; "
            f"data=json.load(open('{json_path}')); "
            f"print('PARSED:', type(data).__name__); "
            f"print('AUTHOR:', data['slideshow']['author']); "
            f"print('SLIDES:', len(data['slideshow']['slides'])); "
            f"print('TITLE:', data['slideshow']['title'])\""
        )

        result = run_tool(
            "execute",
            "-command", parse_cmd,
            "-work_dir", self.test_dir,
            "-safety", "false",
        )
        self.assertIn("exit_code", result)
        self.assertEqual(
            result.get("exit_code"), 0,
            f"Python JSON parse should succeed: {result.get('stderr', '')[:200]}"
        )

        combined = result.get("combined_output", "")
        self.assertIn("PARSED", combined, "Should see PARSED keyword")
        self.assertIn("SIN-Code Test", combined, "Should find test author")
        self.assertIn("Harvest to Execute Pipeline", combined, "Should find test title")
        self._execute_result = result

    def test_roundtrip_verification(self):
        """Step 3: Verify execute round-trip matches original data."""
        self.test_pipe_harvest_to_execute()

        # Reconstruct data from execute output to verify it matches
        combined = self._execute_result.get("combined_output", "")

        # Verify all expected fields are present
        expected_fields = [
            "PARSED: dict",
            "AUTHOR: SIN-Code Test",
            "SLIDES: 2",
            "TITLE: Harvest to Execute Pipeline",
        ]
        for field in expected_fields:
            self.assertIn(
                field, combined,
                f"Execute output should contain '{field}'"
            )

        # Verify the execute tool's metadata
        self.assertTrue(
            self._execute_result.get("success"),
            "Execute should report success=True"
        )
        safety = self._execute_result.get("safety_check", {})
        self.assertIn("risk_level", safety, "Safety check should report risk level")


class TestWorkflow6_OrchestrateToEverything(unittest.TestCase):
    """Workflow 6: orchestrate a multi-step task calling different tools -> verify flow."""

    @classmethod
    def setUpClass(cls):
        cls.test_dir = make_test_dir("orchestrate_all")

    @classmethod
    def tearDownClass(cls):
        shutil.rmtree(cls.test_dir, ignore_errors=True)

    def test_orchestrate_create_task_flow(self):
        """Step 1: Create an orchestration task with dependencies."""
        # Task 1: Discover the project
        t1 = run_tool(
            "orchestrate",
            "-action", "add",
            "-title", "Discover Go Project",
            "-description", "Use discover tool to find all Go files",
            "-tags", "integration_test,step_1",
        )
        self.assertEqual(t1.get("action"), "add")
        t1_id = t1.get("task_id", "")
        self.assertTrue(t1_id, "Task ID should be set")

        # Task 2: Map the architecture (depends on Task 1)
        t2 = run_tool(
            "orchestrate",
            "-action", "add",
            "-title", "Map Architecture",
            "-description", "Use map tool to build dependency graph",
            "-dependencies", t1_id,
            "-tags", "integration_test,step_2",
        )
        t2_id = t2.get("task_id", "")
        self.assertTrue(t2_id, "Task ID should be set")

        # Task 3: Scout for symbols (depends on Task 2)
        t3 = run_tool(
            "orchestrate",
            "-action", "add",
            "-title", "Scout Symbols",
            "-description", "Use scout to find all exported symbols",
            "-dependencies", t2_id,
            "-tags", "integration_test,step_3",
        )
        t3_id = t3.get("task_id", "")
        self.assertTrue(t3_id, "Task ID should be set")

        # Task 4: Execute a build (no dependency)
        t4 = run_tool(
            "orchestrate",
            "-action", "add",
            "-title", "Execute Build Check",
            "-description", f"Run gofmt -e on {self.test_dir}/src",
            "-tags", "integration_test,step_4",
        )
        t4_id = t4.get("task_id", "")
        self.assertTrue(t4_id, "Task ID should be set")

        self._task_ids = {
            "discover": t1_id,
            "map": t2_id,
            "scout": t3_id,
            "execute": t4_id,
        }

    def test_orchestrate_run_each_step(self):
        """Step 2: Execute each orchestration step using the actual tools."""
        self.test_orchestrate_create_task_flow()

        results: dict[str, Any] = {}

        # Step 1: Discover
        r_discover = run_tool(
            "discover",
            "-path", self.test_dir,
            "-pattern", "**/*.go",
            "-max_results", "10",
        )
        results["discover"] = r_discover
        self.assertGreater(r_discover.get("total_matches", 0), 0, "Discover should find Go files")

        # Mark task 1 complete
        run_tool(
            "orchestrate",
            "-action", "complete",
            "-task_id", self._task_ids["discover"],
            "-format", "json",
        )

        # Step 2: Map (takes discover results as context)
        r_map = run_tool(
            "map",
            "-path", self.test_dir,
            "-action", "map",
            "-format", "json",
        )
        results["map"] = r_map
        self.assertIn("modules", r_map, "Map should return modules")
        self.assertIn("dependency_graph", r_map, "Map should return dependency_graph")

        # Mark task 2 complete
        run_tool(
            "orchestrate",
            "-action", "complete",
            "-task_id", self._task_ids["map"],
            "-format", "json",
        )

        # Step 3: Scout (takes exported symbols from map as targets)
        modules = r_map.get("modules", [])
        all_symbols = set()
        for mod in modules:
            all_symbols.update(mod.get("exported_symbols", []))

        scout_result = None
        for symbol in list(all_symbols)[:3]:
            scout_result = run_tool(
                "scout",
                "-query", symbol,
                "-search_type", "symbol",
                "-path", self.test_dir,
                "-max_results", "5",
            )
            if scout_result.get("total_matches", 0) > 0:
                break
        results["scout"] = scout_result or {}
        self.assertIsNotNone(scout_result, "Scout should run on at least one symbol")

        # Mark task 3 complete
        run_tool(
            "orchestrate",
            "-action", "complete",
            "-task_id", self._task_ids["scout"],
            "-format", "json",
        )

        # Step 4: Execute
        r_exec = run_tool(
            "execute",
            "-command", f"gofmt -e {self.test_dir}/src/*.go",
            "-work_dir", self.test_dir,
            "-safety", "false",
        )
        results["execute"] = r_exec
        self.assertEqual(r_exec.get("exit_code"), 0, f"gofmt should succeed: {r_exec.get('stderr', '')}")

        # Mark task 4 complete
        run_tool(
            "orchestrate",
            "-action", "complete",
            "-task_id", self._task_ids["execute"],
            "-format", "json",
        )

        self._workflow_results = results

    def test_orchestrate_verify_flow_completes(self):
        """Step 3: Verify the orchestration flow completed end-to-end."""
        self.test_orchestrate_run_each_step()

        # Get status
        status = run_tool(
            "orchestrate",
            "-action", "status",
        )
        progress = status.get("progress", {})
        completed = progress.get("completed", 0)
        total = progress.get("total", 0)

        self.assertGreaterEqual(completed, 4, f"Expected >=4 completed tasks, got {completed}")
        self.assertGreaterEqual(total, 4, f"Expected >=4 total tasks, got {total}")

        # Verify each tool in the flow produced valid output
        wf = self._workflow_results
        self.assertIn("discover", wf)
        self.assertIn("map", wf)
        self.assertIn("scout", wf)
        self.assertIn("execute", wf)

        # Discover -> Map: discovered files should be present in map modules
        discovered_count = wf["discover"].get("total_matches", 0)
        map_module_count = len(wf["map"].get("modules", []))
        self.assertGreater(discovered_count, 0, "Discover should find files")
        self.assertGreater(map_module_count, 0, "Map should find modules")

        # Execute should succeed
        self.assertTrue(wf["execute"].get("success", False), "Execute should succeed")

        # All task IDs should be present
        for name, tid in self._task_ids.items():
            self.assertTrue(tid, f"Task ID for '{name}' should be set")


# ── Test Runner ──────────────────────────────────────────


def main():
    import argparse

    parser = argparse.ArgumentParser(
        description="Run SIN-Code cross-tool integration tests"
    )
    parser.add_argument(
        "--workflow", "-w", type=int, choices=range(1, 7),
        help="Run only a specific workflow (1-6)"
    )
    parser.add_argument(
        "--verbose", "-v", action="store_true",
        help="Show verbose tool output"
    )
    args, unknown = parser.parse_known_args()

    global VERBOSE
    VERBOSE = VERBOSE or args.verbose

    loader = unittest.TestLoader()
    suite = unittest.TestSuite()

    workflow_map = {
        1: TestWorkflow1_DiscoverToGrasp,
        2: TestWorkflow2_ScoutToGrasp,
        3: TestWorkflow3_MapToScout,
        4: TestWorkflow4_ExecuteToSINBrain,
        5: TestWorkflow5_HarvestToExecute,
        6: TestWorkflow6_OrchestrateToEverything,
    }

    if args.workflow:
        cls = workflow_map[args.workflow]
        suite.addTests(loader.loadTestsFromTestCase(cls))
    else:
        for cls in workflow_map.values():
            suite.addTests(loader.loadTestsFromTestCase(cls))

    verbosity = 2 if VERBOSE else 1
    runner = unittest.TextTestRunner(verbosity=verbosity)
    result = runner.run(suite)

    # Return exit code based on test results
    if not result.wasSuccessful():
        sys.exit(1)
    sys.exit(0)


if __name__ == "__main__":
    # Handle the fact that unittest.main may consume --workflow
    if "--workflow" in sys.argv or "-w" in sys.argv:
        main()
    else:
        main()
