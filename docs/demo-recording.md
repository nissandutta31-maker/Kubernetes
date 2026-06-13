# Demo Recording Script (60–90 seconds)

Use this script to record a screen capture for LinkedIn, your resume, or outreach emails.

## Before recording

1. Fresh terminal, readable font size (14–16pt)
2. Cluster torn down or run `make clean && make kind-down` for a clean start
3. Tools installed: Docker, Kind, kubectl, Python 3.10+

## Recording flow

| Time | Action | What to say (optional voiceover) |
|---|---|---|
| 0:00 | `make all` | "Full pipeline: Kind cluster, Docker build, image load, deploy, verify." |
| 0:20 | Show `make verify` output | "Python automation checks pod phase and HTTP health via the service." |
| 0:35 | New terminal: `make port-forward` | "ClusterIP service exposed locally on 8080." |
| 0:45 | `curl http://localhost:8080/` and `curl http://localhost:8080/health` | "App responds on `/` and `/health` — same paths used by K8s probes." |
| 0:55 | Show `kubectl get pods -n nvidia-runtime-demo` | "Two replicas running with resource limits and probes." |
| 1:05 | One slide or terminal note | "GPU manifest included; Pending on Kind without GPUs — documented honestly in docs/gpu-workloads.md." |

## Where to host

- YouTube (Unlisted) or Loom
- Add the link to README under **Demo Verified** and LinkedIn Featured

## Expected `make verify` output (abbreviated)

```
────────────────────────────────────────────────────────────
  NVIDIA DGX Cloud — Pod Health Report
  Namespace: nvidia-runtime-demo
────────────────────────────────────────────────────────────
  POD NAME                                 PHASE        NODE
  ...
  🎉 All 2 pods are healthy and running!

  HTTP /health check via nvidia-demo-svc ... OK
────────────────────────────────────────────────────────────
```

Replace `YOUR_DEMO_VIDEO_URL` in the README once uploaded.
