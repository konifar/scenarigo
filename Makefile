SHELL := /bin/bash
.DEFAULT_GOAL := test

GO ?= go

BIN_DIR := $(CURDIR)/.bin
PATH := $(abspath $(BIN_DIR)):$(PATH)
TOOLS_DIR := $(CURDIR)/tools

UNAME_OS := $(shell uname -s)
UNAME_ARCH := $(shell uname -m)

PROTO_DIR := $(CURDIR)/testdata/proto
GEN_PB_DIR := $(CURDIR)/testdata/gen/pb
PLUGINS_DIR := $(CURDIR)/test/e2e/testdata/plugins
GEN_PLUGINS_DIR := $(CURDIR)/test/e2e/testdata/gen/plugins

$(BIN_DIR):
	@mkdir -p $(BIN_DIR)

PROTOC := $(BIN_DIR)/protoc
PROTOC_VERSION := 3.11.4
PROTOC_ZIP := protoc-$(PROTOC_VERSION)-$(UNAME_OS)-$(UNAME_ARCH).zip
ifeq "$(UNAME_OS)" "Darwin"
	PROTOC_ZIP=protoc-$(PROTOC_VERSION)-osx-$(UNAME_ARCH).zip
endif
$(PROTOC): | $(BIN_DIR)
	@curl -sSOL \
		"https://github.com/protocolbuffers/protobuf/releases/download/v$(PROTOC_VERSION)/$(PROTOC_ZIP)"
	@unzip -j -o $(PROTOC_ZIP) -d $(BIN_DIR) bin/protoc
	@unzip -o $(PROTOC_ZIP) -d $(BIN_DIR) "include/*"
	@rm -f $(PROTOC_ZIP)

PROTOC_GEN_GO := $(BIN_DIR)/protoc-gen-go
$(PROTOC_GEN_GO): | $(BIN_DIR)
	@cd $(TOOLS_DIR) && \
		$(GO) build -o $(PROTOC_GEN_GO) google.golang.org/protobuf/cmd/protoc-gen-go

PROTOC_GEN_GO_GRPC := $(BIN_DIR)/protoc-gen-go-grpc
$(PROTOC_GEN_GO_GRPC): | $(BIN_DIR)
	@cd $(TOOLS_DIR) && \
		$(GO) build -o $(PROTOC_GEN_GO_GRPC) google.golang.org/grpc/cmd/protoc-gen-go-grpc

GOPROTOYAMLTAG := $(BIN_DIR)/goprotoyamltag
$(GOPROTOYAMLTAG): | $(BIN_DIR)
	@cd $(TOOLS_DIR) && \
		$(GO) build -o $(GOPROTOYAMLTAG) github.com/zoncoen/goprotoyamltag

GOTYPENAMES := $(BIN_DIR)/gotypenames
$(GOTYPENAMES): | $(BIN_DIR)
	@cd $(TOOLS_DIR) && \
		$(GO) build -o $(GOTYPENAMES) github.com/zoncoen/gotypenames

MOCKGEN := $(BIN_DIR)/mockgen
$(MOCKGEN): | $(BIN_DIR)
	@cd $(TOOLS_DIR) && \
		$(GO) build -o $(MOCKGEN) github.com/golang/mock/mockgen

GOBUMP := $(BIN_DIR)/gobump
$(GOBUMP): | $(BIN_DIR)
	@cd $(TOOLS_DIR) && \
		$(GO) build -o $(GOBUMP) github.com/x-motemen/gobump/cmd/gobump

GIT_CHGLOG := $(BIN_DIR)/git-chglog
$(GIT_CHGLOG): | $(BIN_DIR)
	@cd $(TOOLS_DIR) && \
		$(GO) build -o $(GIT_CHGLOG) github.com/git-chglog/git-chglog/cmd/git-chglog

GO_LICENSES := $(BIN_DIR)/go-licenses
$(GO_LICENSES): | $(BIN_DIR)
	@cd $(TOOLS_DIR) && \
		$(GO) build -o $(GO_LICENSES) github.com/google/go-licenses

GOCREDITS := $(BIN_DIR)/gocredits
$(GOCREDITS): | $(BIN_DIR)
	@cd $(TOOLS_DIR) && \
		$(GO) build -o $(GOCREDITS) github.com/Songmu/gocredits/cmd/gocredits

GOLANGCI_LINT := $(BIN_DIR)/golangci-lint
GOLANGCI_LINT_VERSION := v1.30.0
$(GOLANGCI_LINT): | $(BIN_DIR)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(BIN_DIR) $(GOLANCI_LINT_VERSION)

.PHONY: test
E2E_TEST_TARGETS := test/e2e
TEST_TARGETS := $(shell $(GO) list ./... | grep -v $(E2E_TEST_TARGETS))
test: test/race test/norace

.PHONY: test/ci
test/ci: coverage test/race

.PHONY: coverage
coverage: ## measure test coverage
	@$(GO) test $(TEST_TARGETS) ./$(E2E_TEST_TARGETS)/... -coverprofile=coverage.out -covermode=atomic

.PHONY: test/norace
test/norace:
	@$(GO) test $(TEST_TARGETS) ./$(E2E_TEST_TARGETS)/...

.PHONY: test/race
test/race:
	@$(GO) test -race $(TEST_TARGETS) ./$(E2E_TEST_TARGETS)/...

.PHONY: lint
lint: $(GOLANGCI_LINT) ## run lint
	@golangci-lint run

.PHONY: lint/ci
lint/ci:
	@$(GO) version
	@make credits
	@git add --all
	@git diff --cached --exit-code || (echo '"make credits" required'; exit 1)

.PHONY: clean
clean: ## remove generated files
	@rm -rf $(BIN_DIR) $(GEN_PB_DIR) $(GEN_PLUGINS_DIR)

.PHONY: gen
gen: gen/proto gen/plugins ## generate necessary files for testing

.PHONY: gen/proto
PROTOC_OPTION := -I$(PROTO_DIR)
PROTOC_GO_OPTION := --plugin=${BIN_DIR}/protoc-gen-go --go_out=$(GEN_PB_DIR) --go_opt=paths=source_relative
PROTOC_GO_GRPC_OPTION := --go-grpc_out=require_unimplemented_servers=false:$(GEN_PB_DIR) --go-grpc_opt=paths=source_relative
gen/proto: $(PROTOC) $(PROTOC_GEN_GO) $(PROTOC_GEN_GO_GRPC)
	@rm -rf $(GEN_PB_DIR)
	@mkdir -p $(GEN_PB_DIR)
	@find $(PROTO_DIR) -name '*.proto' | xargs -P8 protoc $(PROTOC_OPTION) $(PROTOC_GO_OPTION) $(PROTOC_GO_GRPC_OPTION)
	@make add-yaml-tag
	@make gen/mock

.PHONY: add-yaml-tag
add-yaml-tag: $(GOPROTOYAMLTAG)
	@for file in $$(find $(GEN_PB_DIR) -name '*.pb.go'); do \
		echo "add yaml tag $$file"; \
		goprotoyamltag --filename $$file -w; \
	done

.PHONY: gen/mock
gen/mock: $(GOTYPENAMES) $(MOCKGEN)
	@for file in $$(find $(GEN_PB_DIR) -name '*_grpc.pb.go'); do \
		package=$$(basename $$(dirname $$file)); \
		echo "generate mock for $$file"; \
		dstfile=$$(dirname $$file)/$$(basename $${file%.pb.go})_mock.go; \
		self=github.com/zoncoen/scenarigo`echo $(GEN_PB_DIR)/$$package | perl -pe 's!^$(CURDIR)!!g'`; \
		gotypenames --filename $$file --only-exported --types interface | xargs -ISTRUCT -L1 -P8 mockgen -source $$file -package $$package -self_package $$self -destination $$dstfile; \
		perl -pi -e 's!^// Source: .*\n!!g' $$dstfile ||  (echo "failed to delete generated marker about source path ( Source: /path/to/name.pb.go )"); \
	done

.PHONY: gen/plugins
gen/plugins:
	@rm -rf $(GEN_PLUGINS_DIR)
	@mkdir -p $(GEN_PLUGINS_DIR)
	@for dir in $$(find $(PLUGINS_DIR) -name '*.go' | xargs -L1 -P8 dirname | sort | uniq); do \
		echo "build plugin $$(basename $$dir).so"; \
		$(GO) build -buildmode=plugin -o $(GEN_PLUGINS_DIR)/$$(basename $$dir).so $$dir; \
	done

.PHONY: release
release: $(GOBUMP) $(GIT_CHGLOG) ## release new version
	@$(CURDIR)/scripts/release.sh

.PHONY: changelog
changelog: $(GIT_CHGLOG) ## generate CHANGELOG.md
	@git-chglog -o $(CURDIR)/CHANGELOG.md

.PHONY: changelog/ci
changelog/ci: $(GIT_CHGLOG) $(GOBUMP)
	@git-chglog v$$(gobump show -r $(CURDIR)/version) > $(CURDIR)/.CHANGELOG.md

.PHONY: credits
credits: $(GO_LICENSES) $(GOCREDITS) ## generate CREDITS
	@$(GO) mod download
	@go-licenses check ./...
	@gocredits . > CREDITS
	@$(GO) mod tidy

.PHONY: build/ci
build/ci:
	@rm -rf assets
	@cd scripts/cross-build && PJ_ROOT=$(CURDIR) $(GO) run ./main.go && cd -
	@cp scripts/cross-build/.goreleaser.yml ./

.PHONY: help
help: ## print help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
