# ADR-048: Packages as the Universal Distribution Unit

- Status: Draft
- Date: 2026-09-13
- Related: ADR-003, ADR-004, ADR-013, ADR-020, ADR-021, ADR-025, ADR-041, ADR-046

## Context

Praxis distributes more than executable plugins. Reusable content includes graphs, persistent-agent definitions, domain models/templates, preference/profile seeds, client invocation contracts, documentation, and executable plugin providers.

If the ecosystem treats "plugin" as the universal packaging mechanism, pure graphs/agents would either require unnecessary executable code or become second-class artifacts with a separate lifecycle/catalog. That fragments trust, update, dependency, invocation, and distribution semantics.

## Decision

A versioned immutable **Praxis Package** SHALL be the universal distribution/install/update/rollback unit.

A package MAY contain any combination of:

- graph definitions;
- persistent-agent definitions and generation/bootstrap metadata;
- domain-specific model/spec/template assets;
- `InvocationContract`s;
- preference contracts/profile seeds;
- plugin executable/provider definitions;
- migrations compatible with package state rules;
- documentation and examples;
- dependencies on other packages.

A plugin is therefore an executable extension type carried by a package, not the package system itself.

## Package content manifest

The canonical manifest SHALL enumerate package contents by typed immutable references/digests. Content types SHALL be explicit so installation can determine which runtime subsystems participate without executing package code.

Conceptual example:

```yaml
package_id: praxis/research
version: 1.2.0
content_digest: sha256:...
contents:
  graphs:
    - id: praxis.research.default
      version: 2
      digest: sha256:...
  agents:
    - id: praxis.agent.researcher
      generation_template: 1
      digest: sha256:...
  plugins: []
  invocations:
    - id: praxis.research.invoke
  preferences:
    - research-defaults
```

Pure graph/agent packages SHALL require no native executable plugin.

## Catalog semantics

Catalog/distribution entries SHALL expose discoverable metadata for all package content classes, including:

- package ID/version/publisher/description/tags;
- graph IDs and user-facing entry points;
- agent definitions/personas/capability requests;
- plugin providers where present;
- dependency and compatibility ranges;
- requested capabilities/enforcement properties;
- crypto/signature/provenance data;
- behavioral profile metadata for recommendation/matching.

Discovery MAY filter by content class, e.g. graphs, agents, plugins, domain packages, or mixed packages.

## Graph deployment

Installing a graph-containing package SHALL:

1. resolve and verify the immutable package;
2. inspect dependencies/capability requests;
3. persist package generation;
4. register graph definitions in the graph registry;
5. register applicable invocation contracts;
6. activate according to policy;
7. expose the graph to clients/other graphs without rebuilding Praxis core.

A graph does not gain authority merely by installation.

## Agent deployment

Installing an agent-containing package registers an **agent definition/template**, not automatically a privileged running identity.

Creating/activating a persistent agent instance SHALL create a local governed identity/generation bound to the installed immutable agent definition and applicable user preferences/policies.

This distinction allows many users to install the same agent package while maintaining separate identities, memory, learned state, capability grants, and lineage.

Agent package updates SHALL NOT silently replace existing agent identity/history. Existing agents may migrate/adopt a new definition generation through governed lineage rules.

## Plugin deployment

Executable plugin contents follow SPEC-007 isolation, identity, protocol, capability, and supervision rules. Package verification/installation alone does not activate plugin authority.

## Composition and dependencies

Packages MAY depend on reusable graph/agent/plugin packages. Dependency locks pin immutable package versions/digests before execution. Transitive capabilities remain aggregated/reviewed under SPEC-011.

A domain package can therefore compose shared graphs without copying them.

## Distribution

GitHub Releases is the initial distribution adapter under ADR-046/SPEC-015. The same release/package format distributes pure graphs, agents, plugins, or mixed packages.

Future registries/catalogs SHALL consume the same canonical package format and trust semantics.

## Consequences

- graphs and agents become first-class installable/cataloged artifacts;
- pure declarative functionality does not require executable plugin code;
- one lifecycle covers discovery, install, update, rollback, uninstall, dependencies, signatures, permissions, and CLI registration;
- persistent agent identity remains local/governed rather than being conflated with a downloadable definition;
- catalog UX can recommend packages based on content and behavioral fit without changing core semantics.

## Acceptance direction

1. install a package containing only a graph; no executable plugin is required;
2. graph entry point becomes dynamically invocable after activation;
3. install a package containing an agent definition and instantiate two independent local agents from it;
4. updating the agent package does not overwrite either agent's memory/identity;
5. install a mixed graph+plugin package and enforce plugin isolation/capability rules independently;
6. catalog discovery can filter graph/agent/plugin content types;
7. uninstall removes active registrations while preserving historical run/agent lineage according to retention policy.
