# AGENTS.md

## Cursor Cloud specific instructions

### What this repo is
A single product: a minimal Go HTTP microservice (`app/main.go`) that is containerized
(`Dockerfile`) and normally deployed to a local Kubernetes cluster via the manifests in
`k8s/`, with `automation/health_check.py` validating pod health. There is no frontend and
no database. The service exposes `GET /` ("Hello from NVIDIA DGX Cloud Runtime Pod!") and
`GET /health` ("OK") on `PORT` (default `8080`).

### Lint / test / build / run
Standard commands are documented in `README.md` (Makefile reference) and
`.github/workflows/go.yml` (CI). In short:
- Lint/test (mirrors CI): `cd app && go build ./... && go vet ./...` — no external Go deps.
- Build image: `make build` (produces `nvidia-demo-app:latest`).
- Run (dev): `cd app && PORT=8080 go run main.go`, then `curl localhost:8080/`.
- Run (container): `docker run -d -p 8080:8080 nvidia-demo-app:latest`, then `curl localhost:8080/`.

### Docker daemon must be started manually
The Docker daemon is not running on a fresh VM. Start it before `make build`/`docker`:
`sudo dockerd > /tmp/dockerd.log 2>&1 &` (run it in a tmux session so it persists). The
`ubuntu` user is already in the `docker` group; if the socket still rejects access in the
current shell, `sudo chmod 666 /var/run/docker.sock`. The daemon is pre-configured for this
VM via `/etc/docker/daemon.json` (`fuse-overlayfs` storage driver, `iptables-legacy`).

### Kind / Kubernetes does NOT work in this Cloud VM (important)
`make kind-up`, `make deploy`, `make verify`, `make port-forward`, and `make all` all need
a live cluster, but **Kind cannot start here**. This VM boots with the root cgroup as
`domain threaded`, so the `memory`/`io` cgroup-v2 controllers cannot be delegated
(`echo +memory > .../cgroup.subtree_control` returns `ENOTSUP`). The Kind node's systemd
then fails with `Failed to create /init.scope control group: Structure needs cleaning`, and
cluster creation aborts. This is the documented Cursor Cloud limitation — full Kubernetes
(node systemd, kube-proxy / Service networking) is not supported in Cloud Agent VMs. Do not
spend time trying to make Kind/k3d/k3s work.

To demonstrate the product, build the image and run it directly with Docker (see above) and
`curl` `/` and `/health`. `automation/health_check.py` only works against a reachable
cluster, so it cannot be exercised here.

### Minor repo gotcha (only relevant if a cluster were available)
`make port-forward` targets `svc/nvidia-demo-app`, but `k8s/service.yaml` names the Service
`nvidia-demo-svc`. The deployment uses a local image tag with `imagePullPolicy: IfNotPresent`
but the Makefile does not `kind load docker-image`, so on a real cluster the image would need
to be loaded into the node first.
