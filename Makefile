PROJECT=postgresql-controller
# Current Operator version
VERSION ?= 0.2.0
# Image URL to use all building/pushing image targets
ORG ?= lunarway
REG ?= quay.io
TAG ?= latest
IMG ?= ${REG}/${ORG}/${PROJECT}:${TAG}

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: fmt/check
fmt/check: ## Run gofmt to ensure there is no diff
	@diff -u <(echo -n) <(gofmt -d `find . -type f -name '*.go' -not -path "./vendor/*"`)

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

##@ Build

.PHONY: build
build: generate fmt vet ## Build manager binary.
	go build -o bin/manager .

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run .

.PHONY: docker
docker-buildx-build: test
	docker buildx build --platform=linux/amd64,linux/arm64 -t ${IMG} .

.PHONY: docker
docker-buildx-push: docker-buildx-build
	docker buildx build --platform=linux/amd64,linux/arm64 -t ${IMG} --push .

.PHONY: docker
docker-build: test ## Build docker image with the manager.
	docker build -t ${IMG} .

.PHONY: docker
docker-push: ## Push docker image with the manager.
	docker push ${IMG}

ifndef ignore-not-found
  ignore-not-found = false
endif

##@ Deployment

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | sh kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | sh kubectl delete --ignore-not-found=$(ignore-not-found) -f -


##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest

## Tool Versions
KUSTOMIZE_VERSION ?= v4.5.5
CONTROLLER_TOOLS_VERSION ?= v0.9.2

KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	test -s $(LOCALBIN)/kustomize || { curl -s $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN); }

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(LOCALBIN)/controller-gen || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.24

.PHONY: test/unit
test/unit: fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" gotestsum

POSTGRESQL_CONTROLLER_INTEGRATION_HOST=localhost:5432

.PHONY: test/integration/dependencies/run
test/integration/dependencies/run:
	-docker-compose up -d
	-sleep 5

.PHONY: test/integration/dependencies/stop
test/integration/dependencies/stop:
	-docker-compose down

.PHONY: test/integration
test/integration: test/integration/dependencies/run
	@echo Running integration tests against PostgreSQL instance on ${POSTGRESQL_CONTROLLER_INTEGRATION_HOST}:
	POSTGRESQL_CONTROLLER_INTEGRATION_HOST=${POSTGRESQL_CONTROLLER_INTEGRATION_HOST} make test/unit

.PHONY: test/cluster
test/cluster:
	@echo Create test cluster
	export KUBECONFIG=~/.kube/config
	kind create cluster --name postgresql-controller-test

.PHONY: test/cluster/resources
test/cluster/resources:
	kubectl apply -f config/rbac/role.yaml -f config/rbac/role_binding.yaml
	kubectl apply -f config/crd/bases/postgresql.lunar.tech_postgresqlusers.yaml
	kubectl apply -f config/crd/bases/postgresql.lunar.tech_postgresqldatabases.yaml
	kubectl apply -f test/postgresql.yaml

.PHONY: test/cluster/postgresql
test/cluster/postgresql:
	kubectl apply -f test/postgresql.yaml

.PHONY: release
release:
	sed -i "" 's|^VERSION.*|VERSION ?= ${TAG}|' Makefile
	git add Makefile
	git commit -m"Release v${TAG}"
	git tag v${TAG}
	git push
	git push --tags
