PROJECT=postgresql-controller
ORG?=lunarway
REG?=quay.io
TAG?=latest
NAMESPACE=default
SHELL=/bin/bash
COMPILE_TARGET=./build/_output/bin/$(PROJECT)
GOOS=darwin
GOARCH=amd64
POSTGRESQL_CONTROLLER_INTEGRATION_HOST=localhost:5432

.PHONY: code/run
code/run:
	@operator-sdk up local --namespace=${NAMESPACE} --operator-flags --zap-devel

.PHONY: code/compile
code/compile:
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 go build -o=$(COMPILE_TARGET) ./cmd/manager

.PHONY: code/check
code/check:
	@diff -u <(echo -n) <(gofmt -d `find . -type f -name '*.go' -not -path "./vendor/*"`)

.PHONY: code/fix
code/fix:
	@gofmt -w `find . -type f -name '*.go' -not -path "./vendor/*"`

.PHONY: code/generate
code/generate:
	operator-sdk generate k8s
	operator-sdk generate openapi

.PHONY: image/build
image/build: code/compile
	@operator-sdk build ${REG}/${ORG}/${PROJECT}:${TAG}

.PHONY: image/push
image/push:
	docker push ${REG}/${ORG}/${PROJECT}:${TAG}

.PHONY: image/build/push
image/build/push: image/build image/push

.PHONY: test/unit
test/unit:
	@echo Running tests:
	go test -v -race -cover ./pkg/...

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
	POSTGRESQL_CONTROLLER_INTEGRATION_HOST=${POSTGRESQL_CONTROLLER_INTEGRATION_HOST} go test -v -race -cover -count=1 ./pkg/...

.PHONY: test/cluster
test/cluster:
	@echo Create test cluster
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

.PHONY: release
release:
	sed -i "" 's|${REG}/${ORG}/${PROJECT}.*|${REG}/${ORG}/${PROJECT}:${TAG}|g' deploy/operator.yaml
	git add deploy/operator.yaml
	git commit -m"Release ${TAG}"
	git tag ${TAG}
	git push
	git push --tags
