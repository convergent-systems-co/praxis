#!/bin/bash
cd "$(dirname "$0")"
export PRAXIS_REQUIRE_KEYCHAIN=1 SCRATCH=/private/tmp/claude-501/-Users-polliard-workspace-convergent-systems-co-praxis/22e5c5d9-551a-437b-828b-aac8196b6665/scratchpad
COPIES=1 OUT=results-r7-kc-final.json FRESH=1 python3 harness.py muts-kc.json > run-r7-kc-final.log 2>&1
COPIES=3 OUT=results-r7-rest-final.json FRESH=1 python3 harness.py muts-rest.json > run-r7-rest-final.log 2>&1
echo DONE > run-r7-final.done
