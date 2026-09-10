"""The `praxis doctor` orchestrator.

The five blocks are deliberately ordered runtime, configuration, documents,
executors, and policy. Each check is guarded so one broken installation check
does not hide the checks that follow it.
"""

from __future__ import annotations

from typing import Callable, Mapping, Sequence

from praxis_cli import doctor_documents, doctor_env, doctor_executors, doctor_report
from praxis_executors.interface import Executor


def run_doctor(
    build_adapters: Callable[[], Mapping[str, Executor]],
    *,
    graphs: Sequence[str] = (),
    overlay_manifests: Sequence[str] = (),
) -> int:
    """Run and print the five checks in their stable public order."""
    results: list[doctor_report.CheckResult] = []

    results.append(doctor_report.guarded("runtime", doctor_env.check_runtime))
    doctor_report.print_check(results[-1])
    results.append(
        doctor_report.guarded("configuration", doctor_env.check_configuration)
    )
    doctor_report.print_check(results[-1])
    results.append(
        doctor_report.guarded(
            "documents",
            lambda: doctor_documents.check_documents(graphs, overlay_manifests),
        )
    )
    doctor_report.print_check(results[-1])

    rows: list[dict] = []
    discovery_result, rows = doctor_executors.check_discovery(build_adapters)
    results.append(discovery_result)
    doctor_report.print_check(discovery_result)

    policy_result = doctor_report.guarded(
        "policy", lambda: doctor_executors.check_policy(rows)
    )
    results.append(policy_result)
    doctor_report.print_check(policy_result)
    return doctor_report.exit_code(results)
