"""RED-phase proof for T11 (issues #46/#47): README.md's `## Usage` section must
document the two subcommands this bundle ships -- `praxis doctor` and
`praxis run` -- and must stop making the three claims the bundle falsifies
(spec criteria 1 and 26).

The three stale claims are quoted in the spec:

1. `README.md:320` -- "this bundle adds no graph-driving subcommands to the CLI
   -- that's separate, later work". `praxis run` is exactly such a subcommand.
2. `README.md:320` -- "The way to drive a graph to completion is still as a
   library". `praxis run` is now the shipped command that does it.
3. `README.md:415` -- "`praxis` with no arguments, or with anything other than
   `executors` as its first argument, prints the package version and exits 0".
   After this bundle the dispatch gate recognizes `executors`, `doctor` and
   `run`, and only every *other* token still takes the version path.

This is a doc-content task whose footprint is `README.md` alone, and no other
test file asserts against the prose it changes -- `tests/test_repair_findings_b2_issue45.py`
covers the executors section's runnable examples and nothing else. The same
class of question was already resolved in this repository by
`tests/test_development_overlay_doc.py` (a dedicated doc-content test file for
`docs/overlays/development.md`), which itself followed
`tests/test_parity_decision_addendum.py`; this file applies that resolution to
README.md's Usage section.

Assertions here are scoped to regions of the Usage section rather than to the
whole file, and they pin the *facts a reader needs* (flag spellings, verdict
words, exit codes, the two out-of-scope limits) rather than the sentences that
carry them. Pinning prose verbatim would make every later rewording a failing
suite, which is the failure mode `tests/test_repair_findings_b2_issue45.py`'s
docstring records for source-text assertions.
"""

from __future__ import annotations

import re
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
README = REPO_ROOT / "README.md"

USAGE_HEADING = "## Usage"

EXISTING_USAGE_HEADINGS = [
    "### Quickstart",
    "### Inspecting a run: the dashboard",
    "### Inspecting executors: the `praxis` CLI",
    "### Running the test suite",
]

EXISTING_EXECUTORS_COMMANDS = [
    "praxis executors",
    "praxis executors --json",
    "praxis executors discover",
    "praxis executors match",
]

STALE_CLAIMS = [
    "adds no graph-driving subcommands",
    "The way to drive a graph to completion is still as a library",
    "anything other than `executors` as its first argument",
]

# The five doctor checks, in the order the issue lists them and the order
# criterion 7 requires them to be printed. Each entry is the set of phrasings
# that would legitimately introduce that check; the first one present anchors it.
DOCTOR_CHECKS_IN_ORDER = [
    ("prerequisit", "python version", "runtime version"),
    ("configuration",),
    ("graph/overlay", "graph and overlay", "schema validity", "graph document"),
    ("executor discovery", "discovery"),
    ("policy",),
]


def _readme() -> str:
    return README.read_text()


def _usage_section() -> str:
    """README's `## Usage` section, up to the next heading of the same level."""
    readme = _readme()
    assert USAGE_HEADING in readme, f"README.md no longer has {USAGE_HEADING!r}"
    after = readme.split(USAGE_HEADING, 1)[1]
    return after.split("\n## ", 1)[0]


def _usage_subsections() -> list[str]:
    """The `### ...` subsections of `## Usage`, heading line included."""
    parts = _usage_section().split("\n### ")
    return ["### " + part for part in parts[1:]]


def _doctor_docs() -> str:
    """Every Usage subsection that documents `praxis doctor`."""
    found = [part for part in _usage_subsections() if "praxis doctor" in part]
    assert found, (
        "README.md's Usage section documents no `praxis doctor` command; "
        "criterion 26 requires it alongside the existing `executors` commands"
    )
    return "\n".join(found)


def _run_docs() -> str:
    """Every Usage subsection that documents `praxis run` with its flags.

    Keyed on `--executor` as well as the command name so that the Quickstart's
    pointer at `praxis run` does not stand in for the command's own reference
    block, and so the dashboard subsection (which has its own `--run-dir`)
    is never mistaken for it.
    """
    found = [
        part
        for part in _usage_subsections()
        if "praxis run" in part and "--executor" in part
    ]
    assert found, (
        "README.md's Usage section documents no `praxis run` command with its "
        "`--executor` flag; criterion 26 requires it alongside the existing "
        "`executors` commands"
    )
    return "\n".join(found)


def _prose_only(text: str) -> str:
    """`text` with fenced code blocks removed, so a synopsis line's flags do not
    stand in for the prose that has to explain them."""
    return re.sub(r"```.*?```", "", text, flags=re.DOTALL)


def _sentences(text: str) -> list[str]:
    return re.split(r"(?<=[.!?])\s+", text)


def _clauses(sentence: str) -> list[str]:
    """`sentence` split into clauses on `,`/`;`/`:`, so a check can be scoped to
    the clause that carries its claim instead of to every word in the sentence.
    A sentence that states two constraints joined by a comma otherwise lets the
    first one supply the words the second one is being searched for.
    """
    return re.split(r"[,;:]", sentence)


def _clause(sentence: str, start: int, end: int | None) -> str:
    """The clause of `sentence` beginning at `start`, ending at `end` or at the
    next `;`/`:` -- so a rule stated as "exits 0 when X and exits 1 when Y" can be
    read as two halves rather than as a bag of words that reads the same reversed.
    """
    clause = sentence[start:end] if end is not None else sentence[start:]
    return re.split(r"[;:]", clause)[0]


EXIT_0 = re.compile(r"exit(s|\s+code)?\s+0", re.I)
EXIT_NONZERO = re.compile(r"exit(s|\s+code)?\s+1|nonzero", re.I)
NEGATION = re.compile(r"\b(no|none|not|neither|nor|never|without)\b|n't", re.I)

# A refusal the prose actually asserts, as opposed to one it names only to deny.
# `is refused` counts; `rather than refused` and `never refused` do not -- a bare
# /refus/ search matches an inverted claim just as happily as the true one.
AFFIRMATIVE_REFUSAL = re.compile(
    r"\b(is|are|was|were|be|being|gets?)\s+(\w+ly\s+)?refused\b|\brefus(es|ing)\b",
    re.I,
)
DENIED_REFUSAL = re.compile(
    r"\b(rather than|instead of|not|never|no longer|without)\s+(being\s+)?refus\w*",
    re.I,
)


def _asserts_refusal(sentence: str) -> bool:
    return bool(AFFIRMATIVE_REFUSAL.search(sentence)) and not DENIED_REFUSAL.search(
        sentence
    )


# 1. the bundle's three falsified claims are gone, and nothing else moved


def test_stale_claims_this_bundle_falsifies_are_gone() -> None:
    readme = _readme()
    for claim in STALE_CLAIMS:
        assert claim not in readme, (
            f"README.md still claims {claim!r}, which shipping `praxis doctor` "
            "and `praxis run` falsifies (criteria 1, 26)"
        )


def test_existing_usage_subsections_are_extended_not_replaced() -> None:
    usage = _usage_section()
    for heading in EXISTING_USAGE_HEADINGS:
        assert heading in usage, (
            f"T11 must add to the Usage section around the existing {heading!r} "
            "subsection, not remove or rename it"
        )


def test_existing_usage_content_is_left_in_place() -> None:
    usage = _usage_section()
    for command in EXISTING_EXECUTORS_COMMANDS:
        assert command in usage, (
            f"T11 must not drop the existing `{command}` example from the "
            "executors section"
        )
    assert "python -m praxis_dashboard --graph" in usage, (
        "T11 must not drop the dashboard section's runnable example"
    )
    for symbol in ("build_trivial_graph", "FakeExecutor", "TransitionError"):
        assert symbol in usage, (
            f"T11 must leave the Quickstart's library walkthrough ({symbol}) in place; "
            "`praxis run` is documented alongside it, not instead of it"
        )


# 2. the Quickstart names the shipped command that drives a graph to completion


def test_quickstart_names_praxis_run_as_the_shipped_way_to_drive_a_graph() -> None:
    (quickstart,) = [
        part for part in _usage_subsections() if part.startswith("### Quickstart")
    ]
    assert "`praxis run`" in quickstart, (
        "the Quickstart still presents driving a graph to completion as "
        "library-only; `praxis run` is now the shipped command that does it"
    )


# 3. main()'s dispatch gate now recognizes three tokens, not one


def test_readme_states_the_three_recognized_subcommands_and_the_version_fallback() -> None:
    usage = _usage_section()
    fallback = [
        sentence
        for sentence in _sentences(usage)
        if "version" in sentence and re.search(r"exit(s|\s+code)?\s+0", sentence, re.I)
    ]
    assert fallback, (
        "README.md's Usage section no longer states what `praxis` does with a "
        "token it does not recognize (print the package version, exit 0)"
    )
    for token in ("`executors`", "`doctor`", "`run`"):
        assert any(token in sentence for sentence in fallback), (
            f"the version-fallback statement must name {token} as a recognized "
            "first argument (criterion 1: the gate grew from one token to three)"
        )


# 4. `praxis doctor`


def test_doctor_command_shape_is_documented() -> None:
    doctor = _doctor_docs()
    assert "praxis doctor" in doctor
    for flag in ("--graph", "--overlay-manifest"):
        assert flag in doctor, (
            f"`praxis doctor`'s documentation must show its {flag} flag "
            "(criterion 6)"
        )


def test_doctor_five_checks_are_documented_in_the_order_they_run() -> None:
    prose = _prose_only(_doctor_docs())
    start = prose.find("praxis doctor")
    assert start != -1
    prose = prose[start:].lower()

    positions: list[int] = []
    for alternatives in DOCTOR_CHECKS_IN_ORDER:
        found = [prose.find(alt) for alt in alternatives if prose.find(alt) != -1]
        assert found, (
            "`praxis doctor`'s documentation names no check matching any of "
            f"{alternatives!r}; criterion 7 requires all five checks, in order"
        )
        positions.append(min(found))

    assert positions == sorted(positions), (
        "`praxis doctor`'s five checks must be documented in the order doctor "
        f"prints them; anchors were found at {positions}"
    )


def test_doctor_verdicts_and_exit_code_are_documented() -> None:
    doctor = _doctor_docs()
    for verdict in ("`ok`", "`warn`", "`fail`"):
        assert verdict in doctor, (
            f"`praxis doctor`'s documentation must state the {verdict} verdict "
            "every check ends with (criterion 7)"
        )
    # The two exit codes have to be tied to the fail/no-fail condition *in one
    # sentence*, and in the right direction. Searched for independently, "exits
    # 0" and "exits 1" are satisfied just as well by prose that swaps them.
    rules = [
        (sentence, EXIT_0.search(sentence), EXIT_NONZERO.search(sentence))
        for sentence in _sentences(_prose_only(doctor))
    ]
    rules = [
        (sentence, zero.start(), nonzero.start())
        for sentence, zero, nonzero in rules
        if zero and nonzero and "fail" in sentence.lower()
    ]
    assert rules, (
        "`praxis doctor`'s documentation must state in one sentence that it "
        "exits 0 when no check is `fail` and nonzero when at least one is "
        "(criterion 7); stating the two exit codes apart from the condition "
        "lets them be swapped without any assertion noticing"
    )
    for sentence, zero_at, nonzero_at in rules:
        ok_clause = _clause(
            sentence, zero_at, nonzero_at if nonzero_at > zero_at else None
        )
        fail_clause = _clause(
            sentence, nonzero_at, zero_at if zero_at > nonzero_at else None
        )
        assert NEGATION.search(ok_clause), (
            "exit 0 must be documented as the *absence* of a failing check "
            f"(criterion 7); the exit-0 clause reads {ok_clause!r}"
        )
        assert not NEGATION.search(fail_clause), (
            "the nonzero exit must be documented as at least one check failing, "
            f"not as the absence of one; the clause reads {fail_clause!r}"
        )


def test_doctor_warns_rather_than_fails_on_a_machine_with_no_executor_backends() -> None:
    doctor = _doctor_docs()
    relevant = [
        sentence
        for sentence in _sentences(doctor)
        if "ollama" in sentence.lower() and "claude" in sentence.lower()
    ]
    assert relevant, (
        "`praxis doctor`'s documentation must state what happens on a machine "
        "with no `claude` binary and no Ollama service (criterion 11)"
    )
    assert any("warn" in sentence for sentence in relevant), (
        "that machine must be documented as `warn`, never `fail`"
    )
    assert any(
        re.search(r"exit(s|\s+code)?\s+0", sentence, re.I) for sentence in relevant
    ), "that machine must be documented as still exiting 0"


def test_doctor_documents_that_no_user_configuration_file_exists_to_validate() -> None:
    doctor = _prose_only(_doctor_docs())
    assert re.search(r"no[^.\n]{0,60}configuration file", doctor, re.I), (
        "`praxis doctor`'s configuration check must state that no user "
        "configuration file exists to validate (criterion 9) -- it is a named "
        "gap, like `executors discover`'s `version: unknown`"
    )


def test_doctor_documents_that_it_never_scans_the_working_tree() -> None:
    doctor = _prose_only(_doctor_docs())
    relevant = [
        sentence
        for sentence in _sentences(doctor)
        if "working tree" in sentence.lower()
    ]
    assert relevant, (
        "`praxis doctor`'s graph/overlay check must state that doctor validates "
        "only the documents it is handed and never scans the working tree"
    )
    assert all(
        re.search(
            r"\b(never|not|no|does not|doesn'?t)\b(\s+\w+){0,2}\s+scans?\b",
            sentence,
            re.I,
        )
        for sentence in relevant
    ), (
        "the working-tree statement must be a negation -- prose saying doctor "
        "scans the working tree for documents is the inversion this guards "
        "against"
    )


# 5. `praxis run`


def test_run_command_shape_is_documented() -> None:
    run = _run_docs()
    for flag in ("--executor", "--capability", "--run-dir", "--run-id"):
        assert flag in run, (
            f"`praxis run`'s documentation must show its {flag} flag "
            "(criterion 13)"
        )
    assert re.search(r"default(s|ing)?[^.\n]{0,40}`auto`", run, re.I), (
        "`praxis run`'s documentation must state that `--executor` defaults to "
        "`auto` when omitted (criterion 13)"
    )


def test_run_documents_target_resolution() -> None:
    run = _run_docs()
    assert "graph" in run.lower(), (
        "`praxis run`'s documentation must say `<target>` may be a graph "
        "document path (criterion 14)"
    )
    assert "overlay" in run.lower(), (
        "`praxis run`'s documentation must say `<target>` may be a shipped "
        "overlay id (criterion 14)"
    )
    for overlay_id in ("`trivial`", "`development`"):
        assert overlay_id in run, (
            f"`praxis run`'s documentation must name the shipped overlay id "
            f"{overlay_id}; a reader cannot use the target otherwise "
            "(criterion 14)"
        )


def test_run_documents_that_explicit_selection_is_still_constrained() -> None:
    sentences = _sentences(_prose_only(_run_docs()))
    # Policy and the refusal have to meet in one sentence: searched for
    # separately, prose saying the explicit choice *bypasses* policy and is
    # silently overridden "rather than refused" satisfies both searches.
    policy = [
        sentence
        for sentence in sentences
        if "policy" in sentence.lower() and _asserts_refusal(sentence)
    ]
    assert policy, (
        "`praxis run`'s documentation must state, in one sentence, that an "
        "explicit `--executor` is still subject to policy and that a "
        "policy-denied executor is refused rather than silently overridden "
        "(criterion 18)"
    )
    assert not any(re.search(r"\bbypass", sentence, re.I) for sentence in policy), (
        "an explicit `--executor` must not be documented as bypassing policy "
        "(criterion 18)"
    )
    requirement = [
        sentence
        for sentence in sentences
        if "requirement" in sentence.lower()
        and re.search(r"\bstill\b|\bmust\b", sentence, re.I)
        and _asserts_refusal(sentence)
    ]
    assert requirement, (
        "`praxis run`'s documentation must state that an explicit `--executor` "
        "must still satisfy the node's requirement, and that the node is "
        "refused when it does not (criterion 18)"
    )


def test_run_documents_its_own_exit_code_direction() -> None:
    rules = [
        (sentence, EXIT_0.search(sentence), EXIT_NONZERO.search(sentence))
        for sentence in _sentences(_prose_only(_run_docs()))
    ]
    rules = [
        (sentence, zero.start(), nonzero.start())
        for sentence, zero, nonzero in rules
        if zero and nonzero
    ]
    assert rules, (
        "`praxis run`'s documentation must state in one sentence that it exits "
        "0 once every node reaches a terminal state and nonzero as soon as a "
        "node fails closed"
    )
    for sentence, zero_at, nonzero_at in rules:
        ok_clause = _clause(
            sentence, zero_at, nonzero_at if nonzero_at > zero_at else None
        )
        fail_clause = _clause(
            sentence, nonzero_at, zero_at if zero_at > nonzero_at else None
        )
        assert "fail" in fail_clause.lower(), (
            "the nonzero exit must be the one attached to a node failing "
            f"closed; the nonzero clause reads {fail_clause!r}"
        )
        assert "fail" not in ok_clause.lower(), (
            "exit 0 must be attached to completion, not to a node failing "
            f"closed; the exit-0 clause reads {ok_clause!r}"
        )


def test_run_documents_the_run_dir_contract() -> None:
    run = _prose_only(_run_docs())
    required = [
        sentence
        for sentence in _sentences(run)
        if "--run-dir" in sentence and "required" in sentence.lower()
    ]
    assert required, (
        "`praxis run`'s documentation must state that `--run-dir` is required "
        "(criterion 13)"
    )
    refused = [
        sentence
        for sentence in _sentences(run)
        if "run-state.json" in sentence and _asserts_refusal(sentence)
    ]
    assert refused, (
        "`praxis run`'s documentation must state that a `--run-dir` already "
        "holding a `run-state.json` is refused (criterion 22) -- prose saying "
        "such a directory is overwritten and 'never refused' is the inversion "
        "this guards against"
    )


def test_run_documents_that_its_run_dir_is_what_the_dashboard_reads() -> None:
    run = _run_docs()
    assert "praxis_dashboard" in run, (
        "`praxis run`'s documentation must say the directory it writes is "
        "exactly what `python -m praxis_dashboard --run-dir` reads "
        "(criterion 21)"
    )


# 6. the two limits a reader would otherwise assume


def test_readme_states_that_neither_command_has_json_output() -> None:
    usage = _prose_only(_usage_section())
    relevant = [
        sentence
        for sentence in _sentences(usage)
        if "`--json`" in sentence
        and "doctor" in sentence
        and "run" in sentence
        and NEGATION.search(sentence)
    ]
    assert relevant, (
        "the Usage section must state that `--json` is not available for "
        "`doctor` or `run`; `--json` stays bound to the `executors` status "
        "table. Naming the three tokens without a negation is satisfied by "
        "prose saying both commands accept it too"
    )


def test_readme_states_that_resuming_an_existing_run_is_not_supported() -> None:
    run = _prose_only(_run_docs())
    relevant = [
        sentence
        for sentence in _sentences(run)
        if re.search(r"resum", sentence, re.I)
        and re.search(r"\bno\b|\bnot\b|cannot|out of scope", sentence, re.I)
    ]
    assert relevant, (
        "`praxis run`'s documentation must state that resuming an existing run "
        "is out of scope, not left for a reader to assume"
    )
