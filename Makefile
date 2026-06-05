IMG ?= ghcr.io/nissandutta31-maker/nvidia-runtime-operator:latest
PLATFORMS ?= linux/amd64,linux/arm64

.PHONY: all build test lint docker-build docker-push install uninstall deploy undeploy help

all: build

## build: compile the operator binary
build:
	go build -o bin/manager ./...

## test: run unit tests with race detection
test:
	go test -race -v ./... -coverprofile=cover.out -covermode=atomic
	go tool cover -func=cover.out

## lint: run golangci-lint
lint:
	golangci-lint run ./...

## fmt: run gofmt
fmt:
	go fmt ./...

## vet: run go vet
vet:
	go vet ./...

## generate: re-generate deepcopy methods (requires controller-gen)
generate:
	controller-gen object:headerFile="hack/boilerplate.go.txt" paths="./..."

## manifests: re-generate CRD manifests (requires controller-gen)
manifests:
	controller-gen crd rbac:roleName=manager-role webhook paths="./..." \
	  output:crd:artifacts:config=config/crd \
	  output:rbac:artifacts:config=config/rbac

## docker-build: build multi-arch container image
docker-build:
	docker buildx build \
	  --platform $(PLATFORMS) \
	  --tag $(IMG) \
	  --load \
	  .

## docker-push: push image to registry
docker-push:
	docker buildx build \
	  --platform $(PLATFORMS) \
	  --tag $(IMG) \
	  --push \
	  .

## install: apply CRD to the current cluster
install:
	kubectl apply -f config/crd/

## uninstall: remove CRD from the current cluster
uninstall:
	kubectl delete -f config/crd/

## deploy: deploy the operator into nvidia-system namespace
deploy:
	kubectl apply -f config/rbac/
	kubectl apply -f config/manager/

## undeploy: remove operator from the cluster
undeploy:
	kubectl delete -f config/manager/ --ignore-not-found
	kubectl delete -f config/rbac/ --ignore-not-found

## sample: apply sample RuntimePackage resources
sample:
	kubectl apply -f config/samples/

## help: print this help
help:
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} \
	  /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
