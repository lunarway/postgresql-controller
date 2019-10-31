PROJECT=postgresql-controller
ORG?=lunarway
REG?=quay.io
TAG?=latest
NAMESPACE=default
SHELL=/bin/bash
COMPILE_TARGET=./build/_output/bin/$(PROJECT)
GOOS=darwin
GOARCH=amd64

.PHONY: code/run
code/run:
	@operator-sdk up local --namespace=${NAMESPACE}

.PHONY: code/compile
code/compile:
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 go build -o=$(COMPILE_TARGET) ./cmd/manager

.PHONY: code/check
code/check:
	@diff -u <(echo -n) <(gofmt -d `find . -type f -name '*.go' -not -path "./vendor/*"`)

.PHONY: code/fix
code/fix:
	@gofmt -w `find . -type f -name '*.go' -not -path "./vendor/*"`

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
