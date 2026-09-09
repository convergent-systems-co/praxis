"""Tests for build_adapters(), the shared adapter construction path used by
the praxis_cli commands that need a concrete list of Executor instances.
"""

from __future__ import annotations

from praxis_cli.adapters import build_adapters
from praxis_executors.adapters.claude_cli import ClaudeCliExecutor
from praxis_executors.adapters.fake import FakeCapabilityExecutor
from praxis_executors.adapters.ollama import OllamaExecutor
from praxis_executors.adapters.subprocess_executor import SubprocessExecutor


def test_build_adapters_returns_one_of_each_adapter_class():
    adapters = build_adapters()

    assert isinstance(adapters, list)
    assert len(adapters) == 4
    assert sum(isinstance(a, SubprocessExecutor) for a in adapters) == 1
    assert sum(isinstance(a, FakeCapabilityExecutor) for a in adapters) == 1
    assert sum(isinstance(a, ClaudeCliExecutor) for a in adapters) == 1
    assert sum(isinstance(a, OllamaExecutor) for a in adapters) == 1


def test_build_adapters_construction_never_raises_without_claude_or_ollama_on_path(monkeypatch):
    monkeypatch.setenv("PATH", "")

    adapters = build_adapters()

    assert len(adapters) == 4


def test_build_adapters_returns_independently_constructed_objects_each_call():
    first = build_adapters()
    second = build_adapters()

    assert len(first) == len(second) == 4
    for a, b in zip(first, second):
        assert a is not b
