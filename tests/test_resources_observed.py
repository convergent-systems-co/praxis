"""Tests for observed/touched-resource recording.

The issue's "observed/touched-resource recording" deliverable records the
resources a node's execution actually touched, distinct from the claims it
declared ahead of time in its "resource_claims" document, so a policy can
check the two against each other (see policy.authorize_access).
"""

from __future__ import annotations

import pytest

from praxis_contracts.validator import ContractValidationError
from praxis_runtime.resources.claims import AccessMode, ResourceClaim
from praxis_runtime.resources.observed import parse_observed_resources

DOCUMENT = {
    "spec_version": "1.0.0",
    "claims": [
        {
            "resource_type": "filesystem",
            "quantity": 1,
            "identifier": "/workspace/touched.txt",
            "access_mode": "write",
        }
    ],
}


def test_parse_observed_resources_returns_resource_claims():
    observed = parse_observed_resources(DOCUMENT)

    assert observed == [
        ResourceClaim(
            resource_type="filesystem",
            identifier="/workspace/touched.txt",
            access_mode=AccessMode.WRITE.value,
            quantity=1,
            scope=None,
        )
    ]


def test_parse_observed_resources_fails_closed_on_malformed_document():
    with pytest.raises(ContractValidationError):
        parse_observed_resources({"spec_version": "1.0.0", "claims": [{"quantity": 1}]})
