"""Core/overlay vocabulary boundary guard.

The nine core packages (`praxis_runtime`, `praxis_contracts`, `praxis_evidence`,
`praxis_executors`, `praxis_policy`, `praxis_eval`, `praxis_overlay`,
`praxis_dashboard`, `praxis_learning` -- the exact list the epic spec names as
"Praxis core packages") and the core ontology schemas (plus
`overlay-manifest.schema.json`) must stay domain-neutral: no
software-development/VCS-specific vocabulary (GitHub, pull requests, commits,
branches, TDD, code review, ...) may leak into them. That vocabulary belongs
in `src/overlays/development/`, the one concrete overlay that models the
software-delivery domain -- core has no idea any particular overlay exists.

This is a plain-text scan (source, docstrings, and comments), not an
AST/identifier-only check, because a leaked term in a comment is just as much
a boundary violation as one in an identifier.

Each forbidden pattern below was checked against the current tree before
being added, specifically to avoid colliding with legitimate core vocabulary
that merely looks similar -- e.g. bare "branch" is graph fan-out/fan-in
language in `praxis_runtime.transitions` (see `docs/runtime.md`), bare
"review" is generic evidence/policy language ("human review"), bare "release"
is `LeaseStore.release`, and bare "repository" appears in a self-referential
docstring aside. Those bare words are deliberately excluded or narrowed to a
more specific multi-word phrase (mirroring how "merge" itself -- unlike
"branch" -- turned out to have zero collisions and needed no narrowing).
"""

from __future__ import annotations

import json
import re
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent

CORE_PACKAGE_DIRS = (
    REPO_ROOT / "src" / "praxis_runtime",
    REPO_ROOT / "src" / "praxis_contracts",
    REPO_ROOT / "src" / "praxis_evidence",
    REPO_ROOT / "src" / "praxis_executors",
    REPO_ROOT / "src" / "praxis_policy",
    REPO_ROOT / "src" / "praxis_eval",
    REPO_ROOT / "src" / "praxis_overlay",
    REPO_ROOT / "src" / "praxis_dashboard",
    REPO_ROOT / "src" / "praxis_learning",
)

SCHEMAS_DIR = REPO_ROOT / "schemas" / "v1"

# name -> compiled, case-insensitive regex. Word-boundaried or multi-word
# where the bare term collides with legitimate core vocabulary (see module
# docstring); left as a plain substring where a current-tree sweep found no
# such collision.
FORBIDDEN_TERMS = {
    "github": r"github",
    "gitlab": r"gitlab",
    "bitbucket": r"bitbucket",
    "git": r"\bgit\b",
    "pull request": r"pull[ _-]request",
    "pr": r"\bpr\b",
    "code review": r"code[ -]review",
    "codereview": r"codereview",
    "issue tracker": r"issue[ -]tracker",
    "changelog": r"changelog",
    "tdd": r"\btdd\b",
    "commit": r"\bcommit\b",
    "merge": r"\bmerge\b",
    "git branch": r"git[ -]branch",
    "feature branch": r"feature[ -]branch",
    "release branch": r"release[ -]branch",
    "branch protection": r"branch[ -]protection",
    "git repository": r"git[ -]repository",
    "github repository": r"github[ -]repository",
    "source repository": r"source[ -]repository",
    "code repository": r"code[ -]repository",
    "developer": r"\bdeveloper\b",
    "software development": r"software[ -]development",
    "vcs": r"\bvcs\b",
    "version control": r"version[ -]control",
    "ci/cd": r"ci/cd",
    "worktree": r"\bworktree\b",
    "rebase": r"\brebase\b",
    "checkout": r"\bcheckout\b",
    "stash": r"\bstash\b",
    "hotfix": r"\bhotfix\b",
    "mainline": r"\bmainline\b",
    "master branch": r"master[ -]branch",
    "main branch": r"main[ -]branch",
}

_COMPILED_TERMS = {name: re.compile(pattern, re.IGNORECASE) for name, pattern in FORBIDDEN_TERMS.items()}


def _iter_core_py_files():
    for package_dir in CORE_PACKAGE_DIRS:
        yield from sorted(package_dir.rglob("*.py"))


def _scan_text_for_violations(path: Path, lines: list[str]) -> list[str]:
    violations = []
    for line_number, line in enumerate(lines, start=1):
        for term_name, pattern in _COMPILED_TERMS.items():
            match = pattern.search(line)
            if match:
                violations.append(
                    f"{path.relative_to(REPO_ROOT)}:{line_number}: forbidden term "
                    f"{term_name!r} (matched {match.group(0)!r}) in: {line.strip()!r}"
                )
    return violations


def test_core_packages_contain_no_forbidden_development_vocabulary():
    violations: list[str] = []
    for py_file in _iter_core_py_files():
        lines = py_file.read_text(encoding="utf-8").splitlines()
        violations.extend(_scan_text_for_violations(py_file, lines))

    assert not violations, (
        "core packages must stay domain-neutral -- found software-development/"
        "VCS vocabulary that belongs in an overlay, not core:\n" + "\n".join(violations)
    )


def _iter_description_strings(document, path: str = "$"):
    if isinstance(document, dict):
        for key, value in document.items():
            child_path = f"{path}.{key}"
            if key == "description" and isinstance(value, str):
                yield child_path, value
            else:
                yield from _iter_description_strings(value, child_path)
    elif isinstance(document, list):
        for index, item in enumerate(document):
            yield from _iter_description_strings(item, f"{path}[{index}]")


def test_core_schema_descriptions_contain_no_forbidden_development_vocabulary():
    violations: list[str] = []
    for schema_path in sorted(SCHEMAS_DIR.glob("*.json")):
        document = json.loads(schema_path.read_text(encoding="utf-8"))
        for json_path, description in _iter_description_strings(document):
            for term_name, pattern in _COMPILED_TERMS.items():
                match = pattern.search(description)
                if match:
                    violations.append(
                        f"{schema_path.relative_to(REPO_ROOT)} ({json_path}): forbidden "
                        f"term {term_name!r} (matched {match.group(0)!r}) in description: "
                        f"{description!r}"
                    )

    assert not violations, (
        "core schema descriptions must stay domain-neutral -- found "
        "software-development/VCS vocabulary that belongs in an overlay, not "
        "core:\n" + "\n".join(violations)
    )


def test_forbidden_term_list_is_nonempty():
    """Guards against the scan silently checking nothing."""
    assert len(FORBIDDEN_TERMS) > 10


def test_core_package_dirs_exist():
    """Guards against a typo'd path silently scanning zero files."""
    for package_dir in CORE_PACKAGE_DIRS:
        assert package_dir.is_dir(), f"expected core package dir {package_dir} to exist"
    assert SCHEMAS_DIR.is_dir()
