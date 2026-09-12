# b-issue61 outcomes

The redundant bare `filesystem` check was removed from `_lease_conflict_fn`; the
namespace-qualified suffix check still covers every declared filesystem resource
type. The transitions module-docstring test now positively pins both terminal
statuses and the on-failure edge rule, so the documentation cannot regress to a
vacuous negative-only assertion. The AuthTransportPolicy capabilities guard was
verified as already present, with its existing policy tests passing. The complete
test suite passes: `1066 passed`.
