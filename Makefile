SHELL := /bin/sh

GO ?= go
PYTHON ?= python3
BIN ?= build/praxis
PREFIX ?=
DESTDIR ?=
RELEASE_VERSION ?=
RELEASE_DIR ?=

.PHONY: build clean key-bootstrap init test test-go test-current test-attestation test-python vet fmt doctor install package release-check

build:
	mkdir -p "$(dir $(BIN))"
	$(GO) build -o "$(BIN)" ./cmd/praxis

clean:
	rm -rf -- build

# Bootstrap is intentionally separate from state initialization. All values are
# required so Make cannot choose a provider, identity, profile, or metadata path.
key-bootstrap: build
	@test -n "$$PRAXIS_BOOTSTRAP_RECORD" || (echo 'PRAXIS_BOOTSTRAP_RECORD is required' >&2; exit 2)
	@test -n "$$KEY_BOOTSTRAP_PROVIDER" || (echo 'KEY_BOOTSTRAP_PROVIDER is required' >&2; exit 2)
	@test -n "$$KEY_BOOTSTRAP_ID" || (echo 'KEY_BOOTSTRAP_ID is required' >&2; exit 2)
	@test -n "$$KEY_BOOTSTRAP_OWNER" || (echo 'KEY_BOOTSTRAP_OWNER is required' >&2; exit 2)
	@test -n "$$KEY_BOOTSTRAP_PURPOSE" || (echo 'KEY_BOOTSTRAP_PURPOSE is required' >&2; exit 2)
	@test -n "$$KEY_BOOTSTRAP_PROFILE" || (echo 'KEY_BOOTSTRAP_PROFILE is required' >&2; exit 2)
	"$(BIN)" key-bootstrap \
		--provider "$$KEY_BOOTSTRAP_PROVIDER" \
		--key-id "$$KEY_BOOTSTRAP_ID" \
		--owner "$$KEY_BOOTSTRAP_OWNER" \
		--purpose "$$KEY_BOOTSTRAP_PURPOSE" \
		--profile "$$KEY_BOOTSTRAP_PROFILE" \
		--output "$$PRAXIS_BOOTSTRAP_RECORD"

# init never creates bootstrap authority. The record and database paths must be
# explicit, and state-init owns idempotent SQLite creation/migration.
init: build
	@test -n "$$PRAXIS_BOOTSTRAP_RECORD" || (echo 'PRAXIS_BOOTSTRAP_RECORD is required; run key-bootstrap explicitly first' >&2; exit 2)
	@test -f "$$PRAXIS_BOOTSTRAP_RECORD" || (echo 'bootstrap record does not exist; run key-bootstrap explicitly first' >&2; exit 2)
	@test -n "$$PRAXIS_DB" || (echo 'PRAXIS_DB is required' >&2; exit 2)
	PRAXIS_BOOTSTRAP_RECORD="$$PRAXIS_BOOTSTRAP_RECORD" PRAXIS_DB="$$PRAXIS_DB" "$(BIN)" state-init

test: test-go test-python

test-go: test-current test-attestation

test-current:
	@packages="$$($(GO) list ./... | grep -v '/internal/conformance$$')" || { echo 'unable to enumerate Go packages' >&2; exit 1; }; \
	test -n "$$packages" || { echo 'no current Go packages were enumerated' >&2; exit 1; }; \
	status=0; $(GO) test $$packages || status=$$?; \
	if [ $$status -ne 0 ]; then \
		echo 'Current Go tests failed. Historical conformance evidence is not a permitted workaround.' >&2; \
		exit $$status; \
	fi

test-attestation:
	@status=0; $(GO) test ./internal/conformance || status=$$?; \
	if [ $$status -ne 0 ]; then \
		echo 'Historical conformance/attestation tests failed. Do not rewrite immutable oracle or attestation evidence; investigate qualification state separately.' >&2; \
		exit $$status; \
	fi

test-python:
	$(PYTHON) -m pytest

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

doctor: build
	"$(BIN)" doctor

install: build
	@test -n "$(PREFIX)" || (echo 'PREFIX is required; refusing an implicit system install' >&2; exit 2)
	mkdir -p "$(DESTDIR)$(PREFIX)/bin"
	install -m 0755 "$(BIN)" "$(DESTDIR)$(PREFIX)/bin/praxis"

package:
	@test -n "$(RELEASE_VERSION)" || (echo 'RELEASE_VERSION is required' >&2; exit 2)
	@test -n "$(RELEASE_DIR)" || (echo 'RELEASE_DIR is required; choose an explicit derived output directory' >&2; exit 2)
	bash scripts/build-release.sh "$(RELEASE_VERSION)" "$(RELEASE_DIR)"

release-check: vet test-go package
	git diff --check
