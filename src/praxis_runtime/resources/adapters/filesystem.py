"""Filesystem domain adapter: filesystem claims and glob-overlap detection for
resource-claim scheduling.

paths_overlap implements the same conservative directory-prefix rule as
develop's own footprint scheduler (see literal_prefix / prefixes_conflict in
~/.claude/skills/develop/runtime/schedule.py): the literal prefix of a glob
is everything before its first `*`/`**` wildcard, and two path patterns
overlap when one prefix equals, or is a parent/child of, the other. "*",
"**", and "." each resolve to the empty/root prefix and so overlap
everything.

paths_overlap is deliberately a different, filesystem-specific notion of
conflict from claims.claims_conflict: claims_conflict is generic and
resource-type-agnostic, and only treats two claims as conflicting when
their identifiers match exactly (or either is the workspace-wide "*"
fallback) -- it has no concept of glob prefixes, so "src/a/**" and
"src/a/file.py" are, to claims_conflict, simply two different identifiers
that do not conflict, even though paths_overlap correctly reports them as
overlapping. Callers that need to detect real filesystem contention between
glob-style footprints must use paths_overlap (or claims_from_footprints +
paths_overlap), not claims_conflict, for filesystem identifiers.
"""

from __future__ import annotations

from pathlib import PurePosixPath

from praxis_runtime.resources.claims import ResourceClaim

WILDCARDS = "*?"


def filesystem_claim(path: str, access_mode: str, *, scope: str | None = None) -> ResourceClaim:
    return ResourceClaim(
        resource_type="filesystem",
        identifier=path,
        access_mode=access_mode,
        scope=scope,
    )


def _literal_prefix(glob: str) -> str:
    """The directory (or file) part of a glob before its first wildcard.
    `src/domain/**` -> `src/domain`; `src/a*.ts` -> `src`; `README.md` ->
    `README.md`; `**/x` -> `` (the whole tree)."""
    glob = glob.strip().strip("/")
    cut = len(glob)
    for i, ch in enumerate(glob):
        if ch in WILDCARDS:
            cut = i
            break
    literal = glob[:cut]
    if cut < len(glob):
        literal = literal.rpartition("/")[0]  # wildcard mid-segment: back up to the directory
    return literal.strip("/")


def _prefixes_conflict(a: str, b: str) -> bool:
    if a == "" or b == "":
        return True
    pa, pb = PurePosixPath(a).parts, PurePosixPath(b).parts
    shorter = min(len(pa), len(pb))
    return pa[:shorter] == pb[:shorter]


def paths_overlap(pattern_a: str, pattern_b: str) -> bool:
    return _prefixes_conflict(_literal_prefix(pattern_a), _literal_prefix(pattern_b))


def claims_from_footprints(
    node_id_to_globs: dict[str, list[str]], access_mode: str = "write"
) -> dict[str, list[ResourceClaim]]:
    return {
        node_id: [filesystem_claim(glob, access_mode) for glob in globs]
        for node_id, globs in node_id_to_globs.items()
    }
