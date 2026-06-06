.PHONY: help build kind-up deploy verify clean

APP_NAME    := nvidia-demo-app
NAMESPACE   := nvidia-runtime-demo
K8S_DIR     := k8s

# ── Default target ──────────────────────────────────────────────
help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}'

# ── Build ───────────────────────────────────────────────────────
build: ## Build the Docker image for the Go application
	docker build -t $(APP_NAME):latest .

# ── Cluster ─────────────────────────────────────────────────────
kind-up: ## Create a local Kubernetes cluster using Kind
	kind create cluster --name nvidia-demo || echo "Cluster may already exist."

kind-down: ## Delete the Kind cluster
	kind delete cluster --name nvidia-demo

# ── Deploy ──────────────────────────────────────────────────────
deploy: ## Apply all Kubernetes manifests to the cluster
	kubectl apply -f $(K8S_DIR)/namespace.yaml
	kubectl apply -f $(K8S_DIR)/deployment.yaml
	kubectl apply -f $(K8S_DIR)/service.yaml
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
	kubectl port-forward svc/$(APP_NAME) 8080:80 -n $(NAMESPACE)

# ── Teardown ────────────────────────────────────────────────────
clean: ## Remove all deployed resources from the cluster
	kubectl delete namespace $(NAMESPACE) --ignore-not-found

# ── All-in-one ──────────────────────────────────────────────────
all: kind-up build deploy verify ## Full demo: cluster → build → deploy → verify
	@echo ""
	@echo "🚀 NVIDIA DGX Cloud Runtime Demo is live!"
	@echo "   Run 'make port-forward' then visit http://localhost:8080"
