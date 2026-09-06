"""CLI entrypoint for the dashboard.

`main`'s `--replay-only` path never imports `.server` at all -- a
completed-run inspection needs no live HTTP server, and keeping the import
out of that path is what lets tests exercise `--replay-only` without ever
constructing a socket.
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path

from . import snapshot
from .sources import DashboardSource


def parse_args(argv: "list[str] | None" = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(prog="praxis-dashboard")
    parser.add_argument("--graph", required=True)
    parser.add_argument("--run-dir", required=True)
    parser.add_argument("--lease-dir", default=None)
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=0)
    parser.add_argument("--replay-only", action="store_true")
    return parser.parse_args(argv)


def main(argv: "list[str] | None" = None) -> int:
    args = parse_args(argv)

    source = DashboardSource(
        Path(args.graph),
        Path(args.run_dir),
        lease_directory=Path(args.lease_dir) if args.lease_dir else None,
    )

    if args.replay_only:
        print(json.dumps(snapshot.snapshot_to_document(source.replay_snapshot())))
        return 0

    from . import server

    server.serve(source, host=args.host, port=args.port).serve_forever()
    return 0
