# ADR-040: Untrusted Content and Instruction/Data Separation

- Status: Draft
- Date: 2026-09-13

## Context

Praxis deliberately consumes content controlled by repositories, packages, tools, remote systems, users, and other agents. That content can contain text that looks like instructions to an LLM. Workspace Intelligence makes this risk more important because it deliberately selects repository evidence and places some of it into model context.

Deterministic enforcement below the LLM prevents unauthorized side effects, but it does not by itself prevent prompt injection from corrupting reasoning, poisoning learned state, manipulating retrieval, or causing the model to request permitted-but-unintended actions.

Praxis therefore needs an explicit architectural distinction between authority-bearing control data and untrusted evidence.

## Decision

Praxis SHALL treat externally sourced content as **data, never authority**, unless it has been promoted through a deterministic authority mechanism defined by a canonical contract.

No natural-language content can promote itself into policy, permission, approval, graph definition, preference, durable memory, or executable instruction merely because an LLM interprets it that way.

### Trust classes

Canonical envelopes SHALL carry provenance and a trust class sufficient to distinguish at least:

- `control`: Praxis-generated canonical control structures validated against an authoritative contract;
- `human_authority`: authenticated explicit human decisions bound to a defined scope;
- `package_authority`: installed and authorized package/graph configuration within its granted capabilities;
- `observation`: tool/client/runtime observations that have not been promoted;
- `evidence`: source files, documents, search results, issue text, web/tool output, logs, comments, test output, and similar task data;
- `derived`: summaries, embeddings, inferred relationships, model output, learned candidates, and other non-authoritative transformations.

Trust class and provenance SHALL survive retrieval, context packing, event emission, caching, synchronization, and cross-agent transfer.

### Instruction/data separation

Repository text, comments, documentation, issue bodies, tool output, generated files, retrieved web content, and other evidence SHALL NOT be concatenated into an authoritative instruction channel.

Adapters and inference executors SHALL preserve a structural distinction between system/control instructions and evidence whenever the target model/client supports such separation. Where a client cannot provide structural separation, Praxis SHALL label and delimit evidence explicitly and SHALL NOT claim prompt-injection resistance from formatting alone.

### Authority non-escalation

Untrusted or derived content cannot:

- grant or broaden a capability;
- satisfy an approval requirement;
- alter an enforcement profile;
- install or trust a package;
- modify policy;
- directly promote learned behavior;
- create human-confirmed preferences;
- convert observations into facts solely through repetition;
- instruct Praxis to ignore provenance or security rules.

Any such requested transition must pass through the canonical deterministic command/authority path.

### Learning and memory poisoning

Evidence and model output MAY create learning or memory candidates, but promotion SHALL preserve source provenance and follow the execution/learning/governance separation.

Repeated copies of the same underlying source SHALL NOT be treated as independent corroboration when provenance indicates common ancestry or duplication.

Security-sensitive learned behavior requires stronger promotion evidence than ordinary convenience preferences and cannot weaken deterministic controls.

### Retrieval and context packs

Workspace/context retrieval SHALL carry source identity, version/digest, trust class, and evidence type. Context-pack ranking SHALL NOT increase authority based on semantic similarity, retrieval score, repetition, or model confidence.

Known instruction-like content inside evidence MAY be flagged for telemetry or risk scoring, but detection is defense-in-depth rather than the enforcement mechanism.

### External tool output

Tool output is untrusted evidence even when the tool itself is trusted to execute. A trusted compiler can emit attacker-controlled source text; a trusted Git client can retrieve attacker-controlled commit messages. Tool identity and output-content trust are separate dimensions.

## Consequences

Praxis can reason over hostile repositories and documents without confusing their contents with system authority. Prompt injection may still influence model reasoning, but it cannot directly cross deterministic authority boundaries or silently poison authoritative state.

The architecture requires provenance-aware envelopes and careful context construction, but these are consistent with Praxis's existing evidence, event, and governance model.

## Security invariant

**Content may influence a proposal; content may not authorize the proposal.**

## Non-goals

This ADR does not claim that prompt injection can be perfectly detected or prevented at the model reasoning layer. It prevents untrusted content from becoming authority and limits the consequences of model manipulation through deterministic enforcement and provenance.