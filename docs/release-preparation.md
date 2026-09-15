# Release Preparation Record

This document belongs to the release-preparation branch and records the source state being prepared. It does not alter the qualified source branch.

| Item | Value |
| --- | --- |
| Qualified source SHA | `c5c5e7937b5d1c7562a72d90d761cd630baf7369` |
| Qualified source branch | `redesign/praxis2` |
| Preparation branch | `release/praxis2-v1.0.0-rc.1` |
| Recommended release tag | `v2.0.0` |
| Current release denominator | 38 claims |
| Current release oracle score | 38 true negatives; 0 false positives; 0 false negatives |
| Historical oracle SHA | `7624ff5e2903219dc4d535d2871d7bbf9982c01549e336dfd0035890c6618c12` |

The release candidate must retain the qualified SHA in release notes and archive metadata. Release-specific version metadata, documentation, packaging scripts, and documentation tests may change on this branch. Runtime behavior changes outside release metadata require a new qualification review.
