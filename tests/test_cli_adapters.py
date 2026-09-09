"""Tests for build_adapters(), the shared adapter construction path used by
the praxis_cli commands that need concrete Executor instances.

It returns a `{executor_id: instance}` mapping: the id is what a command
names an executor by, and the adapter's own advertisement -- otherwise the
only public source of it -- is unavailable exactly when the adapter is
unhealthy.

Construction alone probes nothing, so calling `build_adapters()` here starts
no `claude` subprocess and opens no Ollama socket.
"""

from __future__ import annotations

from praxis_cli.adapters import build_adapters
from praxis_executors.adapters.claude_cli import ClaudeCliExecutor
from praxis_executors.adapters.fake import FakeCapabilityExecutor
from praxis_executors.adapters.ollama import OllamaExecutor
from praxis_executors.adapters.subprocess_executor import SubprocessExecutor


def test_build_adapters_returns_one_of_each_adapter_class_keyed_by_executor_id():
    adapters = build_adapters()

    assert isinstance(adapters, dict)
    assert len(adapters) == 4
    instances = list(adapters.values())
    assert sum(isinstance(a, SubprocessExecutor) for a in instances) == 1
    assert sum(isinstance(a, FakeCapabilityExecutor) for a in instances) == 1
    assert sum(isinstance(a, ClaudeCliExecutor) for a in instances) == 1
    assert sum(isinstance(a, OllamaExecutor) for a in instances) == 1


def test_build_adapters_keys_match_the_id_each_instance_was_constructed_with():
    adapters = build_adapters()

    assert list(adapters) == [
        "executor-subprocess-1",
        "executor-fake-1",
        "executor-claude-cli-1",
        "executor-ollama-1",
    ]
    # Every adapter but Ollama advertises without probing, so its key can be
    # checked against the id its own advertisement carries.
    for executor_id in list(adapters)[:-1]:
        assert adapters[executor_id].capabilities()["executor_id"] == executor_id


def test_build_adapters_construction_never_raises_without_claude_or_ollama_on_path(monkeypatch):
    monkeypatch.setenv("PATH", "")

    adapters = build_adapters()

    assert len(adapters) == 4


def test_build_adapters_returns_independently_constructed_objects_each_call():
    first = build_adapters()
    second = build_adapters()

    assert first.keys() == second.keys()
    for key in first:
        assert first[key] is not second[key]


def test_build_adapters_does_not_share_capability_data_between_calls():
    # Independently constructed instances are only half the property:
    # `FakeCapabilityExecutor.__init__` copies the outer capability list and
    # nothing inside it, so a capability dict held at module scope would be the
    # same object in every instance the process ever builds. A caller editing a
    # returned advertisement would then be editing the constant every later
    # call reads.
    first = build_adapters()["executor-fake-1"].capabilities()["capabilities"]
    second = build_adapters()["executor-fake-1"].capabilities()["capabilities"]

    assert first[0] is not second[0]
    assert first[0]["satisfies"][0] is not second[0]["satisfies"][0]

    first[0]["auth_transport"] = "metered_api"
    third = build_adapters()["executor-fake-1"].capabilities()["capabilities"]
    assert third[0]["auth_transport"] == "local"
