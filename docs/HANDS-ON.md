# Hands-On: Run and Understand the NVIDIA Runtime Operator

> **Who this is for:** someone new to Kubernetes who wants to *actually run* this
> operator, understand every piece, and be able to explain it in an interview.
> No prior Kubernetes experience assumed. Budget ~60–90 minutes the first time.

**The goal is not "have a repo." It's "be able to run this and explain it cold."**
A recruiter or engineer at NVIDIA will ask you to walk through it. This guide gets
you to where you can.

---

## 0. The 90-second mental model

A **Kubernetes operator** automates a human operator's job. You tell Kubernetes
*what you want* ("nvidia-container-toolkit v1.14.6 installed on every GPU node"),
and a program called a **controller** continuously makes reality match that wish.

This project implements exactly that for GPU runtime packages:

```
You write a RuntimePackage object  ──►  apiserver stores it
                                         │
                       the controller watches it
                                         │
        "is there an installer DaemonSet for this package?"
                          ┌──────────────┴──────────────┐
                       no │                              │ yes, but wrong version
                  create DaemonSet                  update its image
                  status: Installing                status: Upgrading
                          └──────────────┬──────────────┘
                            all node pods ready?
                                         │ yes
                                  status: Ready
```

A **DaemonSet** is a Kubernetes object that runs exactly one copy of a pod on every
matching node — perfect for "install this on every GPU node."

That's the whole idea. Everything below makes it concrete.

---

## 1. Install the tools

You need four command-line tools. Pick your OS.

| Tool | What it is |
|------|-----------|
| **Go** | the language the operator is written in |
| **Docker** | runs containers; `kind` uses it to fake a cluster |
| **kind** | "Kubernetes IN Docker" — a real cluster on your laptop |
| **kubectl** | the command you use to talk to a cluster |

### macOS (with [Homebrew](https://brew.sh))
```bash
brew install go kubectl kind
brew install --cask docker   # then launch Docker Desktop once
```

### Linux (Debian/Ubuntu)
```bash
# Go
sudo snap install go --classic        # or: https://go.dev/dl
# Docker
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker "$USER"        # log out/in so this takes effect
# kubectl
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -m 0755 kubectl /usr/local/bin/kubectl
# kind
curl -Lo kind https://github.com/kubernetes-sigs/kind/releases/latest/download/kind-linux-amd64
sudo install -m 0755 kind /usr/local/bin/kind
```

### Verify
```bash
go version && docker version && kind version && kubectl version --client
```

> **No Docker?** You can still run a real demo with **Track A** below (envtest),
> which needs only Go. Track B (kind) needs Docker.

---

## 2. Track A — prove it works without Docker (envtest)

This is the fastest way to see the operator drive a `RuntimePackage` through its
whole lifecycle against a **real Kubernetes API server** — no cluster, no Docker.
`envtest` downloads the actual `kube-apiserver` and `etcd` binaries and runs them
locally for the test.

```bash
make test-integration
```

You'll see output like:

```
✓ reconcile #1 created DaemonSet "runtime-pkg-container-toolkit" with image "nvcr.io/nvidia/k8s/nvidia-container-toolkit-installer:1.14.6"
✓ DaemonSet is owned by the RuntimePackage (cascade delete wired)
✓ status: phase=Installing totalNodes=2 (real /status subresource)
✓ reconcile #2: phase=Ready installedVersion=1.14.6 readyNodes=2/2
✓ reconcile #3: version bump rolled DaemonSet image to "...:1.15.0", phase=Upgrading
✓ API server rejected invalid version as expected: spec.version in body should match '^\d+\.\d+\.\d+.*$'
PASS
```

**What just happened (and what to say in an interview):**

- The operator installed the real CRD into a real API server, then created a
  `RuntimePackage`. The controller's `Reconcile` function created an installer
  **DaemonSet** for it. *(This is the core of any operator: a reconcile loop.)*
- It set an **owner reference** so deleting the `RuntimePackage` cascades to its
  DaemonSet. *("Kubernetes garbage-collects owned objects automatically.")*
- It counted matching **nodes** and reported `Installing`, then `Ready` once the
  DaemonSet's pods were ready, writing to the `/status` **subresource**.
- Bumping `spec.version` rolled the DaemonSet's image → `Upgrading`.
- The **API server itself rejected** a bad version, because the CRD carries
  validation rules (`pattern: ^\d+\.\d+\.\d+.*$`). *("Validation lives in the CRD
  schema, enforced server-side — not just in my code.")*

If that passed, **the operator genuinely works.** Now let's see it on a real cluster.

---

## 3. Track B — the full demo on a real cluster (kind)

### The one-command version

```bash
make demo
```

This script (`hack/demo.sh`) creates a real 3-node Kubernetes cluster, runs the
operator, applies a package, drives it to `Ready`, then does a rolling upgrade —
narrating each step. Great for a screen recording to send a recruiter.

### The do-it-yourself version (recommended for learning)

Type these yourself in **two terminals** — you'll learn far more. Each step says
what it does.

**Terminal 1 — build the cluster and run the operator:**

```bash
# 1. Create a real Kubernetes cluster (3 nodes) inside Docker.
kind create cluster --config hack/kind-cluster.yaml --wait 120s

# 2. See your nodes. The two "workers" will stand in for GPU machines.
kubectl get nodes

# 3. Label the workers so the operator treats them as GPU nodes.
#    (On a real cluster, NVIDIA's GPU Feature Discovery adds this label.)
kubectl label nodes -l '!node-role.kubernetes.io/control-plane' \
  nvidia.com/gpu.present=true --overwrite

# 4. Teach the cluster the new resource type, and set up permissions.
#    Create the namespace FIRST — the ServiceAccount in config/rbac/ lives in it.
kubectl create namespace nvidia-system
kubectl apply -f config/crd/
kubectl apply -f config/rbac/

# 5. Run the operator. It stays in the foreground printing logs — leave it.
make run
```

**Terminal 2 — drive it and watch:**

```bash
# 6. Declare what you want: a runtime package on every GPU node.
kubectl apply -f config/samples/demo_runtimepackage.yaml

# 7. Watch the operator reconcile it. Re-run this a few times.
kubectl get rtpkg -n nvidia-system
#   NAME                    PACKAGE                   VERSION  PHASE       READY  TOTAL
#   demo-container-toolkit  nvidia-container-toolkit  1.14.6   Installing  0      2
#   ...a few seconds later...
#   demo-container-toolkit  nvidia-container-toolkit  1.14.6   Ready       2      2

# 8. Look at what the operator CREATED for you — a DaemonSet and one pod per node.
kubectl get daemonset,pods -n nvidia-system -o wide

# 9. Read the full status the operator wrote (phase, conditions, node counts).
kubectl describe rtpkg demo-container-toolkit -n nvidia-system

# 10. Now trigger an upgrade just by changing the desired version.
kubectl patch rtpkg demo-container-toolkit -n nvidia-system \
  --type=merge -p '{"spec":{"version":"1.15.0"}}'

# 11. Watch Terminal 1's logs AND the phase flip to Upgrading, then Ready.
kubectl get rtpkg -n nvidia-system -w     # Ctrl-C to stop watching
```

**Clean up when done:**
```bash
kind delete cluster --name nvidia-demo
```

> **Note:** the demo uses `installerImage: busybox:1.36` (see the sample file) so
> the pods actually run on your laptop. The real NVIDIA installer images live in a
> private registry (`nvcr.io`) that needs NGC credentials. The operator logic is
> identical either way — only the image differs. **This is a great thing to point
> out in an interview:** you understood the difference and made it configurable.

---

## 4. Read the code — so you can explain it

Open these five files in order. This is the tour to give an interviewer.

### 1) `api/v1alpha1/types.go` — *the API you invented*
Defines the `RuntimePackage` resource: its `Spec` (what you want) and `Status`
(what's true). The `// +kubebuilder:` comments are **markers** that generate the
CRD's validation schema and the `kubectl` columns. Look at `RuntimePackageSpec`
and `RuntimePackageStatus`.

> Say: *"I designed a declarative API. Spec is desired state, Status is observed
> state — the fundamental Kubernetes split."*

### 2) `controllers/runtimepackage_controller.go` — *the brain*
The `Reconcile` method is the heart. Trace it top to bottom:
- `Get` the `RuntimePackage` (if gone, nothing to do — return).
- `countMatchingNodes` — how many GPU nodes match.
- `Get` the installer DaemonSet: **not found → create it; found → sync it.**
- `patchStatus` writes phase + conditions and requeues every 30s until `Ready`.

> Say: *"Reconcile is **idempotent** and **level-triggered** — it computes desired
> state from scratch every time and is safe to run repeatedly. I never assume what
> changed; I just converge."* (This is THE sentence that signals you understand
> operators.)

Two more things worth pointing to here:
- `SetupWithManager` calls `.Owns(&appsv1.DaemonSet{})` — so the controller also
  wakes up when its DaemonSet changes, not only when the `RuntimePackage` changes.
- `setCondition` preserves `LastTransitionTime` when the status doesn't change —
  per Kubernetes API conventions.

### 3) `config/crd/runtime.nvidia.com_runtimepackages.yaml` — *the contract*
The generated CRD. This is what makes the API server understand `kind:
RuntimePackage` and enforce validation (the `pattern`, `enum`, `minItems` rules).

### 4) `main.go` — *the wiring*
Sets up the controller-runtime **Manager** (the thing that runs controllers),
health probes, leader election, and registers the reconciler.

### 5) `controllers/*_test.go` — *the proof*
- `runtimepackage_controller_test.go`: fast **unit tests** with a fake client.
- `integration_test.go`: **integration tests** against a real API server (Track A).

---

## 5. Concept cheat-sheet (memorize before an interview)

| Term | One-liner |
|------|-----------|
| **Pod** | smallest deployable unit; one or more containers that share a network. |
| **Node** | a machine (VM/physical) that runs pods. |
| **Namespace** | a virtual folder that groups/​isolates objects. |
| **Deployment** | runs N identical pods, anywhere; self-heals. |
| **DaemonSet** | runs exactly one pod **per node** — used here for per-node installs. |
| **CRD** | Custom Resource Definition: teaches the API server a new object type. |
| **CR** | an instance of a CRD (your `RuntimePackage` object). |
| **Controller / Operator** | a loop that drives real state toward the spec. |
| **Reconcile** | the function that does one round of "make reality match desire." |
| **Idempotent** | safe to run repeatedly with the same result. |
| **Owner reference** | links a child object to its parent for cascade-delete. |
| **Status subresource** | a separate write path for `.status`, so spec & status don't clobber each other. |
| **RBAC** | role-based access control: what the operator is allowed to touch. |
| **Helm** | a package manager for Kubernetes YAML (the chart in `helm/`). |

---

## 6. How this maps to the actual NVIDIA job

The posting asks for *"controller systems that manage runtime components"* and
*"automate runtime installation and upgrade."* Your project is a small, honest
model of exactly that:

- **Their GPU Operator** uses a `ClusterPolicy`/`NVIDIADriver` CRD + per-node
  DaemonSets to install drivers and the container toolkit. **Your operator** uses
  a `RuntimePackage` CRD + per-node DaemonSets to install runtime packages.
- **Their DRA driver** handles GB200/GB300 multi-node NVLink. **Your** CRD has a
  `GB200`/`GB300` architecture enum and your image is built multi-arch (arm64),
  because Grace Blackwell nodes are ARM.

See `../nvidia-internship-research-report.md` for the deep dive and the exact
talking points.

---

## 7. Troubleshooting

| Symptom | Fix |
|---------|-----|
| `make test-integration` fails to download | corporate proxy/firewall; try from a home network. It fetches from `github.com`. |
| `kind create cluster` hangs or errors on image pull | Docker isn't running, or Docker Hub is blocked on your network. Start Docker Desktop / try another network. |
| `make run` → "connection refused" | no cluster reachable. Create a kind cluster first, or check `kubectl config current-context`. |
| Package stuck at `Installing`, pods `ImagePullBackOff` | you used a real `nvcr.io` image without NGC auth. Use `config/samples/demo_runtimepackage.yaml` (busybox) for local runs. |
| `kubectl get rtpkg` → "no resources"/unknown type | install the CRD first: `kubectl apply -f config/crd/`. |
| Permissions errors from the operator | apply RBAC: `kubectl apply -f config/rbac/`. |

---

## 8. What to do next (to genuinely level up)

1. **Break it on purpose.** Delete the DaemonSet (`kubectl delete ds -n
   nvidia-system runtime-pkg-demo-container-toolkit`) and watch the operator
   recreate it. That's reconciliation — *feel* it.
2. **Add a field.** Try adding `spec.priority` to `types.go` + the CRD, rebuild,
   and use it. You'll learn the generate→apply loop.
3. **Read one real operator.** Skim
   [github.com/NVIDIA/gpu-operator](https://github.com/NVIDIA/gpu-operator) — you'll
   recognize the same shapes you just built.
4. **Record a 3-minute demo** of `make demo` and link it when you message
   recruiters. Showing > telling.
