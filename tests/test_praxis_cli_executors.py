from __future__ import annotations

import json
import re

from praxis_cli.main import main

_STATUS_KEYS = {"executor_id", "auth_transport", "status", "capabilities"}


def test_discover_returns_zero_and_prints_all_four_executors(capsys):
    exit_code = main(["executors", "discover"])

    captured = capsys.readouterr()
    assert exit_code == 0
    header_lines = [line for line in captured.out.splitlines() if line.endswith(":")]
    assert len(header_lines) == 4


def test_bare_executors_prints_status_table(capsys):
    exit_code = main(["executors"])

    captured = capsys.readouterr()
    assert exit_code == 0
    # One line per executor distinguishes the status table from both the
    # single-line version fallback and the single-line --json blob.
    assert len(captured.out.splitlines()) == 4


def test_executors_json_prints_four_status_objects(capsys):
    exit_code = main(["executors", "--json"])

    captured = capsys.readouterr()
    assert exit_code == 0
    rows = json.loads(captured.out)
    assert isinstance(rows, list)
    assert len(rows) == 4
    for row in rows:
        assert _STATUS_KEYS.issubset(row.keys())


def test_match_with_capability_and_explain_does_not_crash(capsys):
    exit_code = main(["executors", "match", "--capability", "coding", "--explain"])

    captured = capsys.readouterr()
    assert exit_code == 0
    # "eligible=" only ever comes from match_cmd's --explain output, so this
    # fails if main() mis-routes "executors match" to the status branch.
    assert "eligible=" in captured.out


def test_no_subcommand_still_prints_version(capsys):
    exit_code = main(["--version"])

    captured = capsys.readouterr()
    assert exit_code == 0
    assert re.match(r"\d+\.\d+\.\d+", captured.out.strip())
