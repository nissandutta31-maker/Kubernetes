IMG ?= ghcr.io/nissandutta31-maker/nvidia-runtime-operator:latest
PLATFORMS ?= linux/amd64,linux/arm64

ENVTEST_K8S_VERSION ?= 1.31.0
ENVTEST_BIN_DIR ?= $(CURDIR)/bin/envtest

.PHONY: all build test test-integration setup-envtest run lint fmt vet generate manifests \
        docker-build docker-push install uninstall deploy undeploy sample \
        kind-up kind-down demo help

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
	kubectl create namespace nvidia-system --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply -f config/rbac/
	kubectl apply -f config/manager/

## undeploy: remove operator from the cluster
undeploy:
	kubectl delete -f config/manager/ --ignore-not-found
	kubectl delete -f config/rbac/ --ignore-not-found

## sample: apply sample RuntimePackage resources
sample:
	kubectl apply -f config/samples/

## setup-envtest: download a real kube-apiserver + etcd for integration tests
setup-envtest:
	go run sigs.k8s.io/controller-runtime/tools/setup-envtest@latest \
	  use $(ENVTEST_K8S_VERSION) --bin-dir $(ENVTEST_BIN_DIR)

## test-integration: run integration tests against a real API server (needs setup-envtest)
test-integration: setup-envtest
	KUBEBUILDER_ASSETS="$(shell go run sigs.k8s.io/controller-runtime/tools/setup-envtest@latest use $(ENVTEST_K8S_VERSION) --bin-dir $(ENVTEST_BIN_DIR) -p path)" \
	  go test -tags=integration -count=1 -v ./controllers/ -run TestIntegration

## run: run the operator locally against your current kubeconfig (no Docker needed)
run:
	go run . --leader-elect=false --metrics-bind-address=0 --health-probe-bind-address=0

## kind-up: create a local kind cluster and label its workers as GPU nodes
kind-up:
	kind create cluster --config hack/kind-cluster.yaml --wait 120s
	kubectl label nodes -l '!node-role.kubernetes.io/control-plane' nvidia.com/gpu.present=true --overwrite
	kubectl apply -f config/crd/

## kind-down: delete the local kind cluster
kind-down:
	kind delete cluster --name nvidia-demo

## demo: run the full guided end-to-end demo on a kind cluster
demo:
	./hack/demo.sh

## help: print this help
help:
	@printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"
	@grep -E '^## [a-zA-Z_0-9-]+:' $(MAKEFILE_LIST) | sed -e 's/^## //' \
	  | awk -F': ' '{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
