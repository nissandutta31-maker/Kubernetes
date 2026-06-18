#!/usr/bin/env bash
#
# End-to-end demo of the NVIDIA Runtime Operator on a local kind cluster.
#
# It creates a real 3-node Kubernetes cluster, runs the operator, applies a
# RuntimePackage, and watches the operator drive it to Ready — then performs a
# rolling upgrade. Everything you see is the real controller doing real work.
#
# Prereqs: docker, kind, kubectl, go (see docs/HANDS-ON.md to install them).
# Usage:   ./hack/demo.sh           (run the demo)
#          ./hack/demo.sh --cleanup (just delete the cluster)
#
set -euo pipefail

CLUSTER="nvidia-demo"
NS="nvidia-system"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

bold() { printf "\n\033[1m== %s ==\033[0m\n" "$1"; }
info() { printf "   \033[36m%s\033[0m\n" "$1"; }

if [[ "${1:-}" == "--cleanup" ]]; then
  kind delete cluster --name "$CLUSTER"
  exit 0
fi

for tool in docker kind kubectl go; do
  command -v "$tool" >/dev/null 2>&1 || { echo "ERROR: '$tool' is not installed. See docs/HANDS-ON.md."; exit 1; }
done

OPERATOR_PID=""
OPERATOR_LOG="$(mktemp)"
cleanup() {
  [[ -n "$OPERATOR_PID" ]] && kill "$OPERATOR_PID" 2>/dev/null || true
}
trap cleanup EXIT

bold "1/7  Creating a real 3-node Kubernetes cluster with kind"
if kind get clusters 2>/dev/null | grep -qx "$CLUSTER"; then
  info "cluster '$CLUSTER' already exists, reusing it"
else
  kind create cluster --config hack/kind-cluster.yaml --wait 120s
fi
kubectl config use-context "kind-${CLUSTER}" >/dev/null
kubectl get nodes

bold "2/7  Labeling the worker nodes as GPU nodes"
info "real clusters get this label from NVIDIA's GPU Feature Discovery; we set it by hand"
kubectl label nodes -l '!node-role.kubernetes.io/control-plane' nvidia.com/gpu.present=true --overwrite
kubectl get nodes -L nvidia.com/gpu.present

bold "3/7  Installing the RuntimePackage CRD (teaching Kubernetes a new resource type)"
kubectl create namespace "$NS" --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f config/crd/
kubectl apply -f config/rbac/ # ServiceAccount (in $NS) + ClusterRole + binding
info "the cluster now understands 'kind: RuntimePackage'"
kubectl api-resources | grep -i runtimepackage || true

bold "4/7  Starting the operator (out-of-cluster, so you can watch its logs)"
go run . --leader-elect=false --metrics-bind-address=0 --health-probe-bind-address=0 \
  >"$OPERATOR_LOG" 2>&1 &
OPERATOR_PID=$!
info "operator PID $OPERATOR_PID — logs: $OPERATOR_LOG"
sleep 6
grep -i "starting" "$OPERATOR_LOG" | head -3 || true

bold "5/7  Applying a RuntimePackage — declaring desired state"
kubectl apply -f config/samples/demo_runtimepackage.yaml
info "we asked for: install nvidia-container-toolkit 1.14.6 on all GPU nodes"

bold "6/7  Watching the operator reconcile it to Ready"
phase=""
for _ in $(seq 1 40); do
  phase="$(kubectl get rtpkg demo-container-toolkit -n "$NS" -o jsonpath='{.status.phase}' 2>/dev/null || true)"
  printf "   phase=%s  daemonset-ready=%s\n" \
    "${phase:-<none>}" \
    "$(kubectl get ds runtime-pkg-demo-container-toolkit -n "$NS" -o jsonpath='{.status.numberReady}/{.status.desiredNumberScheduled}' 2>/dev/null || echo '-')"
  [[ "$phase" == "Ready" ]] && break
  sleep 3
done
echo
kubectl get rtpkg -n "$NS"
echo
info "the operator created this DaemonSet (one installer pod per GPU node):"
kubectl get ds,pods -n "$NS" -o wide

if [[ "$phase" != "Ready" ]]; then
  echo; echo "Package did not reach Ready in time. Operator logs:"; tail -20 "$OPERATOR_LOG"
  exit 1
fi

bold "7/7  Rolling upgrade — change the desired version, operator rolls the DaemonSet"
kubectl patch rtpkg demo-container-toolkit -n "$NS" --type=merge -p '{"spec":{"version":"1.15.0"}}'
info "bumped version 1.14.6 -> 1.15.0; watch the phase flip to Upgrading then back to Ready"
for _ in $(seq 1 40); do
  phase="$(kubectl get rtpkg demo-container-toolkit -n "$NS" -o jsonpath='{.status.phase}' 2>/dev/null || true)"
  ver="$(kubectl get rtpkg demo-container-toolkit -n "$NS" -o jsonpath='{.status.installedVersion}' 2>/dev/null || true)"
  printf "   phase=%s installedVersion=%s\n" "${phase:-<none>}" "${ver:-<none>}"
  [[ "$phase" == "Ready" && "$ver" == "1.15.0" ]] && break
  sleep 3
done
echo
kubectl get rtpkg -n "$NS"

bold "Done!"
cat <<EOF
   You just ran a Kubernetes operator end to end:
     • taught the cluster a custom resource (CRD)
     • declared desired state (a RuntimePackage)
     • the controller reconciled reality to match (created a DaemonSet)
     • it tracked per-node readiness and reported status
     • a version change triggered a rolling upgrade

   Inspect more:
     kubectl describe rtpkg demo-container-toolkit -n $NS
     kubectl get events -n $NS --sort-by=.lastTimestamp

   Tear down when finished:
     ./hack/demo.sh --cleanup
EOF
