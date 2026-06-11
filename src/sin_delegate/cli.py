# SPDX-License-Identifier: MIT
"""CLI: standalone `sin-delegate` and `sin delegate` subcommand.

Subcommands:
    run <plan.json>          execute a plan
    plan <goal>              LLM-decompose a goal into a plan file
    auto <goal>              plan + execute end-to-end
    status <plan_id>         latest state per task
    history <plan_id>        full event log
    runs                     list known runs
    cancel <plan_id>         cooperative cancellation
    watch <plan_id>          live status board
    report <plan_id>         markdown report
    doctor                   preflight health check
    stats                    learned backend performance per task class
    escalations <plan_id>    list open decision requests
    resolve <plan_id> <esc>  answer an escalation
    resume <plan_id>         apply resolutions and continue
"""

from __future__ import annotations

import argparse
import asyncio
import json
import sys
from pathlib import Path

from .engine import Delegator
from .escalation import EscalationBroker
from .ledger import Ledger
from .models import TaskState
from .multirepo import multirepo_plan_from_dict
from .multirepo_engine import MultiRepoDelegator
from .planfile import load_plan

_ICON = {
    TaskState.DONE: "[ok]", TaskState.FAILED: "[FAIL]",
    TaskState.SKIPPED: "[skip]", TaskState.ESCALATED: "[ESCALATE]",
    TaskState.CANCELLED: "[cancel]",
}


def _cmd_run(args: argparse.Namespace) -> int:
    data = json.loads(Path(args.plan).read_text())
    if "repos" in data:
        mrp = multirepo_plan_from_dict(data)
        print(f"plan {mrp.id}: {len(mrp.plan.tasks)} tasks across "
              f"{len(mrp.repos)} repos ({', '.join(mrp.repos)})")
        dele = MultiRepoDelegator(mrp, max_parallel=args.parallel)
        result = dele.run_sync()
    else:
        plan = load_plan(args.plan, repo=args.repo)
        dele = Delegator(plan, max_parallel=args.parallel,
                         dry_run=args.dry_run,
                         keep_worktrees=args.keep_worktrees)
        print(f"plan {plan.id}: {len(plan.tasks)} tasks")
        result = dele.run_sync()
    if args.json:
        print(result.to_json())
    else:
        for tid, o in result.outcomes.items():
            icon = _ICON.get(o.state, f"[{o.state.value}]")
            extra = f" — {o.error}" if o.error else ""
            print(f"  {icon} {tid}{extra}")
        print("result:", "SUCCESS" if result.ok else "INCOMPLETE",
              f"(plan {result.plan_id})")
    return 0 if result.ok else 1


def _cmd_status(args: argparse.Namespace) -> int:
    states = Ledger().task_states(args.plan_id)
    if not states:
        print(f"no events for plan {args.plan_id}", file=sys.stderr)
        return 1
    print(json.dumps({k: v.value for k, v in states.items()}, indent=2))
    return 0


def _cmd_history(args: argparse.Namespace) -> int:
    for ev in Ledger().history(args.plan_id):
        print(f"{ev['seq']:5d}  {ev['task_id']:<18} {ev['kind']:<22} "
              f"{json.dumps(ev['payload'])[:80]}")
    return 0


def _cmd_runs(_: argparse.Namespace) -> int:
    for r in Ledger().list_runs():
        print(f"{r['plan_id']}  {r['goal'][:60]}")
    return 0


def _cmd_cancel(args: argparse.Namespace) -> int:
    Ledger().emit(args.plan_id, "*", "cancel:requested")
    print(f"cancel requested for plan {args.plan_id}")
    return 0


def _cmd_plan(args: argparse.Namespace) -> int:
    from .planner import plan_sync
    plan = plan_sync(args.goal, repo=args.repo,
                     backend=args.backend, model=args.model,
                     critique=not args.no_critique)
    payload = {
        "goal": plan.goal,
        "base_branch": plan.base_branch,
        "tasks": [{
            "key": t.id, "title": t.title,
            "instructions": t.instructions,
            "deps": list(t.deps),
            "files": list(t.files_hint),
            "risk": t.risk.value,
            "verify": list(t.verify),
        } for t in plan.tasks],
    }
    text = json.dumps(payload, indent=2, ensure_ascii=False)
    if args.out:
        Path(args.out).write_text(text, encoding="utf-8")
        print(f"plan {plan.id} written to {args.out} "
              f"({len(plan.tasks)} tasks)")
    else:
        print(text)
    return 0


def _cmd_auto(args: argparse.Namespace) -> int:
    from .planner import plan_sync
    from .observe import report
    plan = plan_sync(args.goal, repo=args.repo,
                     backend=args.backend, model=args.model)
    print(f"plan {plan.id}: {len(plan.tasks)} tasks")
    for t in plan.tasks:
        deps = f" <- {','.join(t.deps)}" if t.deps else ""
        print(f"  [{t.risk.value:>6}] {t.title}{deps}")
    if not args.yes:
        answer = input("execute? [y/N] ").strip().lower()
        if answer != "y":
            print("aborted")
            return 1
    dele = Delegator(plan, max_parallel=args.parallel,
                     dry_run=args.dry_run)
    result = dele.run_sync()
    print(report(result.plan_id))
    return 0 if result.ok else 1


def _cmd_watch(args: argparse.Namespace) -> int:
    from .observe import StatusBoard
    StatusBoard(args.plan_id).watch()
    return 0


def _cmd_report(args: argparse.Namespace) -> int:
    from .observe import report
    print(report(args.plan_id))
    return 0


def _cmd_doctor(args: argparse.Namespace) -> int:
    from .doctor import doctor, print_report
    backends = (args.backends.split(",") if args.backends else None)
    return print_report(doctor(repo=args.repo, backends=backends))


def _cmd_stats(args: argparse.Namespace) -> int:
    from .analytics import Analytics
    rows = Analytics().table()
    if not rows:
        print("no verified runs yet — stats build up automatically")
        return 0
    if args.json:
        print(json.dumps(rows, indent=2))
        return 0
    hdr = (f"{'task_class':<22} {'backend':<10} {'model':<24} "
           f"{'n':>4} {'pass':>5} {'wilson':>7} {'~sec':>6} {'~try':>5}")
    print(hdr)
    print("-" * len(hdr))
    for r in rows:
        print(f"{r['task_class']:<22} {r['backend']:<10} "
              f"{r['model']:<24} {r['trials']:>4} "
              f"{r['pass_rate']:>5.0%} {r['wilson_score']:>7.3f} "
              f"{r['ema_seconds']:>6.0f} {r['ema_attempts']:>5.1f}")
    return 0


def _cmd_escalations(args: argparse.Namespace) -> int:
    open_ = EscalationBroker().open_escalations(args.plan_id)
    if not open_:
        print("no open escalations")
        return 0
    for e in open_:
        print(f"\n[{e['id']}] {e['kind']}  task: {e['task_title']}")
        print(f"  {e['summary']}")
        if e.get("branch"):
            print(f"  branch: {e['branch']}")
        for o in e["options"]:
            inp = " (requires --input)" if o["requires_input"] else ""
            print(f"    --option {o['id']:<8} {o['label']}{inp}")
            print(f"             -> {o['consequence']}")
    return 0


def _cmd_resolve(args: argparse.Namespace) -> int:
    result = EscalationBroker().resolve(
        args.plan_id, args.escalation_id, args.option,
        user_input=args.input, decided_by="cli")
    if not result["ok"]:
        print(f"error: {result['error']}", file=sys.stderr)
        return 1
    print(f"resolved -> {result['action']} for task {result['task_id']}")
    print("run `sin delegate resume <plan_id>` to continue the run")
    return 0


def _cmd_resume(args: argparse.Namespace) -> int:
    from .observe import report
    from .planner import plan_from_dict as _pfd
    ledger = Ledger()
    raw = ledger.load_plan_json(args.plan_id)
    if not raw:
        print(f"unknown plan {args.plan_id}", file=sys.stderr)
        return 1
    data = json.loads(raw)
    plan = _pfd(
        {"goal": data["goal"], "base_branch": data.get("base_branch", "main"),
         "tasks": [{**t, "key": t["id"],
                    "files": t.get("files_hint", [])}
                   for t in data["tasks"]]},
        repo=data.get("repo", args.repo))
    dele = Delegator(plan, ledger=ledger, max_parallel=args.parallel)
    result = dele.run_sync()
    print(report(result.plan_id))
    return 0 if result.ok else 1


def build_parser(prog: str = "sin-delegate") -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(prog=prog, description=__doc__)
    sub = p.add_subparsers(dest="cmd", required=True)

    run = sub.add_parser("run", help="execute a plan file")
    run.add_argument("plan")
    run.add_argument("--repo", default=".")
    run.add_argument("--parallel", type=int, default=4)
    run.add_argument("--dry-run", action="store_true")
    run.add_argument("--keep-worktrees", action="store_true")
    run.add_argument("--json", action="store_true")
    run.set_defaults(fn=_cmd_run)

    pl = sub.add_parser("plan",
                        help="LLM-decompose a goal into a plan file")
    pl.add_argument("goal")
    pl.add_argument("--repo", default=".")
    pl.add_argument("--backend", default="opencode")
    pl.add_argument("--model", default="")
    pl.add_argument("--out", default="")
    pl.add_argument("--no-critique", action="store_true")
    pl.set_defaults(fn=_cmd_plan)

    au = sub.add_parser("auto", help="plan + execute a goal end-to-end")
    au.add_argument("goal")
    au.add_argument("--repo", default=".")
    au.add_argument("--backend", default="opencode")
    au.add_argument("--model", default="")
    au.add_argument("--parallel", type=int, default=4)
    au.add_argument("--dry-run", action="store_true")
    au.add_argument("-y", "--yes", action="store_true")
    au.set_defaults(fn=_cmd_auto)

    st = sub.add_parser("status", help="latest state per task")
    st.add_argument("plan_id")
    st.set_defaults(fn=_cmd_status)

    hi = sub.add_parser("history", help="full event log of a run")
    hi.add_argument("plan_id")
    hi.set_defaults(fn=_cmd_history)

    sub.add_parser("runs", help="list known runs").set_defaults(fn=_cmd_runs)

    ca = sub.add_parser("cancel", help="request cooperative cancellation")
    ca.add_argument("plan_id")
    ca.set_defaults(fn=_cmd_cancel)

    wa = sub.add_parser("watch", help="live status board for a running plan")
    wa.add_argument("plan_id")
    wa.set_defaults(fn=_cmd_watch)

    re_ = sub.add_parser("report", help="markdown report of a run")
    re_.add_argument("plan_id")
    re_.set_defaults(fn=_cmd_report)

    doc = sub.add_parser("doctor", help="preflight health check")
    doc.add_argument("--repo", default=".")
    doc.add_argument("--backends", default="")
    doc.set_defaults(fn=_cmd_doctor)

    st_ = sub.add_parser(
        "stats",
        help="learned backend performance per task class (Wilson-scored)")
    st_.add_argument("--json", action="store_true")
    st_.set_defaults(fn=_cmd_stats)

    es = sub.add_parser("escalations",
                        help="list open decision requests of a run")
    es.add_argument("plan_id")
    es.set_defaults(fn=_cmd_escalations)

    rv = sub.add_parser("resolve", help="answer an escalation")
    rv.add_argument("plan_id")
    rv.add_argument("escalation_id")
    rv.add_argument("--option", required=True)
    rv.add_argument("--input", default="",
                    help="guidance text for retry options")
    rv.set_defaults(fn=_cmd_resolve)

    rs = sub.add_parser("resume",
                        help="apply resolutions and continue a run")
    rs.add_argument("plan_id")
    rs.add_argument("--repo", default=".")
    rs.add_argument("--parallel", type=int, default=4)
    rs.set_defaults(fn=_cmd_resume)

    return p


def main(argv=None) -> int:
    args = build_parser().parse_args(argv)
    return args.fn(args)


def register(argv: list) -> int:
    """Plugin contract for the `sin` lazy-dispatch:
    `sin delegate run plan.json` → register(["run", "plan.json"]).
    Reuse main() with a custom prog name."""
    parser = build_parser(prog="sin delegate")
    args = parser.parse_args(argv)
    return args.fn(args)


if __name__ == "__main__":
    raise SystemExit(main())
