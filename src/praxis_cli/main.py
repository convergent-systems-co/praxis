from __future__ import annotations

import argparse
import importlib.metadata
import sys


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


def main(argv: list[str] | None = None) -> int:
    if argv is None:
        argv = sys.argv[1:]

    if not argv or argv[0] != "executors":
        _print_version()
        return 0

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
        return match_cmd.run_match(
            built.values(), capabilities=args.capability, explain=args.explain
        )
    return status_cmd.run_status(built, as_json=args.json)
