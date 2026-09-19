package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type cliHelpSpec struct {
	usage       string
	description string
	options     string
}

// The help catalog is the command-dispatcher's contract. Command handlers own
// execution and validation, but they do not own discovery or help semantics.
var cliHelpCatalog = map[string]cliHelpSpec{
	"":                                      {"usage: praxis <command|installed-entry-point> [arguments] [options]", "Praxis control plane and installed package entry points.", "Commands: discover, info, install, update, rollback, disable, uninstall, list, status, resume, cancel, supervise, providers, doctor, key-bootstrap, state-init, migration, authority, publisher, version, help"},
	"status":                                {"usage: praxis status <run-id> [--db <path>]", "Inspect one durable run without mutation.", "--db <path>  Explicit Praxis database (or PRAXIS_DB)"},
	"discover":                              {"usage: praxis discover [options]", "Discover package or installed entry-point information.", "See package discovery options."},
	"info":                                  {"usage: praxis info [options]", "Inspect package information.", "See package information options."},
	"install":                               {"usage: praxis install <owner/repo[@tag]> [--allow-classical-signature-fallback]", "Install a verified package through the governed package lifecycle.", "<owner/repo[@tag]> is required. Optional: --allow-classical-signature-fallback. Selectors: PRAXIS_DB, PRAXIS_TRUSTED_KEYS, and PRAXIS_PACKAGE_APPROVAL_ID; GITHUB_TOKEN is optional for GitHub transport."},
	"update":                                {"usage: praxis update <package-ref> [options]", "Update an installed package through the governed lifecycle.", "Package reference and approval options are required by policy."},
	"rollback":                              {"usage: praxis rollback <package-id> [options]", "Rollback a package through the governed lifecycle.", "Approval and authority options are required by policy."},
	"disable":                               {"usage: praxis disable <package-id> [options]", "Disable an installed package without deleting lineage.", "Approval and authority options are required by policy."},
	"uninstall":                             {"usage: praxis uninstall <package-id> [options]", "Uninstall an installed package through the governed lifecycle.", "Approval and authority options are required by policy."},
	"list":                                  {"usage: praxis list", "List installed package state.", "No options."},
	"doctor":                                {"usage: praxis doctor", "Run read-only installation and runtime diagnostics.", "No options."},
	"key-bootstrap":                         {"usage: praxis key-bootstrap --provider <provider> --key-id <id> --owner <owner> --purpose <purpose> --profile <profile> --output <path>", "Create protected first-installation bootstrap metadata.", "All listed options are required."},
	"state-init":                            {"usage: praxis state-init", "Initialize a new Praxis state store; existing stores require governed migration.", "No options."},
	"version":                               {"usage: praxis version", "Print the Praxis control-plane version.", "No options."},
	"resume":                                {"usage: praxis resume <run-id> [options]", "Resume a governed suspended run.", "Run-control options are required by policy."},
	"cancel":                                {"usage: praxis cancel <run-id> [options]", "Cancel a governed run.", "Run-control options are required by policy."},
	"providers":                             {"usage: praxis providers", "List the goal-drive provider catalog: first-party subscription profiles (available when their CLI is on PATH) and the environment command worker, with the exact --provider option each accepts.", "No options. goal-drive requires an explicit --provider; Praxis never selects one implicitly."},
	"supervise":                             {"usage: praxis supervise <observe|comment|correction|constraint|suspend|cancel|resume|reconcile|materialize> [options]", "Observe or intervene in supervised execution. observe works at invocation scope (--goal-id, --goal-version, --invocation-id) and discovers the invocation's turns; --turn-id narrows to one turn. Interventions require the exact --turn-id.", "--goal-id, --goal-version, --invocation-id  Exact execution identity (required)\n  --turn-id  Exact turn (optional for observe, required for interventions)\n  --follow  Keep streaming durable activity until the newest turn ends\n  --after <version>  Resume from a stream version\n  --text  Intervention text; --actor-id/--actor-kind or PRAXIS_ACTOR_ID/KIND for interventions"},
	"lifecycle-recover":                     {"usage: praxis lifecycle-recover <status|storage-schema|runtime-state> --installation <id> [options]", "Reserved control-plane lifecycle_transition recovery entry point.", "Transitions require --plan, --storage-authority-request, and --runtime-authority-request. --db <path> or PRAXIS_DB; --actor-id/--actor-kind or PRAXIS_ACTOR_ID/PRAXIS_ACTOR_KIND."},
	"supervise materialize":                 {"usage: praxis supervise materialize --goal-id <id> --goal-version <version> --invocation-id <invocation> --turn-id <turn>", "Deterministically re-materialize the unit completion a qualified, published historical turn earned but never recorded (#164). Not settlement: no judgment, no new evidence, the historical turn is unchanged.", "Requires the turn's own qualification (validated progress, published checkpoint, declared validation passed), the exact checkpoint still on the published branch, and a proposal naming the selected unit; refuses ambiguity, conflict, a changed consequence, and an already complete unit."},
	"supervise observe":                     {"usage: praxis supervise observe [options]", "Read live and durable supervision activity.", "--goal-id, --invocation-id, --turn-id, --after, --follow"},
	"supervise comment":                     {"usage: praxis supervise comment --text <text> [options]", "Record a human supervision comment.", "--text <text> and the governed target are required."},
	"supervise correction":                  {"usage: praxis supervise correction --text <text> [options]", "Record a human supervision correction.", "--text <text> and the governed target are required."},
	"supervise constraint":                  {"usage: praxis supervise constraint --text <text> [options]", "Record a human supervision constraint.", "--text <text> and the governed target are required."},
	"supervise suspend":                     {"usage: praxis supervise suspend [options]", "Request suspension of supervised execution.", "The governed target is required."},
	"supervise cancel":                      {"usage: praxis supervise cancel [options]", "Request cancellation of supervised execution.", "The governed target is required."},
	"supervise resume":                      {"usage: praxis supervise resume [options]", "Request resumption of supervised execution.", "The governed target is required."},
	"migration":                             {"usage: praxis migration <status|preview|execute|recover> [options]", "Inspect or execute an explicit governed schema migration.", "Migration execution requires an exact system-produced preview and owner confirmation."},
	"migration status":                      {"usage: praxis migration status", "Show migration state without mutation.", "No options."},
	"migration preview":                     {"usage: praxis migration preview [--output <file>]", "Produce an exact read-only migration plan.", "--output <file> writes the exact system-produced preview artifact."},
	"migration execute":                     {"usage: praxis migration execute --preview-file <path>", "Apply one exact owner-authorized migration plan.", "--preview-file <path> is required."},
	"migration recover":                     {"usage: praxis migration recover", "Inspect or recover an interrupted migration.", "No options."},
	"authority":                             {"usage: praxis authority <bootstrap|delegate|request-inspect|model-preview|model-adopt|model-abandon|model-status|package-deploy-preview|package-deploy-proposal|package-deploy-review|package-deploy-request|package-deploy-intent-preview|package-deploy-intent-request|package-deploy-approve|root-successor-preview|root-successor-proposal|root-successor-review|root-successor-accept|installation-repair-request|installation-repair-approve> [options]", "Manage bounded installation authority, immutable root succession, and model transitions.", "Each subcommand has its own governed inputs and confirmation."},
	"authority bootstrap":                   {"usage: praxis authority bootstrap --scope <scope>", "Enroll the one installation governance root.", "--scope <scope> is required; interactive owner confirmation is required."},
	"authority delegate":                    {"usage: praxis authority delegate (--request <digest> | --request-file <path>)", "Decide one exact bounded authority delegation request.", "--request <digest> is the canonical durable path; --request-file is restricted to supported legacy producers."},
	"authority root-successor-preview":      {"usage: praxis authority root-successor-preview [--from-historical-root] [--output <file>]", "Derive the exact immutable root successor without mutation: the ADR-089 repair-bearing successor of the current root, or with --from-historical-root the ADR-090 current canonical successor of a historical schema-11 enrollment root.", "--output writes the system-produced exact preview."},
	"authority root-successor-proposal":     {"usage: praxis authority root-successor-proposal --preview-file <file>", "Persist one exact root-succession proposal.", "The preview must be system-produced and still name the active predecessor."},
	"authority root-successor-review":       {"usage: praxis authority root-successor-review --proposal <digest>", "Record independent authenticated-owner review of one exact proposal.", "Interactive exact confirmation is required."},
	"authority root-successor-accept":       {"usage: praxis authority root-successor-accept --proposal <digest> --review <digest>", "Atomically persist the accepted successor and predecessor supersession.", "Interactive exact confirmation is required."},
	"authority installation-repair-request": {"usage: praxis authority installation-repair-request --operation <storage_schema|runtime_state> --expires-at <RFC3339>", "Create one independently scoped repair authority request.", "The active successor and its durable decision are revalidated."},
	"authority installation-repair-approve": {"usage: praxis authority installation-repair-approve --request <digest>", "Approve one exact durable repair authority request.", "Interactive exact confirmation is required."},
	"authority pending":                     {"usage: praxis authority pending [--goal-id <id> --goal-version <version>] [--all]", "List durable authority requests with their truthful disposition and the exact digest each decision must name.", "--goal-id/--goal-version  Restrict to one exact Goal generation\n  --all  Include decided requests"},
	"authority decide":                      {"usage: praxis authority decide --request <digest> --outcome approve|reject [--reason <text>]", "Record the installation owner's decision on one exact pending authority request. Praxis derives the request digest, current root, granted scope, and model identity from durable state; the owner supplies only the outcome and types the printed confirmation.", "--request <digest>  Exact durable AuthorityRequest digest (from `praxis authority pending`)\n  --outcome approve|reject  The human decision\n  --reason <text>  Optional reason"},
	"authority request-inspect":             {"usage: praxis authority request-inspect --request <digest>", "Inspect one canonical AuthorityRequest without mutation.", "--request <digest> is required."},
	"authority model-preview":               {"usage: praxis authority model-preview [--output <file>]", "Preview the exact built-in authority-model successor.", "--output <file> writes the exact system-produced preview artifact; no caller-supplied model identity is accepted."},
	"authority model-adopt":                 {"usage: praxis authority model-adopt --preview-file <file>", "Adopt the exact owner-confirmed authority-model successor.", "--preview-file <file> is required and must identify a system-produced preview; owner confirmation is required."},
	"authority model-abandon":               {"usage: praxis authority model-abandon --adoption <digest>", "Abandon one exact incomplete authority-model adoption attempt.", "--adoption <digest> is required; owner confirmation is required and committed adoptions cannot be abandoned."},
	"authority model-status":                {"usage: praxis authority model-status", "Show the active authority model without mutation.", "No options."},
	"authority package-deploy-preview":      {"usage: praxis authority package-deploy-preview --expires-at <RFC3339> [--output <file>]", "Preview installation-bound package-manager deployment authority.", "The preview is system-produced and read-only."},
	"authority package-deploy-proposal":     {"usage: praxis authority package-deploy-proposal --preview-file <file>", "Persist one exact package-manager deployment proposal.", "The preview file must be system-produced."},
	"authority package-deploy-review":       {"usage: praxis authority package-deploy-review --proposal-digest <digest>", "Review one exact package-manager deployment proposal.", "Owner confirmation is required."},
	"authority package-deploy-request":      {"usage: praxis authority package-deploy-request --proposal-digest <digest> --review-digest <digest>", "Produce the canonical PACKAGE_DEPLOY AuthorityRequest.", "Both digests must be durable system-produced references."},
	"authority package-deploy-approve":      {"usage: praxis authority package-deploy-approve --request <digest>", "Approve one exact verified package deployment request and derive its installer binding.", "Owner confirmation is required; the exact intent and evidence are displayed and revalidated."},
	"authority package-deploy-intent-preview": {"usage: praxis authority package-deploy-intent-preview <owner/repo[@tag]> [--output <file>]", "Verify an immutable package closure and persist exact evidence for deployment governance.", "This does not install or grant authority."},
	"authority package-deploy-intent-request": {"usage: praxis authority package-deploy-intent-request --preview-file <file>", "Persist the canonical exact package.deploy request from a verified intent preview.", "The preview and durable verification evidence must match exactly."},
	"publisher":                                                {"usage: praxis publisher <key-create|key-inspect|enroll-preview|enroll-approve|enroll-approval-inspect|enroll|authority-preview|authority-proposal|authority-review|authority-request|package-build|sign-preview|sign|receipt|goals-publication-prepare|goals-publication-execute|goals-publication-inspect|goals-publication-reconcile|goals-publication-abandon-preview|goals-publication-abandon|goals-publication-recovery-prepare|goals-publication-recovery-failed-publication-prepare|goals-publication-recovery-execute|goals-publication-recovery-reconcile|goals-publication-recovery-resolve-observation> [options]", "Manage first-party publisher keys, enrollment, authority, package artifacts, and signing.", "All identities and digests used for governed transitions are system-produced."},
	"publisher key-create":                                     {"usage: praxis publisher key-create --key-id <protected-key-reference>", "Create a protected Ed25519 publisher-signing key.", "--key-id <protected-key-reference> is required; private material is never exported."},
	"publisher key-inspect":                                    {"usage: praxis publisher key-inspect --key-id <protected-key-reference>", "Inspect protected publisher-key metadata.", "--key-id <protected-key-reference> is required."},
	"publisher enroll-preview":                                 {"usage: praxis publisher enroll-preview --key-id <protected-key> --generation <generation> [--predecessor <digest>] [--namespace <namespace>] [--output <file>]", "Preview publisher-generation enrollment.", "The preview is read-only and produces the exact approval digest."},
	"publisher enroll-approve":                                 {"usage: praxis publisher enroll-approve --preview-file <file>", "Approve one exact system-produced enrollment preview.", "--preview-file <file> is required; owner confirmation is required."},
	"publisher enroll-approval-inspect":                        {"usage: praxis publisher enroll-approval-inspect --preview-digest <digest>", "Inspect the approval bound to one enrollment preview.", "--preview-digest <digest> is required."},
	"publisher enroll":                                         {"usage: praxis publisher enroll --approval <digest>", "Consume one exact canonical enrollment approval.", "--approval <digest> is required; caller generation, timestamp, and key fields are not accepted."},
	"publisher authority-preview":                              {"usage: praxis publisher authority-preview --publisher-generation-digest <digest> --namespace <namespace> --expires-at <RFC3339> [--output <file>]", "Preview bounded package.publish authority.", "The preview is read-only; --output writes the exact system-produced proposal artifact."},
	"publisher authority-proposal":                             {"usage: praxis publisher authority-proposal --preview-file <file>", "Persist one exact package.publish proposal.", "--preview-file <file> is required."},
	"publisher authority-review":                               {"usage: praxis publisher authority-review --proposal-digest <digest>", "Review one exact package.publish proposal.", "--proposal-digest <digest> is required; owner confirmation is required."},
	"publisher authority-request":                              {"usage: praxis publisher authority-request --proposal-digest <digest> --review-digest <digest>", "Produce the canonical AuthorityRequest from durable proposal and review lineage.", "Both digests must be system-produced references."},
	"publisher package-build":                                  {"usage: praxis publisher package-build --executable <path> --output-dir <dir> --source-identity <commit/tree>", "Build deterministic unsigned package artifacts.", "All listed options are required."},
	"publisher sign-preview":                                   {"usage: praxis publisher sign-preview --package-dir <dir> --generation-digest <digest> --source-identity <commit/tree>", "Create a read-only signing preview bound to exact package and authority state.", "Package, generation, and source identities are required."},
	"publisher sign":                                           {"usage: praxis publisher sign --package-dir <dir> --preview <signing-preview-digest>", "Sign the exact owner-authorized signing preview.", "Both options are required; the protected signer is used."},
	"publisher goals-publication-prepare":                      {"usage: praxis publisher goals-publication-prepare --package-dir <dir> --expires-at <RFC3339>", "Persist the exact signed Goals initial-publication request; does not grant authority or mutate GitHub.", "Requires the explicitly adopted publication model and exact existing signed assets."},
	"publisher goals-publication-execute":                      {"usage: praxis publisher goals-publication-execute --package-dir <dir> --request-id <id>", "Execute or resume the one exact authorized Goals publication.", "Requires durable bounded delegation; unknown outcomes fail closed without redispatch."},
	"publisher goals-publication-reconcile":                    {"usage: praxis publisher goals-publication-reconcile --request-id <id>", "Read remote state and append local reconciliation evidence for an uncertain effect.", "Never retries a mutation or adopts matching objects; available after authority loss."},
	"publisher goals-publication-inspect":                      {"usage: praxis publisher goals-publication-inspect --request-id <id>", "Inspect local publication effects without mutation.", "No authority or publication changes."},
	"publisher goals-publication-abandon-preview":              {"usage: praxis publisher goals-publication-abandon-preview --request-id <id> --reason <text> --output <new-file>", "Freeze one exact Goals predecessor abandonment payload for owner review.", "Writes immutable exact payload bytes; no Praxis state is changed."},
	"publisher goals-publication-abandon":                      {"usage: praxis publisher goals-publication-abandon --preview-file <frozen-payload> --confirmation 'ABANDON <digest>'", "Terminally abandon one exact Goals publication execution under owner governance.", "Revalidates current lineage and appends only the exact confirmed payload."},
	"publisher goals-publication-recovery-prepare":             {"usage: praxis publisher goals-publication-recovery-prepare --package-dir <dir> --predecessor-request-id <id> --expires-at <RFC3339>", "Prepare a new exact-state successor request for the abandoned Goals publication.", "Read-back checks the existing commit, refs, draft release, empty inventory and signed bytes; creates no external mutation or authority."},
	"publisher goals-publication-recovery-chain-prepare":       {"usage: praxis publisher goals-publication-recovery-chain-prepare --package-dir <dir> --prior-recovery-request-id <id> --expires-at <RFC3339>", "Prepare the bounded next successor after an abandoned recovery.", "Binds the complete two-generation lineage and creates no authority or external mutation."},
	"publisher goals-publication-recovery-ordered-prepare":     {"usage: praxis publisher goals-publication-recovery-ordered-prepare --package-dir <dir> --latest-recovery-request-id <id> --expires-at <RFC3339>", "Prepare the ordered Goals recovery successor from all abandoned generations.", "Validates the complete causal chain and creates no authority or external mutation."},
	"publisher goals-publication-recovery-execute":             {"usage: praxis publisher goals-publication-recovery-execute --package-dir <dir> --request-id <id>", "Execute only the freshly authorized Goals successor effects.", "Requires active v5 and the exact successor grant; unresolved outcomes are never retried."},
	"publisher goals-publication-recovery-reconcile":           {"usage: praxis publisher goals-publication-recovery-reconcile --request-id <id>", "Read the exact successor release and append unresolved reconciliation evidence.", "Never retries a mutation or treats matching state as attribution."},
	"publisher goals-publication-recovery-resolve-observation": {"usage: praxis publisher goals-publication-recovery-resolve-observation --request-id <id> --effect-id <id> [--confirmation 'RESOLVE <digest>']", "Resolve one validated UNKNOWN observational effect without redispatch.", "Without confirmation this emits a stable challenge and writes nothing; confirmation requires active authority."},
	"publisher receipt":                                        {"usage: praxis publisher receipt --digest <provenance-digest>", "Inspect signing provenance without mutation.", "--digest <provenance-digest> is required."},
}

// lookupInstalledEntryPointHelp resolves help for an installed package entry
// point from the same active invocation registry the dynamic dispatcher
// resolves commands from. It is a variable so qualification can point it at
// a fixture installation.
var lookupInstalledEntryPointHelp = installedEntryPointHelpFromRegistry

func dispatchCLIHelp(args []string, out io.Writer) (bool, error) {
	path, requested := helpPath(args)
	if !requested {
		return false, nil
	}
	key := strings.Join(path, " ")
	spec, ok := cliHelpCatalog[key]
	if ok && key == "" {
		spec.options += installedEntryPointSummary()
	}
	if !ok && len(path) == 1 {
		if installed, found := lookupInstalledEntryPointHelp(path[0]); found {
			return true, writeCLIHelp(out, installed)
		}
	}
	if !ok {
		parent := path
		for len(parent) > 0 {
			parent = parent[:len(parent)-1]
			if candidate, exists := cliHelpCatalog[strings.Join(parent, " ")]; exists {
				_ = writeCLIHelp(out, candidate)
				return true, fmt.Errorf("unknown command %q; see %s", key, candidate.usage)
			}
		}
		return true, errors.New("unknown command " + key)
	}
	return true, writeCLIHelp(out, spec)
}

func helpPath(args []string) ([]string, bool) {
	if len(args) == 0 {
		return nil, true
	}
	if args[0] == "help" {
		path := args[1:]
		for _, arg := range path {
			if arg == "--help" || arg == "-h" {
				continue
			}
		}
		return path, true
	}
	for i, arg := range args {
		if arg == "--help" || arg == "-h" {
			return args[:i], true
		}
	}
	return nil, false
}

func writeCLIHelp(out io.Writer, spec cliHelpSpec) error {
	if _, err := fmt.Fprintln(out, spec.usage); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "\nDescription:\n  "+spec.description); err != nil {
		return err
	}
	_, err := fmt.Fprintln(out, "\nOptions:\n  "+spec.options)
	return err
}

// installedEntryPointHelpFromRegistry renders help for one installed entry
// point (by alias or entry-point id) from the active invocation registry.
// Discovery grants no execution authority: the contract is read from the
// installation's durable registry, never from a manifest on disk.
func installedEntryPointHelpFromRegistry(name string) (cliHelpSpec, bool) {
	contracts, err := installedInvocationContracts()
	if err != nil {
		return cliHelpSpec{}, false
	}
	for _, contract := range contracts {
		matched := contract.EntryPointID == name
		for _, alias := range contract.Aliases {
			if alias == name {
				matched = true
			}
		}
		if !matched {
			continue
		}
		return invocationContractHelp(name, contract), true
	}
	return cliHelpSpec{}, false
}

func installedInvocationContracts() ([]contracts.InvocationContract, error) {
	path := os.Getenv("PRAXIS_DB")
	if path == "" {
		return nil, errors.New("PRAXIS_DB is not set")
	}
	db, err := state.OpenSQLiteReadOnly(context.Background(), path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	items, err := state.New(db).ActiveInvocations(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]contracts.InvocationContract, 0, len(items))
	for _, item := range items {
		out = append(out, item.Contract)
	}
	return out, nil
}

func installedEntryPointSummary() string {
	items, err := installedInvocationContracts()
	if err != nil {
		return "\n  Installed entry points: unavailable (" + err.Error() + ")"
	}
	aliases := make([]string, 0)
	for _, contract := range items {
		aliases = append(aliases, contract.Aliases...)
	}
	sort.Strings(aliases)
	if len(aliases) == 0 {
		return "\n  Installed entry points: none"
	}
	return "\n  Installed entry points: " + strings.Join(aliases, ", ")
}

func invocationContractHelp(name string, contract contracts.InvocationContract) cliHelpSpec {
	var options strings.Builder
	for _, option := range contract.Options {
		options.WriteString("\n  --" + option.Name + " <" + option.Type + ">")
		if option.Required {
			options.WriteString(" (required)")
		}
		if option.Default != "" {
			options.WriteString(" [default: " + option.Default + "]")
		}
		if option.Description != "" {
			options.WriteString("  " + option.Description)
		}
	}
	if options.Len() == 0 {
		options.WriteString("\n  (none)")
	}
	description := fmt.Sprintf("Installed entry point %q of package %s@%s (graph %s@%s); aliases: %s.", contract.EntryPointID, contract.PackageID, contract.PackageVersion, contract.GraphID, contract.GraphVersion, strings.Join(contract.Aliases, ", "))
	if len(contract.RequiredCapabilities) > 0 {
		description += " Required capabilities: " + strings.Join(contract.RequiredCapabilities, ", ") + "."
	}
	if len(contract.RequiredEnforcement) > 0 {
		description += " Required enforcement: " + strings.Join(contract.RequiredEnforcement, ", ") + "."
	}
	return cliHelpSpec{usage: "usage: praxis " + name + " [--option=value ...]", description: description, options: strings.TrimPrefix(options.String(), "\n  ")}
}
