# Kubernetes

A lightweight NVIDIA-focused Kubernetes showcase project.

## What this demo does

- Generates a GPU-ready Kubernetes Deployment + Service manifest.
- Adds NVIDIA GPU resource requests/limits (`nvidia.com/gpu`).
- Prints measurable summary output (replicas, CPU/memory, GPU totals) for easy demo narration.

## Run locally

```bash
cd <project-root>
go run . generate-manifest
```

### Custom example

```bash
go run . generate-manifest \
  -name nvidia-internship-demo \
  -namespace ai \
  -image nvcr.io/nvidia/pytorch:24.03-py3 \
  -replicas 2 \
  -cpu 1000m \
  -memory 2Gi \
  -gpus 1 \
  -port 8080 \
  -out ./nvidia-dgx-cloud-k8s-demo.yaml
```

When `-out` is used, YAML is written to the file and the measurable summary is still printed to stdout.

## CI-style validation

This repository currently has no `go.mod`, so CI runs with `GO111MODULE=off`.

```bash
GO111MODULE=off go build -v ./...
GO111MODULE=off go test -v ./...
```

## 2-minute internship demo script

1. **Problem**: Teams need a fast, reproducible way to define GPU workloads in Kubernetes.
2. **Solution**: This CLI generates deployment manifests with explicit NVIDIA GPU scheduling and service exposure.
3. **Architecture**: A Go command-line entrypoint + manifest generator + measurable summary output + tests.
4. **Result**: You can instantly produce GPU-aware YAML and quantify requested resources for capacity planning.

## Honest scope

This is a focused starter showcase, not a full production platform.

## Suggested next steps

- Add cluster health checks for NVIDIA device plugin readiness.
- Add Prometheus metrics export and dashboard snapshots.
- Add sample benchmark runs (before/after scaling) and deployment screenshots.
