PROJECT=postgresql-controller
# Current Operator version
VERSION ?= 0.0.23
# Default bundle image tag
BUNDLE_IMG ?= controller-bundle:$(VERSION)
# Options for 'bundle-build'
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)
# Image URL to use all building/pushing image targets
ORG ?= lunarway
REG ?= quay.io
TAG ?= latest
IMG ?= ${REG}/${ORG}/${PROJECT}:${TAG}
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"
SHELL=/bin/bash

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))VERSION
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: manager

BINARY_ASSETS=bin
# Build manager binary
manager: fmt vet
	go build -o ${BINARY_ASSETS}/manager .

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run ./main.go

# Install CRDs into a cluster
install: manifests kustomize
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

# Uninstall CRDs from a cluster
uninstall: manifests kustomize
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests kustomize
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Run go fmt against code
fmt:
	go fmt ./...

.PHONY: fmt/check
# Run gofmt to ensure there is no diff
fmt/check:
	@diff -u <(echo -n) <(gofmt -d `find . -type f -name '*.go' -not -path "./vendor/*"`)

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen openapi-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."
	$(OPENAPI_GEN) --input-dirs ./api/v1alpha1  --output-package ./api/v1alpha1

# Build the docker image
docker-build: test
	docker build . -t ${IMG}

# Push the docker image
docker-push:
	docker push ${IMG}

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.3.0 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

openapi-gen::
ifeq (, $(shell which openapi-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get k8s.io/kube-openapi/cmd/openapi-gen ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
OPENAPI_GEN_EXEC=$(GOBIN)/openapi-gen
else
OPENAPI_GEN_EXEC=$(shell which openapi-gen)
endif
# setup common arguments for the openapi-gen executor
OPENAPI_GEN=${OPENAPI_GEN_EXEC} ./${BINARY_ASSETS}/openapi-gen --logtostderr=true \
		--output-base "" \
		--output-file-base zz_generated.openapi \
		--go-header-file 'hack/boilerplate.go.txt' \
		--report-filename "-"

kustomize:
ifeq (, $(shell which kustomize))
	@{ \
	set -e ;\
	KUSTOMIZE_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$KUSTOMIZE_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/kustomize/kustomize/v3@v3.5.4 ;\
	rm -rf $$KUSTOMIZE_GEN_TMP_DIR ;\
	}
KUSTOMIZE=$(GOBIN)/kustomize
else
KUSTOMIZE=$(shell which kustomize)
endif

OPERATOR_SDK_VERSION = v1.0.1
OPERATOR_SDK ?= ./${BINARY_ASSETS}/operator-sdk-${OPERATOR_SDK_VERSION}

operator-sdk:
	mkdir -p ${BINARY_ASSETS}
	test -f ${OPERATOR_SDK} || (curl -o ${OPERATOR_SDK} -LO https://github.com/operator-framework/operator-sdk/releases/download/${OPERATOR_SDK_VERSION}/operator-sdk-${OPERATOR_SDK_VERSION}-x86_64-apple-darwin && chmod +x ${OPERATOR_SDK})

# Generate bundle manifests and metadata, then validate generated files.
bundle: manifests operator-sdk
	$(OPERATOR_SDK) generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	kustomize build config/manifests | $(OPERATOR_SDK) generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	$(OPERATOR_SDK) bundle validate ./bundle

# Build the bundle image.
bundle-build:
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

ENVTEST_ASSETS_DIR=$(shell pwd)/testbin

# Run tests
.PHONY: test/unit
test/unit: fmt vet
	mkdir -p ${ENVTEST_ASSETS_DIR}
	test -f ${ENVTEST_ASSETS_DIR}/setup-envtest.sh || curl -sSLo ${ENVTEST_ASSETS_DIR}/setup-envtest.sh https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/master/hack/setup-envtest.sh
	source ${ENVTEST_ASSETS_DIR}/setup-envtest.sh; \
	fetch_envtest_tools $(ENVTEST_ASSETS_DIR); \
	setup_envtest_env $(ENVTEST_ASSETS_DIR); \
	go test -v -race ./... -coverprofile cover.out

POSTGRESQL_CONTROLLER_INTEGRATION_HOST=localhost:5432
POSTGRESQL_CONTROLLER_INTEGRATION_HOST_CONTAINER=postgresql-controler-int-test

.PHONY: test/integration/postgresql/run
test/integration/postgresql/run:
	@echo Running integration test PostgreSQL instance on localhost:5432:
	-docker run --rm -p 5432:5432 -e POSTGRES_USER=admin --name ${POSTGRESQL_CONTROLLER_INTEGRATION_HOST_CONTAINER} -d timms/postgres-logging:11.5 && \
		sleep 5 && \
		docker exec ${POSTGRESQL_CONTROLLER_INTEGRATION_HOST_CONTAINER} \
		  psql -Uadmin -c "CREATE USER iam_creator WITH CREATEDB CREATEROLE VALID UNTIL 'infinity';"
	@echo Database running and iam_creator role created.
	@echo Attach to instance with 'make test/integration/postgresql/attach'

.PHONY: test/integration/postgresql/attach
test/integration/postgresql/attach:
	docker attach ${POSTGRESQL_CONTROLLER_INTEGRATION_HOST_CONTAINER}

.PHONY: test/integration/postgresql/stop
test/integration/postgresql/stop:
	-docker kill ${POSTGRESQL_CONTROLLER_INTEGRATION_HOST_CONTAINER}

.PHONY: test/integration
test/integration: test/integration/postgresql/run
	@echo Running integration tests against PostgreSQL instance on ${POSTGRESQL_CONTROLLER_INTEGRATION_HOST}:
	POSTGRESQL_CONTROLLER_INTEGRATION_HOST=${POSTGRESQL_CONTROLLER_INTEGRATION_HOST} make test/unit

.PHONY: test/cluster
test/cluster:
	@echo Create test cluster
	export KUBECONFIG=~/.kube/config
	kind create cluster --name postgresql-controller-test

.PHONY: test/cluster/resources
test/cluster/resources:
	kubectl apply -f deploy/role.yaml -f deploy/role_binding.yaml -f deploy/service_account.yaml
	kubectl apply -f deploy/crds/lunarway.com_postgresqlusers_crd.yaml
	kubectl apply -f deploy/crds/lunarway.com_postgresqldatabases_crd.yaml
	kubectl apply -f test/postgresql.yaml

.PHONY: test/cluster/postgresql
test/cluster/postgresql:
	kubectl apply -f test/postgresql.yaml
