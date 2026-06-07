# SPDX-License-Identifier: MIT
"""DAG-Kanban Parser & Task Runner for Pocock Workflow.

Analyzes the PRD.md to build a directed acyclic graph (DAG) of tasks/slices,
identifies dependencies, and determines the optimal execution order using
topological sorting (Kahn's algorithm).

Docs: dag_kanban.doc.md
"""

from __future__ import annotations

import os
import re
import sys
import json
import argparse
from pathlib import Path
from typing import Optional
from collections import defaultdict, deque
from dataclasses import dataclass, field, asdict


@dataclass
class TaskNode:
    """A single task in the DAG."""
    id: str
    label: str
    description: str
    dependencies: list[str] = field(default_factory=list)
    status: str = "pending"
    executor: Optional[str] = None
    container: Optional[str] = None


class DAGKanban:
    """DAG-based Kanban board that parses PRDs and coordinates task execution."""

    def __init__(self, prd_path: str = "PRD.md"):
        self.prd_path = prd_path
        self.tasks: dict[str, TaskNode] = {}
        self.adj_list: defaultdict[str, list[str]] = defaultdict(list)
        self.in_degree: defaultdict[str, int] = defaultdict(int)

    def parse_prd(self) -> bool:
        """Parse PRD.md and extract task slices.
        
        Returns:
            True if tasks were found, False otherwise.
        """
        if not os.path.exists(self.prd_path):
            print(f"❌ Fehler: '{self.prd_path}' wurde nicht gefunden!")
            return False

        with open(self.prd_path, "r", encoding="utf-8") as f:
            content = f.read()

        # Extract task slices from PRD
        # Matches patterns like:
        # - [ ] Slice 1: Description
        # - [ ] Task 1: Description
        # - [ ] Step 1: Description
        patterns = [
            r"- \[\s*\]\s*(Slice\s*\d+):\s*(.*)",
            r"- \[\s*\]\s*(Task\s*\d+):\s*(.*)",
            r"- \[\s*\]\s*(Step\s*\d+):\s*(.*)",
            r"- \[\s*\]\s*(\d+\.\s+[^:]+):\s*(.*)",
        ]

        matches = []
        for pattern in patterns:
            matches = re.findall(pattern, content)
            if matches:
                break

        # Also look for "Tracer Bullets" section specifically
        if not matches:
            # Look for any list items under "Technische Spezifikation" or "Architekturschnitte"
            section_match = re.search(
                r"##\s*Technische Spezifikation.*?(?=##|$)",
                content,
                re.DOTALL | re.IGNORECASE
            )
            if section_match:
                section_content = section_match.group(0)
                matches = re.findall(r"- \[\s*\]\s*(.+)", section_content)
                if matches:
                    matches = [(f"Task {i+1}", m) for i, m in enumerate(matches)]

        if not matches:
            print(f"⚠️  Keine verwertbaren Arbeitsschritte in {self.prd_path} gefunden.")
            return False

        for idx, (task_label, desc) in enumerate(matches):
            task_id = task_label.strip().lower().replace(" ", "_")
            self.tasks[task_id] = TaskNode(
                id=task_id,
                label=task_label.strip(),
                description=desc.strip(),
                dependencies=[]
            )

        # Build automatic sequential dependencies: Slice N depends on Slice N-1
        self._build_sequential_dependencies()
        self._build_graph()
        return True

    def _build_sequential_dependencies(self) -> None:
        """Build automatic sequential dependencies between tasks."""
        task_ids = list(self.tasks.keys())
        for idx in range(1, len(task_ids)):
            prev_id = task_ids[idx - 1]
            current_id = task_ids[idx]
            self.tasks[current_id].dependencies.append(prev_id)

    def _build_graph(self) -> None:
        """Build adjacency list and in-degree map."""
        for t_id, task in self.tasks.items():
            for dep in task.dependencies:
                self.adj_list[dep].append(t_id)
                self.in_degree[t_id] += 1
            if t_id not in self.in_degree:
                self.in_degree[t_id] = 0

    def get_execution_order(self) -> list[str]:
        """Get topological sort using Kahn's algorithm.
        
        Returns:
            List of task IDs in execution order.
            
        Raises:
            ValueError: If circular dependencies are detected.
        """
        queue = deque([node for node in self.tasks if self.in_degree[node] == 0])
        order = []

        while queue:
            curr = queue.popleft()
            order.append(curr)
            for neighbor in self.adj_list[curr]:
                self.in_degree[neighbor] -= 1
                if self.in_degree[neighbor] == 0:
                    queue.append(neighbor)

        if len(order) != len(self.tasks):
            raise ValueError("❌ Zirkuläre Abhängigkeiten im DAG detektiert!")

        return order

    def get_parallel_groups(self) -> list[list[str]]:
        """Get tasks grouped by parallel execution groups.
        
        Returns:
            List of groups where tasks within each group can run in parallel.
        """
        in_degree_copy = defaultdict(int)
        for t_id in self.tasks:
            in_degree_copy[t_id] = self.in_degree.get(t_id, 0)

        for t_id, task in self.tasks.items():
            for dep in task.dependencies:
                in_degree_copy[t_id] += 1

        groups = []
        while len(sum(groups, [])) < len(self.tasks):
            # Find all tasks with in-degree 0
            current_group = [
                t_id for t_id in self.tasks
                if in_degree_copy[t_id] == 0 and t_id not in sum(groups, [])
            ]
            if not current_group:
                break
            groups.append(current_group)
            # Remove these tasks from the graph
            for t_id in current_group:
                for neighbor in self.adj_list[t_id]:
                    in_degree_copy[neighbor] -= 1

        return groups

    def assign_executors(self, executor_pattern: str = "agent-{}") -> None:
        """Assign executors to tasks for parallel execution.
        
        Args:
            executor_pattern: Pattern for executor naming (e.g., "agent-{}")
        """
        groups = self.get_parallel_groups()
        for group_idx, group in enumerate(groups):
            for task_idx, task_id in enumerate(group):
                self.tasks[task_id].executor = executor_pattern.format(group_idx + 1)
                self.tasks[task_id].container = f"container-{task_id}"

    def run(self) -> list[str]:
        """Execute the DAG analysis and display results.
        
        Returns:
            List of task IDs in execution order.
        """
        print("🔍 Analysiere PRD-Spezifikation für die Task-Pipeline...")
        if not self.parse_prd():
            print("⚠️  Keine verwertbaren Arbeitsschritte gefunden.")
            return []

        try:
            order = self.get_execution_order()
            self.assign_executors()
            self._display_results(order)
            return order
        except ValueError as e:
            print(str(e))
            return []

    def _display_results(self, order: list[str]) -> None:
        """Display the DAG analysis results."""
        parallel_groups = self.get_parallel_groups()

        print("\n" + "=" * 70)
        print("          🗂️  OPENSIN DAG-KANBAN-BOARD")
        print("=" * 70)

        # Display execution order
        print("\n📋 Ausführungsreihenfolge (Topologisch sortiert):")
        for step, t_id in enumerate(order, 1):
            task = self.tasks[t_id]
            deps = ", ".join(task.dependencies).upper() if task.dependencies else "KEINE"
            print(f"\n   [{step:02d}] {task.label.upper()}")
            print(f"        Beschreibung: {task.description}")
            print(f"        Abhängigkeiten: {deps}")
            print(f"        Executor: {task.executor}")
            print(f"        Container: {task.container}")

        # Display parallel groups
        print("\n" + "─" * 70)
        print("⚡ Parallelisierbare Gruppen:")
        for group_idx, group in enumerate(parallel_groups, 1):
            tasks = [self.tasks[t_id].label for t_id in group]
            print(f"   Gruppe {group_idx}: {', '.join(tasks)}")

        print("\n" + "=" * 70)
        print(f"✅ Total: {len(order)} Tasks, {len(parallel_groups)} parallele Gruppen")
        print("=" * 70 + "\n")

    def to_json(self) -> str:
        """Export DAG to JSON."""
        return json.dumps({
            "tasks": {t_id: asdict(t) for t_id, t in self.tasks.items()},
            "execution_order": self.get_execution_order() if self.tasks else [],
            "parallel_groups": [[t_id for t_id in group] for group in self.get_parallel_groups()] if self.tasks else [],
        }, indent=2, ensure_ascii=False)

    def export_docker_compose(self, output_path: str = "docker-compose.dag.yml") -> str:
        """Generate Docker Compose file for parallel execution.
        
        Returns:
            Path to generated docker-compose file.
        """
        services = {}
        for t_id, task in self.tasks.items():
            services[task.container] = {
                "image": "opensin/agent:latest",
                "container_name": task.container,
                "environment": {
                    "TASK_ID": t_id,
                    "TASK_LABEL": task.label,
                    "TASK_DESCRIPTION": task.description,
                    "TASK_DEPENDENCIES": ",".join(task.dependencies),
                    "EXECUTOR": task.executor or "default",
                },
                "volumes": [
                    "./:/workspace",
                    "./.dag-locks:/locks",
                ],
                "depends_on": {
                    self.tasks[dep].container: {"condition": "service_completed_successfully"}
                    for dep in task.dependencies if dep in self.tasks
                } if task.dependencies else {},
            }

        compose = {
            "version": "3.8",
            "services": services,
        }

        with open(output_path, "w", encoding="utf-8") as f:
            import yaml
            yaml.dump(compose, f, default_flow_style=False, sort_keys=False)

        print(f"🐳 Docker Compose generiert: {output_path}")
        return output_path


def run_dag_kanban(prd_path: str = "PRD.md", output_json: bool = False, export_docker: bool = False) -> list[str]:
    """Convenience function to run DAG Kanban.
    
    Args:
        prd_path: Path to PRD.md file
        output_json: Output JSON instead of human-readable
        export_docker: Export Docker Compose file
        
    Returns:
        List of task IDs in execution order
    """
    runner = DAGKanban(prd_path)
    order = runner.run()
    
    if output_json:
        print(runner.to_json())
    
    if export_docker:
        runner.export_docker_compose()
    
    return order


def main():
    """CLI entry point for DAG Kanban."""
    parser = argparse.ArgumentParser(
        description="DAG-Kanban Parser - Task Orchestration for Pocock Workflow",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  %(prog)s                          # Analyze PRD.md in current directory
  %(prog)s --prd docs/PRD.md         # Specify PRD path
  %(prog)s --json                    # Output JSON
  %(prog)s --docker                  # Export docker-compose.dag.yml
  %(prog)s --prd PRD.md --json --docker
        """
    )
    parser.add_argument("--prd", default="PRD.md", help="Pfad zur PRD.md")
    parser.add_argument("--json", action="store_true", help="JSON-Output")
    parser.add_argument("--docker", action="store_true", help="Docker Compose exportieren")
    parser.add_argument("--output", help="Output-Datei für Docker Compose")

    args = parser.parse_args()

    runner = DAGKanban(args.prd)
    order = runner.run()

    if args.json:
        print(runner.to_json())

    if args.docker:
        output_path = args.output or "docker-compose.dag.yml"
        runner.export_docker_compose(output_path)


if __name__ == "__main__":
    main()
