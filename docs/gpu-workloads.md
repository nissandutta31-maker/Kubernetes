# GPU Workloads on DGX Cloud-Style Clusters

This document describes how the optional GPU manifest in [`k8s/gpu-deployment.yaml`](../k8s/gpu-deployment.yaml) maps to production DGX Cloud deployments.

## What this manifest demonstrates

| Pattern | Purpose |
|---|---|
| `resources.limits.nvidia.com/gpu: 1` | Reserves one GPU per pod — same resource model used for inference and training jobs |
| `nodeSelector: nvidia.com/gpu.present: "true"` | Schedules only onto GPU-capable nodes |
| GPU toleration | Allows scheduling onto tainted GPU node pools common in managed clusters |
| `nvidia-smi` startup command | Sanity-checks driver visibility inside the container |

## Local Kind cluster (honest limits)

**This demo runs on Kind without GPUs.** If you apply the GPU deployment locally:

```bash
kubectl apply -f k8s/gpu-deployment.yaml
kubectl get pods -n nvidia-runtime-demo -l app=nvidia-gpu-demo
```

The pod will remain **Pending** with an event like `Insufficient nvidia.com/gpu`. That is expected — it proves the scheduler is enforcing GPU constraints correctly.

Do **not** claim GPU execution in outreach materials unless you have run this on a real GPU cluster.

## Production requirements

To run this manifest on a real cluster you need:

1. **NVIDIA GPU Operator** or **device plugin** installed
2. Nodes labeled/tainted for GPU workloads
3. Container runtime configured for NVIDIA (`nvidia-container-toolkit`)
4. A cluster with allocatable `nvidia.com/gpu` capacity (`kubectl describe node`)

## Deploy on a GPU cluster

```bash
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/gpu-deployment.yaml
kubectl wait --for=condition=ready pod -l app=nvidia-gpu-demo -n nvidia-runtime-demo --timeout=120s
kubectl logs -n nvidia-runtime-demo -l app=nvidia-gpu-demo --tail=20
```

Expected log output includes `nvidia-smi` device listing.

## Related: inference serving

For model serving on DGX Cloud, teams typically deploy **Triton Inference Server** or **vLLM** with:

- PersistentVolumeClaims for model weights
- HorizontalPodAutoscaler on GPU utilization or request queue depth
- Readiness probes against `/v2/health/ready` (Triton) or `/health` (vLLM)

This repo focuses on the **platform primitives** (scheduling, probes, automation). A follow-on project could add a Triton manifest with the same namespace and resource patterns.
