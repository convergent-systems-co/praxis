"""RED-phase proof for T3 (issue #51): `docs/executors.md` must gain an
`## Executor configuration` section between `## praxis_executors.policy` and
`## praxis_executors.registry` documenting the config document shape, discovery
order, precedence and merge rules, the five environment variables from spec
criterion 9, the config-file-only `allowed_auth_transports` knob, and the
fail-closed error posture -- and `README.md`'s `### Inspecting executors` section
must point at that surface without restating the table.

This is a doc-content task whose footprint (`docs/executors.md`, `README.md`)
carries no test file of its own, while the task's evidence bar requires RED
proof. The repository already resolved this same class of question twice: T20 of
issue #28 added `tests/test_parity_decision_addendum.py` for `docs/parity/
decision.md`, and T10 added `tests/test_development_overlay_doc.py` for
`docs/overlays/development.md`, both citing `agents/tdd-writer.md`'s priority
that a real test in a built-in facility beats no test. This file applies the
identical resolution to T3; it extends T3's footprint by exactly this one new
path, which collides with no concurrent task in this bundle.

Assertions are grounded in the T3 brief's steps and the enhanced spec's criteria
6-10 and 13; the reproduced default document is the one in
`docs/develop/plans/b-issue51.md`.
"""

from __future__ import annotations

from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
EXECUTORS_DOC = REPO_ROOT / "docs" / "executors.md"
README = REPO_ROOT / "README.md"

CONFIG_HEADING = "## Executor configuration"
POLICY_HEADING = "## `praxis_executors.policy`"
REGISTRY_HEADING = "## `praxis_executors.registry`"
ADAPTER_HEADING = "## Adding a new executor adapter"
README_CLI_HEADING = "### Inspecting executors: the `praxis` CLI"

DEFAULT_PROVIDER_IDS = [
    "executor-subprocess-1",
    "executor-fake-1",
    "executor-claude-cli-1",
    "executor-ollama-1",
]

ENV_VARIABLES = [
    "PRAXIS_EXECUTORS_ENABLED",
    "PRAXIS_EXECUTORS_DISABLED",
    "PRAXIS_EXECUTORS_DENIED_AUTH_TRANSPORTS",
    "PRAXIS_EXECUTORS_PREFERRED_CAPABILITY_KINDS",
    "PRAXIS_CONFIG",
]


def _executors_doc() -> str:
    return EXECUTORS_DOC.read_text()


def _config_section() -> str:
    doc_text = _executors_doc()
    idx = doc_text.find(CONFIG_HEADING)
    assert idx != -1, (
        f"expected {EXECUTORS_DOC} to contain a {CONFIG_HEADING!r} heading"
    )
    rest = doc_text[idx:]
    next_idx = rest.find("\n## ", 1)
    return rest if next_idx == -1 else rest[:next_idx]


def _readme_cli_section() -> str:
    readme_text = README.read_text()
    idx = readme_text.find(README_CLI_HEADING)
    assert idx != -1, f"README.md no longer has {README_CLI_HEADING!r}"
    rest = readme_text[idx:]
    next_idx = rest.find("\n### ", 1)
    return rest if next_idx == -1 else rest[:next_idx]


def test_config_section_sits_between_policy_and_registry() -> None:
    doc_text = _executors_doc()
    for heading in (POLICY_HEADING, REGISTRY_HEADING, ADAPTER_HEADING):
        assert heading in doc_text, (
            f"T3 must extend around the existing {heading!r} section, not remove "
            "or rename it"
        )
    policy_idx = doc_text.find(POLICY_HEADING)
    config_idx = doc_text.find(CONFIG_HEADING)
    registry_idx = doc_text.find(REGISTRY_HEADING)
    assert config_idx != -1, (
        f"docs/executors.md must gain a {CONFIG_HEADING!r} section"
    )
    assert policy_idx < config_idx < registry_idx, (
        "the Executor configuration section must sit after "
        f"{POLICY_HEADING!r} and before {REGISTRY_HEADING!r}, so the policy "
        "vocabulary it references is already introduced"
    )


def test_config_section_reproduces_the_default_document_as_json() -> None:
    section = _config_section()
    assert "JSON" in section, (
        "the section must state the config file is JSON, not YAML"
    )
    assert "```json" in section, (
        "the section must reproduce the fully populated default document in a "
        "json code block"
    )
    for provider_id in DEFAULT_PROVIDER_IDS:
        assert provider_id in section, (
            f"the reproduced default document must list the default-enabled id "
            f"{provider_id!r}"
        )
    for leaf in (
        '"spec_version"',
        '"providers"',
        '"allowed_auth_transports": null',
        '"denied_auth_transports": []',
        '"allowed_executor_ids": null',
        '"denied_executor_ids": []',
        '"preferred_capability_kinds": []',
    ):
        assert leaf in section, (
            f"the reproduced default document must contain {leaf} exactly as the "
            "plan's default_document() spells it"
        )


def test_config_section_documents_discovery_order() -> None:
    section = _config_section()
    assert "`$PRAXIS_CONFIG`" in section or "`PRAXIS_CONFIG`" in section, (
        "discovery order must name the PRAXIS_CONFIG variable"
    )
    assert "~/.config/praxis/config.json" in section, (
        "discovery order must name the default user config path"
    )
    explicit_idx = section.lower().find("explicit")
    env_idx = section.find("PRAXIS_CONFIG")
    default_path_idx = section.find("~/.config/praxis/config.json")
    assert explicit_idx != -1, (
        "discovery order must start with an explicit path passed by the caller"
    )
    assert explicit_idx < env_idx < default_path_idx, (
        "discovery order must read most-specific-first: explicit path, then "
        "$PRAXIS_CONFIG, then ~/.config/praxis/config.json, then defaults only"
    )
    assert "not an error" in section, (
        "the section must state a missing file at the default location is not "
        "an error"
    )


def test_config_section_documents_precedence_and_merge_rules() -> None:
    section = _config_section()
    section_lower = section.lower()
    defaults_idx = section_lower.find("default")
    file_idx = section_lower.find("file", defaults_idx)
    env_idx = section_lower.find("environment", file_idx)
    assert -1 not in (defaults_idx, file_idx, env_idx), (
        "the section must document precedence as defaults, then file, then "
        "environment"
    )
    assert defaults_idx < file_idx < env_idx, (
        "precedence must be stated in order: defaults, then file, then "
        "environment wins"
    )
    assert "key-by-key" in section_lower, (
        "the merge rules must state objects merge key-by-key"
    )
    assert "replace" in section_lower, (
        "the merge rules must state scalars replace"
    )
    assert "wholesale" in section_lower and "union" in section_lower, (
        "the merge rules must state arrays replace wholesale and never union"
    )


def test_config_section_reproduces_the_five_variable_table() -> None:
    section = _config_section()
    for variable in ENV_VARIABLES:
        assert variable in section, (
            f"the environment table must list {variable}"
        )
    assert "| ---" in section or "|---" in section, (
        "the five environment variables must be reproduced as a markdown table, "
        "as in spec criterion 9"
    )
    assert "PRAXIS_EXECUTORS_ALLOWED_AUTH_TRANSPORTS" not in section, (
        "no PRAXIS_EXECUTORS_ALLOWED_AUTH_TRANSPORTS variable ships in this "
        "bundle, so the table must not list one"
    )


def test_config_section_states_the_two_unguessable_environment_rules() -> None:
    section = _config_section()
    section_lower = section.lower()
    assert "unset" in section_lower, (
        "the section must state that an unset variable is no override"
    )
    assert "empty string" in section_lower, (
        "the section must state that a variable set to the empty string is an "
        "explicit clear"
    )
    assert "clear" in section_lower, (
        "the empty-string case must be described as an explicit clear to an "
        "empty list, not as 'unset'"
    )


def test_config_section_states_enabled_replaces_providers_wholesale() -> None:
    section = _config_section()
    assert "PRAXIS_EXECUTORS_ENABLED" in section, (
        "the section must name PRAXIS_EXECUTORS_ENABLED"
    )
    assert "executors.providers" in section or "`providers`" in section, (
        "the section must name the executors.providers block that "
        "PRAXIS_EXECUTORS_ENABLED replaces"
    )
    assert "not enabled" in section, (
        "the section must state that an id absent from the resolved providers "
        "map is not enabled"
    )


def test_config_section_explains_the_config_file_only_allowed_transports() -> None:
    section = _config_section()
    assert "allowed_auth_transports" in section, (
        "the section must document the allowed_auth_transports knob"
    )
    assert "metered_api" in section and "api_key" in section, (
        "the section must name the two transports enabling them would loosen"
    )
    assert "fail-closed" in section, (
        "the section must explain the knob against the fail-closed default this "
        "document already describes"
    )
    assert "no environment variable" in section.lower(), (
        "the section must state allowed_auth_transports is honored from the "
        "config file only and has no environment variable"
    )


def test_config_section_states_the_fail_closed_error_posture() -> None:
    section = _config_section()
    section_lower = section.lower()
    assert "unknown provider id" in section_lower, (
        "the section must state that an unknown provider id is an error"
    )
    assert "non-zero" in section_lower, (
        "the section must state that a schema violation exits non-zero"
    )
    assert "never" in section_lower and "defaults" in section_lower, (
        "the section must state that an invalid config never silently falls "
        "back to defaults"
    )


def test_config_section_cross_references_adding_a_new_adapter() -> None:
    section = _config_section()
    assert "Adding a new executor adapter" in section, (
        "the section must cross-reference the existing 'Adding a new executor "
        "adapter' section"
    )
    assert "no schema change" in section.lower(), (
        "the cross-reference must state that adding an adapter still requires "
        "no schema change"
    )
    assert "open string" in section.lower(), (
        "the cross-reference must give the reason: provider keys are open "
        "strings"
    )


def test_readme_cli_section_points_at_the_config_surface() -> None:
    section = _readme_cli_section()
    assert "~/.config/praxis/config.json" in section, (
        "the README's praxis CLI section must name the config file location"
    )
    assert "PRAXIS_CONFIG" in section, (
        "the README's praxis CLI section must name the PRAXIS_CONFIG variable"
    )
    assert "docs/executors.md" in section, (
        "the README's praxis CLI section must point at docs/executors.md for "
        "the full surface"
    )


def test_readme_does_not_restate_the_environment_table() -> None:
    section = _readme_cli_section()
    for variable in (
        "PRAXIS_EXECUTORS_ENABLED",
        "PRAXIS_EXECUTORS_DISABLED",
        "PRAXIS_EXECUTORS_DENIED_AUTH_TRANSPORTS",
        "PRAXIS_EXECUTORS_PREFERRED_CAPABILITY_KINDS",
    ):
        assert variable not in section, (
            f"the README must not restate the environment table; {variable} "
            "belongs in docs/executors.md only"
        )


def test_docs_promise_no_unshipped_config_subcommand_or_flag() -> None:
    section = _config_section()
    readme_section = _readme_cli_section()
    for text, where in ((section, "docs/executors.md"), (readme_section, "README.md")):
        assert "praxis config" not in text, (
            f"{where} must not describe a `praxis config` subcommand; it does "
            "not ship in this bundle"
        )
        assert "--config" not in text, (
            f"{where} must not describe a `--config` flag; it does not ship in "
            "this bundle"
        )
