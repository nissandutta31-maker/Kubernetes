# AGENTS.md

## Cursor Cloud specific instructions

This repo is the **NVIDIA DGX Cloud Kubernetes Runtime Demo**: a Go HTTP microservice
(`app/main.go`) that is containerized (`Dockerfile`), deployed to a local Kubernetes
cluster via `k8s/*.yaml`, and validated by a Python script (`automation/health_check.py`).
The `Makefile` wraps the workflow (`build`, `kind-up`, `deploy`, `verify`, `port-forward`).

### Toolchain
- Go 1.22, Python 3.12, Docker, `kubectl`, and `kind` are pre-installed in the VM snapshot.
- There are **no third-party dependencies**: `app/go.mod` has no `require`s and the Python
  script uses only the stdlib. The update script (`go mod download`) is therefore essentially
  a no-op cache warm.

### Docker must be started manually each session
The Docker daemon is **not** auto-started (no systemd as PID 1). Start it once per session:
```bash
sudo dockerd > /tmp/dockerd.log 2>&1 &   # prefer a tmux session for long-lived runs
sleep 8 && sudo chmod 666 /var/run/docker.sock   # let the 'ubuntu' user talk to docker
```
`/etc/docker/daemon.json` is already configured for this VM with
`{"storage-driver": "fuse-overlayfs", "ip6tables": false}` and iptables is set to legacy mode.
`ip6tables: false` is required — without it Docker network creation fails (`ip6tables ... table 'raw' does not exist`).

### Lint / test / build / run (what works here)
- **Lint/test (matches CI `.github/workflows/go.yml`):** `cd app && go build -v ./... && go vet ./...`
- **Build image:** `make build` (builds `nvidia-demo-app:latest`).
- **Run the app:** the service is headless HTTP. Run the container directly and curl it:
  ```bash
  docker run -d --name demo --rm -p 8080:8080 nvidia-demo-app:latest
  curl http://localhost:8080/        # -> "Hello from NVIDIA DGX Cloud Runtime Pod!"
  curl http://localhost:8080/health  # -> "OK"
  ```

### IMPORTANT: nested Kubernetes (kind / k3d / k3s / minikube) does NOT work in this VM
`make kind-up` / `make deploy` / `make verify` cannot run here. The cgroup-v2 namespace root
is `domain threaded` (set by the Firecracker host), so the `memory` controller cannot be
delegated into `cgroup.subtree_control` (ENOTSUP) and child cgroups are `domain invalid`.
As a result, systemd inside a `kind` node container fails to boot:
`Failed to create /init.scope control group: Structure needs cleaning`. This is a known
Cursor Cloud limitation (kube-proxy/Service networking is also unsupported due to missing
`xt_comment`/nftables kernel modules). Do not spend time retrying cluster creation; demonstrate
the K8s layer by validating manifests and running the container directly instead.

### Repo quirks (do not "fix" unless asked)
- `Dockerfile` runs the Go build line twice (harmless, just redundant).
- `Makefile` `port-forward` targets `svc/nvidia-demo-app`, but `k8s/service.yaml` names the
  Service `nvidia-demo-svc`. If a cluster ever runs, port-forward to `svc/nvidia-demo-svc`.
- `make deploy` does not `kind load` the locally built image, so on a real kind cluster the
  pods would `ImagePullBackOff` until you run `kind load docker-image nvidia-demo-app:latest --name nvidia-demo`.
