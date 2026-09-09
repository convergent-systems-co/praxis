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


# What `discover` passes as the caller's one `.capabilities()` result. Only its
# presence matters to `installed_field`; the contents are never read there.
_ADVERTISEMENT = {
    "spec_version": "1.0.0",
    "executor_id": "executor-x",
    "capabilities": [
        {"spec_version": "1.0.0", "satisfies": [{"kind": "coding"}], "auth_transport": "local"}
    ],
}


# installed_field()


def test_installed_field_claude_yes_when_on_path(monkeypatch):
    monkeypatch.setattr("praxis_cli.fields.shutil.which", lambda name: "/usr/local/bin/claude")

    assert installed_field(_claude(), _ADVERTISEMENT) == "yes"


def test_installed_field_claude_no_when_not_on_path(monkeypatch):
    monkeypatch.setattr("praxis_cli.fields.shutil.which", lambda name: None)

    assert installed_field(_claude(), _ADVERTISEMENT) == "no"


def test_installed_field_ollama_yes_when_reachable(monkeypatch):
    executor = _ollama()
    monkeypatch.setattr(executor, "health", lambda: ExecutorAvailability.AVAILABLE)

    assert installed_field(executor, None) == "yes"


def test_installed_field_ollama_no_when_unreachable(monkeypatch):
    executor = _ollama()
    monkeypatch.setattr(executor, "health", lambda: ExecutorAvailability.UNAVAILABLE)

    assert installed_field(executor, None) == "no"


def test_installed_field_ollama_reads_a_returned_advertisement_instead_of_reprobing(monkeypatch):
    # `capabilities()` and `health()` both GET `/api/tags`. An advertisement
    # that came back already answers the question `health()` would re-ask.
    executor = _ollama()

    def _unexpected_probe():
        raise AssertionError("health() re-probed a service that already advertised")

    monkeypatch.setattr(executor, "health", _unexpected_probe)

    assert installed_field(executor, _ADVERTISEMENT) == "yes"


def test_installed_field_subprocess_is_builtin():
    assert installed_field(_subprocess(), _ADVERTISEMENT) == "n/a (built-in)"


def test_installed_field_fake_is_builtin():
    assert installed_field(_fake(), _ADVERTISEMENT) == "n/a (built-in)"


def test_installed_field_ollama_unknown_when_health_raises_json_decode_error(monkeypatch):
    # A non-Ollama server answering 200 with a non-JSON body surfaces as
    # `json.JSONDecodeError`, a `ValueError` subclass -- not the adapter's own
    # `ExecutorError` -- and must degrade this one field, not crash the row.
    executor = _ollama()

    def _raise():
        raise ValueError("Expecting value: line 1 column 1 (char 0)")

    monkeypatch.setattr(executor, "health", _raise)

    assert installed_field(executor, None) == "unknown"


def test_installed_field_returns_a_neutral_value_for_an_unrecognised_adapter():
    # "Installed" is not derivable for a class this module knows nothing
    # about, and raising would take a whole command down for one unknown row.
    assert installed_field(_UnknownExecutor(), _ADVERTISEMENT) == "n/a"


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


def test_authenticated_field_claude_survives_a_version_probe_that_cannot_run(monkeypatch):
    # Pins the real adapter behaviour that leaves `authenticated_field` with no
    # `ValueError` to guard against: the one subprocess `health()` runs already
    # swallows its own `OSError`, so a `claude` binary that will not execute
    # still resolves to an availability rather than an exception.
    executor = _claude()
    monkeypatch.setattr(
        "praxis_executors.adapters.claude_cli.shutil.which", lambda name: "/usr/local/bin/claude"
    )

    def _raise(*args, **kwargs):
        raise OSError("Exec format error")

    monkeypatch.setattr("praxis_executors.adapters.claude_cli.subprocess.run", _raise)

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


def _partial_transport_advertisement() -> dict:
    """Schema-valid without an `auth_transport` on every capability.

    `capability.schema.json` requires only `spec_version` and `satisfies`, so a
    conforming adapter may advertise a capability that names no transport.
    """
    return {
        "spec_version": "1.0.0",
        "executor_id": "executor-x",
        "capabilities": [
            {"spec_version": "1.0.0", "satisfies": [{"kind": "coding"}]},
            {
                "spec_version": "1.0.0",
                "auth_transport": "local",
                "satisfies": [{"kind": "reasoning"}],
            },
        ],
    }


def test_auth_transports_skips_a_capability_that_names_no_transport():
    assert auth_transports(_partial_transport_advertisement()) == ["local"]


def test_auth_transports_is_empty_when_no_capability_names_a_transport():
    advertisement = {
        "spec_version": "1.0.0",
        "executor_id": "executor-x",
        "capabilities": [{"spec_version": "1.0.0", "satisfies": [{"kind": "coding"}]}],
    }

    assert auth_transports(advertisement) == []


# render_cell() -- the one display rule `discover` and `status` share


def test_render_cell_joins_a_list_without_python_syntax():
    assert render_cell(["coding", "reasoning"]) == "coding,reasoning"


def test_render_cell_renders_none_as_empty_and_leaves_a_string_alone():
    assert render_cell(None) == ""
    assert render_cell("available") == "available"
    assert render_cell([]) == ""
