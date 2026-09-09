"""Tests for praxis_cli.fields: per-adapter field derivation for the CLI.

Constructs real adapter instances and patches `shutil.which` / the
instances' own `.health()` so no real `claude`/`ollama` process is ever
invoked.
"""

from __future__ import annotations

from praxis_executors.adapters.claude_cli import ClaudeCliExecutor
from praxis_executors.adapters.fake import FakeCapabilityExecutor
from praxis_executors.adapters.ollama import OllamaExecutor
from praxis_executors.adapters.subprocess_executor import SubprocessExecutor
from praxis_executors.interface import Executor, ExecutorAvailability

from praxis_cli.fields import (
    auth_transports,
    authenticated_field,
    capability_kinds,
    installed_field,
    render_cell,
    version_field,
)


class _UnknownExecutor(Executor):
    """An `Executor` implemented straight off the ABC.

    Deliberately none of the four concrete adapter classes `fields.py`
    dispatches on -- a stand-in for the fifth adapter that lands after this
    module was written, which the CLI has to survive.
    """

    def capabilities(self) -> dict:
        raise NotImplementedError

    def health(self) -> ExecutorAvailability:
        raise NotImplementedError

    def launch(self, request):
        raise NotImplementedError

    def status(self, handle):
        raise NotImplementedError

    def cancel(self, handle):
        raise NotImplementedError

    def result(self, handle):
        raise NotImplementedError


def _claude() -> ClaudeCliExecutor:
    return ClaudeCliExecutor(executor_id="executor-claude-cli-1")


def _ollama() -> OllamaExecutor:
    return OllamaExecutor(executor_id="executor-ollama-1")


def _subprocess() -> SubprocessExecutor:
    return SubprocessExecutor(executor_id="executor-subprocess-1", satisfies_kinds=["tools"])


def _fake() -> FakeCapabilityExecutor:
    return FakeCapabilityExecutor(executor_id="executor-fake-1", capabilities=[], script={})


# installed_field()


def test_installed_field_claude_yes_when_on_path(monkeypatch):
    monkeypatch.setattr("praxis_cli.fields.shutil.which", lambda name: "/usr/local/bin/claude")

    assert installed_field(_claude()) == "yes"


def test_installed_field_claude_no_when_not_on_path(monkeypatch):
    monkeypatch.setattr("praxis_cli.fields.shutil.which", lambda name: None)

    assert installed_field(_claude()) == "no"


def test_installed_field_ollama_yes_when_reachable(monkeypatch):
    executor = _ollama()
    monkeypatch.setattr(executor, "health", lambda: ExecutorAvailability.AVAILABLE)

    assert installed_field(executor) == "yes"


def test_installed_field_ollama_no_when_unreachable(monkeypatch):
    executor = _ollama()
    monkeypatch.setattr(executor, "health", lambda: ExecutorAvailability.UNAVAILABLE)

    assert installed_field(executor) == "no"


def test_installed_field_subprocess_is_builtin():
    assert installed_field(_subprocess()) == "n/a (built-in)"


def test_installed_field_fake_is_builtin():
    assert installed_field(_fake()) == "n/a (built-in)"


def test_installed_field_ollama_unknown_when_health_raises_json_decode_error(monkeypatch):
    # A non-Ollama server answering 200 with a non-JSON body surfaces as
    # `json.JSONDecodeError`, a `ValueError` subclass -- not the adapter's own
    # `ExecutorError` -- and must degrade this one field, not crash the row.
    executor = _ollama()

    def _raise():
        raise ValueError("Expecting value: line 1 column 1 (char 0)")

    monkeypatch.setattr(executor, "health", _raise)

    assert installed_field(executor) == "unknown"


def test_installed_field_returns_a_neutral_value_for_an_unrecognised_adapter():
    # "Installed" is not derivable for a class this module knows nothing
    # about, and raising would take a whole command down for one unknown row.
    assert installed_field(_UnknownExecutor()) == "n/a"


# version_field()


def test_version_field_always_unknown():
    assert version_field(_claude()) == "unknown"
    assert version_field(_ollama()) == "unknown"
    assert version_field(_subprocess()) == "unknown"
    assert version_field(_fake()) == "unknown"


# authenticated_field()


def test_authenticated_field_claude_not_installed():
    assert authenticated_field(_claude(), installed="no") == "n/a (not installed)"


def test_authenticated_field_claude_installed_available(monkeypatch):
    executor = _claude()
    monkeypatch.setattr(executor, "health", lambda: ExecutorAvailability.AVAILABLE)

    assert authenticated_field(executor, installed="yes") == "yes"


def test_authenticated_field_claude_installed_degraded(monkeypatch):
    executor = _claude()
    monkeypatch.setattr(executor, "health", lambda: ExecutorAvailability.DEGRADED)

    assert authenticated_field(executor, installed="yes") == "unknown"


def test_authenticated_field_claude_installed_unavailable(monkeypatch):
    executor = _claude()
    monkeypatch.setattr(executor, "health", lambda: ExecutorAvailability.UNAVAILABLE)

    assert authenticated_field(executor, installed="yes") == "no"


def test_authenticated_field_claude_installed_unknown_when_health_raises_json_decode_error(monkeypatch):
    executor = _claude()

    def _raise():
        raise ValueError("Expecting value: line 1 column 1 (char 0)")

    monkeypatch.setattr(executor, "health", _raise)

    assert authenticated_field(executor, installed="yes") == "unknown"


def test_authenticated_field_ollama_is_na():
    assert authenticated_field(_ollama(), installed="yes") == "n/a"


def test_authenticated_field_subprocess_is_na():
    assert authenticated_field(_subprocess(), installed="n/a (built-in)") == "n/a"


def test_authenticated_field_fake_is_na():
    assert authenticated_field(_fake(), installed="n/a (built-in)") == "n/a"


# capability_kinds()


def _advertisement_with_duplicates() -> dict:
    return {
        "spec_version": "1.0.0",
        "executor_id": "executor-x",
        "capabilities": [
            {
                "spec_version": "1.0.0",
                "satisfies": [{"kind": "coding"}, {"kind": "reasoning"}],
                "auth_transport": "local",
            },
            {
                "spec_version": "1.0.0",
                "satisfies": [{"kind": "reasoning"}, {"kind": "tools"}],
                "auth_transport": "subscription_cli",
            },
        ],
    }


def test_capability_kinds_dedups_preserving_first_seen_order():
    advertisement = _advertisement_with_duplicates()

    assert capability_kinds(advertisement) == ["coding", "reasoning", "tools"]


# auth_transports()


def test_auth_transports_dedups_preserving_first_seen_order():
    advertisement = _advertisement_with_duplicates()

    assert auth_transports(advertisement) == ["local", "subscription_cli"]


# render_cell() -- the one display rule `discover` and `status` share


def test_render_cell_joins_a_list_without_python_syntax():
    assert render_cell(["coding", "reasoning"]) == "coding,reasoning"


def test_render_cell_renders_none_as_empty_and_leaves_a_string_alone():
    assert render_cell(None) == ""
    assert render_cell("available") == "available"
    assert render_cell([]) == ""
