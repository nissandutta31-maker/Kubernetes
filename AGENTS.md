# Agent Instructions

## Model selection: DeepSeek V4 Pro and Cloud Agents

DeepSeek V4 Pro cannot run on **Cursor Cloud Agents**. Cloud Agents only support Cursor's curated Max Mode model list (Claude, GPT, Gemini, Composer, Grok, Kimi, etc.). DeepSeek is not on that list.

### Option A — Use DeepSeek V4 Pro locally

1. In Cursor desktop, switch the agent dropdown from **Cloud** to **Local**.
2. Open **Settings → Models → Add Custom Model**.
3. Enter the model ID exactly: `deepseek-v4-pro` (or `deepseek-v4-flash` for a cheaper/faster tier).
4. Set **Base URL** to `https://api.deepseek.com` (do not add `/v1`; Cursor appends it).
5. Paste your DeepSeek API key and click **Verify**.
6. Update Cursor to the latest stable version to avoid known "model not found" resume bugs with BYOK models.

If multi-turn agent sessions fail with `reasoning_content` errors on `deepseek-v4-pro`, try `deepseek-v4-flash` for simpler tasks, or route through a local Ollama proxy.

### Option B — Use Cloud Agents with a supported model

1. Keep **Cloud** selected in the agent dropdown.
2. Choose a supported Max Mode model (e.g. Claude Opus/Sonnet, GPT-5.x, Gemini 3.x, Composer 2.5).
3. Ensure API usage and a Cloud Agent spend limit are configured in your Cursor dashboard.

## Cursor Cloud specific instructions

This repo is a Kubernetes runtime demo. Cloud agents should use a supported model (Option B above), not DeepSeek.

### Prerequisites installed in the cloud environment

The `.cursor/Dockerfile` provides Go 1.22, Docker, kubectl, kind, and Python 3.

### Common commands

```bash
# Build the app image
make build

# Create a local Kind cluster, build image, load into cluster, deploy, and verify
make all

# Step by step
make kind-up
make load-image   # build + load image into kind (required before deploy)
make deploy
make verify

# Expose the service locally inside the VM
make port-forward   # forwards svc/nvidia-demo-svc to localhost:8080
```

### Verification

After `make deploy`, run `make verify`. All pods in the `nvidia-runtime-demo` namespace should report `Running`. If pods are missing or not healthy, `make verify` exits with a non-zero status.

### Notes

- Kind requires Docker. The cloud environment starts the Docker daemon on boot.
- `make kind-up` is idempotent; it tolerates an existing cluster.
- `make deploy` automatically builds the image and runs `kind load docker-image` so the cluster can pull `nvidia-demo-app:latest`.
- The health check script (`automation/health_check.py`) needs a reachable Kubernetes API — run `make kind-up` and `make deploy` first.

### Known limitation: Kind / Kubernetes may not boot in this cloud VM

`make kind-up` (and therefore `make deploy`, `make verify`, `make all`) can fail in the Cursor Cloud VM with:

```
ERROR: failed to create cluster: could not find a log line that matches "Reached target .*Multi-User System.*|detected cgroup v1"
# kind node log: Failed to create /init.scope control group: Structure needs cleaning
```

Root cause is environmental, not a repo bug: the VM's cgroup namespace root is `domain threaded` (`cat /sys/fs/cgroup/cgroup.type` → `domain threaded`), so any child cgroup is `domain invalid` and the systemd-based Kind node container cannot initialize its cgroup hierarchy. This is set by the outer Firecracker/Cloud host and cannot be changed from inside the VM (no grub/reboot access). Plain (non-systemd) containers are unaffected — Docker, `docker build`, and `docker run` all work normally.

What still works and how to verify the app without a cluster:

```bash
make build                                   # build the image
docker run -d --name demo -p 8080:8080 nvidia-demo-app:latest
curl http://localhost:8080/                  # -> Hello from NVIDIA DGX Cloud Runtime Pod!
curl http://localhost:8080/health            # -> OK
```

The Go CI checks (`go build -v ./...`, `go vet ./...` in `app/`) and YAML manifests under `k8s/` are independent of the cluster and run fine. If you need a real cluster on this host, you would need a non-systemd k8s runtime (e.g. k3s/k3d), which is outside this project's Kind-based workflow.
