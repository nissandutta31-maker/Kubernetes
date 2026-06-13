# Profile Materials (Resume + LinkedIn)

Copy and customize these for your resume and LinkedIn profile.

## One-line pitch

> Built a reproducible Kubernetes runtime demo (Go + Docker + Kind) with Python health automation and a documented GPU scheduling manifest; full deploy verified via Makefile in under 5 minutes.

## Resume bullet (pick one)

- Designed and deployed a 2-replica Go microservice on Kubernetes (Kind) with multi-stage Docker builds, liveness/readiness probes, and Python cluster health + HTTP validation automation.
- Documented DGX Cloud-style GPU workload scheduling (`nvidia.com/gpu`, node selectors, tolerations) with honest local-cluster limits and production deployment notes.

## LinkedIn Featured section

**Title:** DGX Cloud K8s Runtime Demo

**Description:**

End-to-end Kubernetes demo: Go HTTP service, multi-stage Alpine container, declarative manifests (Deployment, Service, Namespace), Makefile-driven workflow, and Python health checks against pod state and `/health`.

Includes optional GPU deployment manifest and production notes. Runs on local Kind without GPUs; GPU pod stays Pending by design until scheduled on a GPU node.

**Links:**
- GitHub: https://github.com/nissandutta31-maker/Kubernetes
- Demo video: [YOUR_DEMO_VIDEO_URL]

## Honest limits (use in conversations)

- Runs on Kind without GPU hardware
- GPU manifest demonstrates scheduling patterns, not CUDA kernel work
- Focus is platform/runtime automation aligned with DGX Cloud operations

## About section addition (2 sentences)

I build hands-on infrastructure demos to learn how GPU cloud platforms operate at scale. My current project packages a Go service into Kubernetes with automated verification and documented GPU scheduling patterns inspired by DGX Cloud workflows.
