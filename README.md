# NVIDIA Runtime Operator

A Kubernetes operator that manages the lifecycle of GPU runtime packages across
clusters — mirroring the automation patterns used by NVIDIA's DGX Cloud Runtime team
to distribute the container toolkit, DRA drivers, and other accelerated compute
components to GPU nodes.

## Overview

The operator introduces the `RuntimePackage` Custom Resource Definition (CRD).
Each `RuntimePackage` describes a GPU runtime package (name, version, target
architectures, node selector) and the controller ensures that an installer
DaemonSet exists on every matching node, automatically handling installs and
rolling upgrades.

```
Operator (controller-manager)
  └── watches RuntimePackage CRs
        └── creates/updates installer DaemonSets per package
              └── one Pod per matching node → installs package to host filesystem
```

## Supported GPU Architectures

| Architecture | Example Product    |
|-------------|--------------------|
| A100        | NVIDIA A100 80GB   |
| H100        | NVIDIA H100 SXM5   |
| GB200       | NVIDIA GB200 NVL72 |
| GB300       | NVIDIA GB300 NVL72 |

## Quick Start

### Prerequisites

- Kubernetes 1.27+
- `kubectl` configured against your cluster
- Go 1.21+ (for local development)

### Install the CRD

```bash
make install
```

### Deploy the Operator

```bash
make deploy
```

### Apply a Sample RuntimePackage

```bash
make sample
```

Or inline:

```yaml
apiVersion: runtime.nvidia.com/v1alpha1
kind: RuntimePackage
metadata:
  name: nvidia-container-toolkit
  namespace: nvidia-system
spec:
  packageName: nvidia-container-toolkit
  version: "1.14.6"
  targetArchitectures: [H100]
  nodeSelector:
    nvidia.com/gpu.present: "true"
  autoUpgrade: true
```

Check status:

```bash
kubectl get rtpkg -n nvidia-system
# NAME                      PACKAGE                   VERSION  PHASE  READY  TOTAL
# nvidia-container-toolkit  nvidia-container-toolkit  1.14.6   Ready  3      3
```

### Install via Helm

```bash
helm install nvidia-runtime-operator helm/nvidia-runtime-operator \
  --namespace nvidia-system \
  --create-namespace
```

## Development

```bash
make test        # run unit tests with race detection
make build       # compile the operator binary
make docker-push IMG=your-registry/nvidia-runtime-operator:dev
make manifests   # re-generate CRD YAML after type changes
make generate    # re-generate deepcopy methods after type changes
```

## Architecture

### CRD: `RuntimePackage`

| Field                       | Description                                         |
|-----------------------------|-----------------------------------------------------|
| `spec.packageName`          | Package name (e.g. `nvidia-container-toolkit`)      |
| `spec.version`              | Semver version string                               |
| `spec.targetArchitectures`  | GPU architectures (A100, H100, GB200, GB300)        |
| `spec.nodeSelector`         | Labels filtering target nodes                       |
| `spec.autoUpgrade`          | Roll a new DaemonSet image when version changes     |
| `spec.validationScript`     | Post-install script (e.g. `nvidia-smi` smoke test)  |

### Controller Reconcile Loop

```
Reconcile(RuntimePackage)
  ├── Count nodes matching NodeSelector
  ├── Get installer DaemonSet
  │   ├── Not found → Create DaemonSet  → status: Installing
  │   ├── Image outdated → Update DS    → status: Upgrading
  │   └── DS ready == desired           → status: Ready
  └── Requeue every 30s until Ready
```

### RBAC

Minimum-privilege `ClusterRole`:
- Full CRUD on `runtimepackages` and owned `daemonsets`
- Read-only on `nodes` (counting targets)
- Write `events` for observability

## Project Structure

```
├── api/v1alpha1/       # CRD type definitions and deepcopy
├── controllers/        # Reconciler implementation and unit tests
├── config/
│   ├── crd/            # CRD YAML manifest
│   ├── rbac/           # ClusterRole, ClusterRoleBinding, ServiceAccount
│   ├── manager/        # Operator Deployment manifest
│   └── samples/        # Example RuntimePackage resources (H100, GB200/GB300)
├── helm/               # Helm chart for production deployment
├── Dockerfile          # Distroless multi-arch build
└── Makefile            # Build, test, deploy targets
```

## License

Apache 2.0
