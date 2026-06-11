# AGENTS.md

## Cursor Cloud specific instructions

### Product overview

Single Go HTTP microservice (`app/main.go`) packaged as a Docker image and deployed to Kubernetes via manifests in `k8s/`. See `README.md` for architecture and Makefile targets.

### Services

| Service | How to run | Notes |
|---|---|---|
| Go app (local) | `cd app && go run main.go` | Listens on `:8080`; optional `PORT` env var |
| Docker image | `make build` or `sudo docker build -t nvidia-demo-app:latest .` | Requires Docker daemon |
| K8s cluster (Kind) | `make kind-up` | See **Kubernetes limitations** below |
| Load image into Kind | `make load-image` | Required after `make build` before deploy on Kind |
| Deploy + verify | `make deploy && make verify` | Needs a running cluster |
| Fast local build | `make build-local` | Builds `bin/server` without Docker |
| Port-forward | `make port-forward` | Forwards `nvidia-demo-svc` to `localhost:8080` |

### Lint / test / build

Matches CI in `.github/workflows/go.yml`:

```bash
cd app && go build -v ./... && go vet ./...
```

There is no separate linter config or automated test suite in this repo.

### Docker daemon (required for `make build`)

Docker is not managed by systemd in this Cloud Agent VM. Start it manually in a tmux session:

```bash
SESSION_NAME="dockerd"
tmux -f /exec-daemon/tmux.portal.conf has-session -t "=$SESSION_NAME" 2>/dev/null || \
  tmux -f /exec-daemon/tmux.portal.conf new-session -d -s "$SESSION_NAME" -c "$PWD" -- "${SHELL:-zsh}" -l
tmux -f /exec-daemon/tmux.portal.conf send-keys -t "dockerd:0.0" 'sudo dockerd' C-m
```

Use `sudo docker …` until the shell picks up the `docker` group, or run `newgrp docker`.

Daemon config at `/etc/docker/daemon.json` should include `"storage-driver": "fuse-overlayfs"` and `"ip6tables": false` for nested-container networking.

### Kubernetes limitations in Cloud Agent VMs

Local Kind/k3d clusters **fail** in this environment because nested containers cannot access delegated memory cgroups (`failed to find memory cgroup (v2)`). Kind also hits missing `ip6tables` kernel modules unless Docker `ip6tables` is disabled.

**Workaround for app development:** build and run the container directly:

```bash
make build   # or sudo docker build -t nvidia-demo-app:latest .
sudo docker run -d --name nvidia-demo-local -p 8080:8080 nvidia-demo-app:latest
curl http://localhost:8080/
curl http://localhost:8080/health
```

Full `make all` (Kind → deploy → verify) requires a host with proper cgroup delegation and kernel module support, not available in the current Cloud Agent nested Docker setup.

### Environment variables

Only `PORT` (default `8080`) is used. No secrets or `.env.example` file.
