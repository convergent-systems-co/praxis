#!/bin/bash
# Repair 7 qualification chain on the frozen source. Run from the repository root.
# Sequential on purpose: several suites are load-sensitive.
set -u
B=docs/research/dogfood/praxis-human-interface/bootstrap-v4
Q=$B/repair-7-evidence/qualification
export PRAXIS_REQUIRE_KEYCHAIN=1
PKGS="./pkg/contracts ./internal/bootstrapv4 ./internal/goaldrive ./internal/goalstore ./internal/state ./internal/goalspublication ./internal/publisher ./internal/lifecycle ./internal/faa/... ./internal/crypto ./packages/goals ./cmd/praxis"
( make test-current; echo "EXIT=$?" ) > $Q/test-current.log 2>&1
( PYTHONDONTWRITEBYTECODE=1 python3 -m pytest -p no:cacheprovider; echo "EXIT=$?" ) > $Q/python.log 2>&1
go run $B/verify/specification_bundle.go > $Q/specification-bundle.log 2>&1
python3 docs/research/dogfood/praxis-human-interface/reviews/bootstrap-v4-kernel-astra-review-3-evidence/r3-activation/bundle_indep.py > $Q/bundle-independent-python.log 2>&1
go test ./internal/conformance -count=1 > $Q/historical.log 2>&1
go test ./internal/goalstore ./cmd/praxis -run 'TestFAAMixedSnapshotAdversaryNeverRegainsRetiredAuthority|TestRepair5EvidencePowersetClassificationQualification' -count=1 -v -timeout 1500s > $Q/powerset-and-mixed-snapshot.log 2>&1
python3 $B/repair-7-evidence/probes/replay_review_probes.py $Q/probe-replays > $Q/probe-replay-summary.log 2>&1
python3 $B/repair-7-evidence/probes/image_probe_replay.py $Q/probe-replays > $Q/image-probe-replay-summary.log 2>&1
go test -race -count=1 -timeout 2400s $PKGS > $Q/race.log 2>&1
echo DONE > $Q/run.done
