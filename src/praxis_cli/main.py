from __future__ import annotations

import argparse
import importlib.metadata
import sys


_RECOGNIZED_COMMANDS = frozenset({"executors", "doctor", "run"})


def _print_version() -> None:
    print(importlib.metadata.version("praxis-contracts"))


def _build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="praxis")
    subparsers = parser.add_subparsers()

    executors_parser = subparsers.add_parser("executors")
    executors_parser.add_argument("--json", action="store_true")
    executors_subparsers = executors_parser.add_subparsers(dest="executors_command")

    executors_subparsers.add_parser("discover")

    match_parser = executors_subparsers.add_parser("match")
    match_parser.add_argument("--capability", action="append", required=True)
    match_parser.add_argument("--explain", action="store_true", default=False)

    return parser


def _build_doctor_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="praxis doctor")
    parser.add_argument("--graph", action="append", default=[])
    parser.add_argument("--overlay-manifest", action="append", default=[])
    return parser


def _build_run_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="praxis run")
    parser.add_argument("target")
    parser.add_argument("--executor", default="auto")
    parser.add_argument("--capability", action="append", default=[])
    parser.add_argument("--run-dir", required=True)
    parser.add_argument("--run-id")
    return parser


def main(argv: list[str] | None = None) -> int:
    if argv is None:
        argv = sys.argv[1:]

    # Criterion 3: anything whose first token is not one of the three recognized
    # commands never reaches
    # `argparse` at all, which is what keeps `praxis --version` and the
    # pre-existing bare `main()` call working unchanged.
    #
    # Known limitation, recorded in docs/develop/plans/b2-issue45.md: an
    # unrecognized subcommand (`praxis bogus`) takes this path too, so it prints
    # a version and exits 0 with no diagnostic. Narrowing the gate to reject one
    # is its own change -- it has to distinguish an unknown subcommand from the
    # legacy flags this branch exists to pass through -- and wants its own issue.
    if not argv or argv[0] not in _RECOGNIZED_COMMANDS:
        _print_version()
        return 0

    if argv[0] == "doctor":
        parser = _build_doctor_parser()
        args = parser.parse_args(argv[1:])
        from praxis_cli import adapters, doctor_cmd

        return doctor_cmd.run_doctor(
            adapters.build_adapters,
            graphs=args.graph,
            overlay_manifests=args.overlay_manifest,
        )

    if argv[0] == "run":
        parser = _build_run_parser()
        args = parser.parse_args(argv[1:])
        from praxis_cli import adapters, run_cmd

        return run_cmd.run_run(
            adapters.build_adapters(),
            target=args.target,
            executor=args.executor,
            capabilities=args.capability,
            run_dir=args.run_dir,
            run_id=args.run_id,
        )

    parser = _build_parser()
    args = parser.parse_args(argv)

    if args.json and args.executors_command is not None:
        # --json only shapes the bare `praxis executors` status output; every
        # other route would accept it and silently print the plain report.
        parser.error(f"--json is not valid for 'executors {args.executors_command}'")

    # Imported here, not at module load: the legacy version path above never
    # needs an adapter, and importing them eagerly pulls in every backing
    # adapter module for a command that only prints a version string.
    from praxis_cli import adapters, discover_cmd, match_cmd, status_cmd

    built = adapters.build_adapters()

    if args.executors_command == "discover":
        return discover_cmd.run_discover(built)
    if args.executors_command == "match":
        return match_cmd.run_match(built, capabilities=args.capability, explain=args.explain)
    return status_cmd.run_status(built, as_json=args.json)
