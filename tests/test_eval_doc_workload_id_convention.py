"""`docs/eval.md` must document both `workload_id` citation forms (b-issue49 T7,
criterion 12).

The "The `workload_id` citation convention" paragraph fixes the citation
discipline ("an exact external workload/scenario identifier verbatim, never a
paraphrase") but once named only a `benchmark/corpus/*.md` filename as the
example. A live execution has no corpus file; the exact identifier it has is the
graph `node_id` the caller passed in, used unmodified.

Criterion 12 requires that second citation form to be stated in the same
paragraph, as the same rule rather than a documented exception. This is a
doc-content task with no other test file covering that paragraph's prose, so it
gets a dedicated doc-content test file, the same resolution
`tests/test_development_overlay_doc.py` applies to `docs/overlays/development.md`.
"""

from __future__ import annotations

import re
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent

CONVENTION_HEADING = "**The `workload_id` citation convention:**"


def _convention_paragraph() -> str:
    """Return the single paragraph documenting the workload_id convention."""
    text = (REPO_ROOT / "docs" / "eval.md").read_text()
    paragraphs = text.split("\n\n")
    matching = [p for p in paragraphs if CONVENTION_HEADING in p]

    assert matching, (
        "docs/eval.md must contain a paragraph introduced by "
        f"{CONVENTION_HEADING!r}"
    )
    assert len(matching) == 1, (
        "the workload_id citation convention must stay one rule in one place; "
        f"found {len(matching)} paragraphs introducing it"
    )
    return matching[0]


def test_convention_still_documents_the_corpus_citation_form():
    """The pre-existing benchmark-corpus form must survive the extension."""
    paragraph = _convention_paragraph()

    assert "benchmark/corpus" in paragraph, (
        "docs/eval.md must keep citing a benchmark/corpus/*.md filename as the "
        "corpus-scenario form of the workload_id convention"
    )
    assert "verbatim" in paragraph, (
        "docs/eval.md must keep requiring the identifier be cited verbatim"
    )


def test_convention_documents_the_live_execution_node_id_form():
    """A live execution cites the graph node_id, in the same paragraph."""
    paragraph = _convention_paragraph()

    assert "`node_id`" in paragraph, (
        "docs/eval.md's workload_id citation convention must state the second "
        "citation form: a live execution cites the graph `node_id` (criterion 12)"
    )
    assert re.search(r"live execution", paragraph, re.IGNORECASE), (
        "docs/eval.md must say the node_id form applies to a live execution, "
        "which has no corpus file"
    )


def test_node_id_citation_is_required_to_be_unmodified():
    """The node_id form carries the same exactness discipline, spelled out."""
    paragraph = _convention_paragraph()

    node_id_sentences = [
        sentence
        for sentence in re.split(r"(?<=\.)\s+", paragraph)
        if "`node_id`" in sentence
    ]
    assert node_id_sentences, (
        "docs/eval.md must have at least one sentence stating the `node_id` "
        "citation form"
    )

    joined = " ".join(node_id_sentences)
    assert re.search(r"unmodified|verbatim", joined, re.IGNORECASE), (
        "the `node_id` citation form must be required unmodified/verbatim, "
        "matching the corpus form's exactness discipline"
    )
    assert re.search(r"no prefix", joined, re.IGNORECASE), (
        "docs/eval.md must rule out prefixing, formatting or synthesizing the "
        "node_id string (criterion 12)"
    )


def test_live_execution_form_is_not_framed_as_an_exception():
    """Both forms are the same discipline, not a rule plus an escape hatch."""
    paragraph = _convention_paragraph()

    assert not re.search(r"\bexceptions?\b", paragraph, re.IGNORECASE), (
        "the live-execution citation form must be presented as the same rule, "
        "not as an exception to the workload_id convention"
    )
