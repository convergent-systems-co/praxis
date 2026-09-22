#!/bin/bash
cd "$(dirname "$0")"
export PRAXIS_REQUIRE_KEYCHAIN=1 SCRATCH=/private/tmp/claude-501/-Users-polliard-workspace-convergent-systems-co-praxis/22e5c5d9-551a-437b-828b-aac8196b6665/scratchpad
COPIES=1 OUT=results-r6-kc-final.json python3 harness.py muts-kc.json > run-r6-kc-final.log 2>&1
echo DONE > run-r6-kc-final.done
