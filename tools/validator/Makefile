BINARY := bin/policystack-validator
PKG := ./cmd/policystack-validator
ROOT := ../..
# validate runs from the repo root, where CI annotations and --extra-values paths resolve. ELEMENT
# limits validate and preview to one chart.
VALIDATOR := $(abspath $(BINARY)) --repo-root . $(if $(CI),--github) $(if $(ELEMENT),--element $(ELEMENT))
BASELINE := tools/validator/testdata/baseline.yaml
# Every chart that depends on policy-library, TestConformance's fixture included.
CHARTS := $(patsubst %/Chart.yaml,%,$(wildcard $(ROOT)/stack/*/Chart.yaml)) $(ROOT)/lifecycle $(ROOT)/tools/sample-element internal/chart/testdata/element

.PHONY: build test lint fmt clean run tidy deps crds validate preview ci

# CRDs POLICY080 validates rendered objects against, at the ACM 2.16 floor (docs/develop/validation.md#crd-schemas).
# Renovate bumps each SHA to its branch head; the branches move with the floor, by hand.
CRDS := .crds
# renovate: datasource=git-refs depName=https://github.com/stolostron/governance-policy-propagator branch=release-2.16
PROPAGATOR_SHA ?= 2d03f8350c7da2a7a40892f11c88e91bb843abaf
# renovate: datasource=git-refs depName=https://github.com/stolostron/config-policy-controller branch=release-2.16
CPC_SHA ?= 73831c0649f8c55e8a3693fd8185a14649bf8a42
# renovate: datasource=git-refs depName=https://github.com/stolostron/cert-policy-controller branch=release-2.16
CERT_SHA ?= b274b410e6fa824b20cca59ab32d359320beed86
# The open-cluster-management.io/api version propagator release-2.16 requires (v1.2.0).
# renovate: datasource=git-refs depName=https://github.com/open-cluster-management-io/api branch=release-1.2
OCM_API_SHA ?= b449b3b4e4b95955b76c2babed59cae51e70fe53
RAW := https://raw.githubusercontent.com
CRD_URLS := \
	$(foreach k,policies placementbindings policysets,$(RAW)/stolostron/governance-policy-propagator/$(PROPAGATOR_SHA)/deploy/crds/policy.open-cluster-management.io_$(k).yaml) \
	$(foreach k,configurationpolicies operatorpolicies,$(RAW)/stolostron/config-policy-controller/$(CPC_SHA)/deploy/crds/policy.open-cluster-management.io_$(k).yaml) \
	$(RAW)/stolostron/cert-policy-controller/$(CERT_SHA)/deploy/crds/policy.open-cluster-management.io_certificatepolicies.yaml \
	$(RAW)/open-cluster-management-io/api/$(OCM_API_SHA)/cluster/v1beta1/0000_02_clusters.open-cluster-management.io_placements.crd.yaml

build:
	go build -trimpath -o $(BINARY) $(PKG)

test: crds
	go test -race ./...

# golangci-lint also reports gofmt and goimports drift (.golangci.yml); make fmt fixes it.
lint:
	golangci-lint run
	go mod tidy -diff
	go tool govulncheck ./...

fmt:
	golangci-lint fmt

tidy:
	go mod tidy

clean:
	rm -rf bin

run: build
	./$(BINARY) --repo-root ../..

# Packages lib/policy-library into every chart's charts/ ahead of time. validate does it on every run.
deps:
	@for c in $(CHARTS); do helm dependency update --skip-refresh $$c >/dev/null || exit 1; done

# Refetches only when a pin changes. CI caches the directory keyed on this file.
crds:
	@[ "$$(cat $(CRDS)/.pins 2>/dev/null)" = "$(CRD_URLS)" ] || { \
	  rm -rf $(CRDS) && mkdir -p $(CRDS) && \
	  for u in $(CRD_URLS); do curl -fsSL --retry 3 -o "$(CRDS)/$${u##*/}" "$$u" || exit 1; done && \
	  echo "$(CRD_URLS)" > $(CRDS)/.pins; }

# Three passes, all run, any failure fails:
# 1. Fixture clusters plus tools/sample-element. POLICY081 runs in pass 2, where every converter
#    renders. POLICY071 (install/) runs here only.
# 2. Each profile turns every element and optional template on, tools/sample-element too
#    (docs/develop/validation.md#acm-templates). In CI, sort -u drops the annotations two profiles both report.
# 3. A fork's real fleet, when it has one. Upstream ships none. POLICY052 (fleet pins) runs here
#    only: the fixtures' revisions are not real pins.
validate: build crds
	@cd $(ROOT) && failed=0; \
	$(VALIDATOR) --extra-values $(BASELINE) --include-sample-element --skip POLICY052,POLICY081 || failed=1; \
	acm=$$(mktemp); \
	for p in tools/validator/testdata/profiles/*.yaml; do \
	  $(VALIDATOR) --extra-values "$(BASELINE),$$p" --include-sample-element --skip POLICY052,POLICY071 >> "$$acm" || failed=1; \
	done; \
	$(if $(CI),sort -u,cat) "$$acm"; rm -f "$$acm"; \
	if [ -n "$$(ls fleet/*.yaml fleet/hubs/*.yaml 2>/dev/null)" ]; then \
	  $(VALIDATOR) --fixtures-dir fleet --extra-values $(BASELINE) --skip POLICY071 || failed=1; \
	fi; \
	exit $$failed

# Renders ELEMENT, turned on, for CLUSTER (default every fixture) into a new directory, or PREVIEW
# (relative to tools/validator): <fleet file>/<element>.yaml holds the Policies and
# <element>.evaluated.yaml the objects ACM applies on an empty cluster, ACM templates resolved.
# Prints both when CLUSTER is set.
preview: build
	@test -n "$(ELEMENT)" || { echo "usage: make preview ELEMENT=<element> [CLUSTER=<fleet file>]"; exit 2; }
	@dir=$${PREVIEW:-$$(mktemp -d)} && mkdir -p "$$dir" && dir=$$(cd "$$dir" && pwd) && \
	enable=$$(mktemp) && trap 'rm -f "$$enable"' EXIT && \
	key=$$(echo "$(ELEMENT)" | awk -F- '{printf "%s", $$1; for (i = 2; i <= NF; i++) printf "%s", toupper(substr($$i, 1, 1)) substr($$i, 2)}') && \
	printf 'stack:\n  %s:\n    enabled: true\n' "$$key" > "$$enable" && \
	cd $(ROOT) && failed=0; \
	$(VALIDATOR) --extra-values "$(BASELINE),$$enable" --include-sample-element $(if $(CLUSTER),--cluster $(CLUSTER)) \
	  --only RENDER000,POLICY081 --render-dir "$$dir" --evaluate || failed=1; \
	find "$$dir" -name '*.yaml' | sort | $(if $(CLUSTER),xargs tail -n +1,cat); \
	exit $$failed

ci: lint validate test
