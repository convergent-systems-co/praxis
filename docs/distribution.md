# Distribution Surface

This document states plainly where Praxis is developed versus where it is distributed, so that no
part of this repository or its bundles is mistaken for a publishing step.

## `ai-atoms` remains the catalog/distribution surface

[`ai-atoms`](https://github.com/convergent-systems-co/ai-atoms) is the catalog and distribution
surface for skills, including the `develop` skill that this bundle's compatibility plan
(`docs/overlays/development-compat.md`) concerns itself with. `ai-atoms` is where skills are
packaged and made available for consumption; it is not where Praxis itself is built.

Praxis — the runtime, contracts, executors, evidence, policy, and overlay mechanism — is developed
directly in this repository, `convergent-systems-co/praxis`, rather than inside `ai-atoms`.

## Practical implication

- Nothing in this bundle moves or duplicates `ai-atoms`-hosted skill content into this repository.
- Nothing in this repository is published to `ai-atoms` as a side effect of this bundle.

Any future publication of a Praxis bundle/descriptor into `ai-atoms` is a separate, deliberate
step, not an automatic consequence of work landing here.
