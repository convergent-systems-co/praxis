#!/bin/bash
cd "$(dirname "$0")"
export PRAXIS_REQUIRE_KEYCHAIN=1 SCRATCH=/private/tmp/claude-501/-Users-polliard-workspace-convergent-systems-co-praxis/22e5c5d9-551a-437b-828b-aac8196b6665/scratchpad
COPIES=1 OUT=results-r6-kc.json python3 harness.py muts-kc.json > run-r6-kc.log 2>&1
COPIES=3 OUT=results-r6-rest.json python3 harness.py muts-rest.json > run-r6-rest.log 2>&1
echo DONE > run-r6.done
