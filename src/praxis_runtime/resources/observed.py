"""Observed/touched-resource recording.

parse_observed_resources reads a node's "observed_resources" document --
expressed with the same resource-claim shape as a declared "resource_claims"
document (see schemas/v1/resource-claim.schema.json) -- and returns the
ResourceClaims a node's execution actually touched, as opposed to the claims
it declared ahead of time. TransitionEngine checks each one against the
node's declared claims via policy.authorize_access before a terminal
transition commits, so a node that touched an undeclared resource cannot
silently have that mutation committed under a STRICT policy.
"""

from __future__ import annotations

from praxis_runtime.resources.claims import ResourceClaim, parse_claims


def parse_observed_resources(document: dict) -> list[ResourceClaim]:
    return parse_claims(document)
