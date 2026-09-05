"""Filesystem domain adapter: filesystem claims and glob-overlap detection for
resource-claim scheduling.

paths_overlap implements the same conservative directory-prefix rule as
develop's own footprint scheduler (see literal_prefix / prefixes_conflict in
~/.claude/skills/develop/runtime/schedule.py): the literal prefix of a glob
is everything before its first `*`/`**` wildcard, and two path patterns
overlap when one prefix equals, or is a parent/child of, the other. "*",
"**", and "." each resolve to the empty/root prefix and so overlap
everything.
"""

from __future__ import annotations

from praxis_runtime.resources.adapters.filesystem import (
    claims_from_footprints,
    filesystem_claim,
    paths_overlap,
)
from praxis_runtime.resources.claims import ResourceClaim, claims_conflict


def test_filesystem_claim_returns_resource_claim_with_expected_fields():
    claim = filesystem_claim("src/a/file.py", "write", scope="build")

    assert claim == ResourceClaim(
        resource_type="filesystem",
        identifier="src/a/file.py",
        access_mode="write",
        scope="build",
    )


def test_filesystem_claim_defaults_scope_to_none():
    claim = filesystem_claim("src/a/file.py", "read")

    assert claim.scope is None


def test_filesystem_claim_reflects_distinct_path_and_access_mode():
    claim = filesystem_claim("docs/other/README.md", "read")

    assert claim.identifier == "docs/other/README.md"
    assert claim.access_mode == "read"


def test_glob_and_child_file_overlap():
    assert paths_overlap("src/a/**", "src/a/file.py") is True


def test_sibling_directory_globs_do_not_overlap():
    assert paths_overlap("src/a/**", "src/b/**") is False


def test_double_star_wildcard_overlaps_unrelated_path():
    assert paths_overlap("**", "docs/unrelated/README.md") is True


def test_single_star_wildcard_overlaps_unrelated_path():
    assert paths_overlap("*", "docs/unrelated/README.md") is True


def test_dot_wildcard_overlaps_unrelated_path():
    assert paths_overlap(".", "docs/unrelated/README.md") is True


def test_claims_from_footprints_produces_overlap_matching_paths_overlap():
    claims = claims_from_footprints(
        {
            "T2": ["src/a/**", "src/c/**"],
            "T3": ["src/a/file.py", "src/b/**"],
        }
    )

    assert set(claims) == {"T2", "T3"}
    for node_claims in claims.values():
        assert all(isinstance(c, ResourceClaim) for c in node_claims)
        assert all(c.resource_type == "filesystem" for c in node_claims)
        assert all(c.access_mode == "write" for c in node_claims)

    overlapping_a, disjoint_a = claims["T2"]
    overlapping_b, disjoint_b = claims["T3"]

    assert paths_overlap(overlapping_a.identifier, overlapping_b.identifier) is True
    assert paths_overlap(disjoint_a.identifier, disjoint_b.identifier) is False

    # claims_conflict is the generic, resource-type-agnostic rule from
    # claims.py: it only conflicts on an exact identifier match (or a "*"
    # fallback), so it does not understand glob prefixes like "src/a/**".
    # It therefore disagrees with paths_overlap on the overlapping pair here
    # ("src/a/**" vs "src/a/file.py" are literally different identifiers) —
    # this divergence is expected and documented in filesystem.py; callers
    # that need filesystem-aware conflict detection must use paths_overlap,
    # not claims_conflict, for glob identifiers.
    assert claims_conflict(overlapping_a, overlapping_b) is False
    assert claims_conflict(disjoint_a, disjoint_b) is False


def test_claims_from_footprints_honors_non_default_access_mode():
    claims = claims_from_footprints({"T2": ["src/a/**"]}, access_mode="read")

    assert all(c.access_mode == "read" for c in claims["T2"])
