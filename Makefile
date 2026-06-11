.PHONY: help build build-local kind-up kind-down load-image deploy verify port-forward clean all

APP_NAME    := nvidia-demo-app
SVC_NAME    := nvidia-demo-svc
NAMESPACE   := nvidia-runtime-demo
KIND_CLUSTER := nvidia-demo
K8S_DIR     := k8s

# ── Default target ──────────────────────────────────────────────
help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}'

# ── Build ───────────────────────────────────────────────────────
build: ## Build the Docker image for the Go application
	docker build -t $(APP_NAME):latest .

build-local: ## Build the Go binary locally (fast iteration, no Docker)
	cd app && CGO_ENABLED=0 go build -ldflags="-s -w" -o ../bin/server main.go

# ── Cluster ─────────────────────────────────────────────────────
kind-up: ## Create a local Kubernetes cluster using Kind
	kind create cluster --name $(KIND_CLUSTER) || echo "Cluster may already exist."

kind-down: ## Delete the Kind cluster
	kind delete cluster --name $(KIND_CLUSTER)

load-image: ## Load the Docker image into the Kind cluster
	kind load docker-image $(APP_NAME):latest --name $(KIND_CLUSTER)

# ── Deploy ──────────────────────────────────────────────────────
deploy: ## Apply all Kubernetes manifests to the cluster
	kubectl apply -f $(K8S_DIR)/
	@echo ""
	@echo "⏳ Waiting for pods to become ready..."
	kubectl wait --for=condition=ready pod \
		-l app=$(APP_NAME) \
		-n $(NAMESPACE) \
		--timeout=60s || true

# ── Verify ──────────────────────────────────────────────────────
verify: ## Run the Python health-check script against the cluster
	python3 automation/health_check.py

# ── Port Forward (for local testing) ────────────────────────────
port-forward: ## Forward the service port to localhost:8080
	kubectl port-forward svc/$(SVC_NAME) 8080:80 -n $(NAMESPACE)

# ── Teardown ────────────────────────────────────────────────────
clean: ## Remove all deployed resources from the cluster
	kubectl delete namespace $(NAMESPACE) --ignore-not-found

# ── All-in-one ──────────────────────────────────────────────────
all: kind-up build load-image deploy verify ## Full demo: cluster → build → load → deploy → verify
	@echo ""
	@echo "🚀 NVIDIA DGX Cloud Runtime Demo is live!"
	@echo "   Run 'make port-forward' then visit http://localhost:8080"
