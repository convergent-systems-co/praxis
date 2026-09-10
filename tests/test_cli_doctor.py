"""Hermetic tests for the five-block `praxis doctor` command."""

from __future__ import annotations

from praxis_cli.doctor_cmd import run_doctor


def test_doctor_prints_all_checks_in_order_and_warns_without_adapters(capsys):
    exit_code = run_doctor(lambda: {})

    output = capsys.readouterr().out
    names = [line[:-1] for line in output.splitlines() if line.endswith(":")]
    assert names == ["runtime", "configuration", "documents", "executors", "policy"]
    assert "status: skipped (no document supplied)" in output
    assert exit_code == 0


def test_doctor_continues_after_adapter_factory_failure(capsys):
    def build():
        raise RuntimeError("adapter construction failed")

    exit_code = run_doctor(build)

    output = capsys.readouterr().out
    assert exit_code == 1
    assert "executors:" in output
    assert "reason: RuntimeError: adapter construction failed" in output
    assert "policy:" in output
