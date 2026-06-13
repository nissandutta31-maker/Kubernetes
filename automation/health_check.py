#!/usr/bin/env python3
"""
DGX Cloud Runtime Health Check

Queries the Kubernetes API for all pods in the nvidia-runtime-demo
namespace, reports their running status, and validates HTTP /health
via the ClusterIP service from an in-cluster curl pod.
"""

import json
import subprocess
import sys
import time
import uuid
from typing import Any

NAMESPACE = "nvidia-runtime-demo"
APP_LABEL = "app=nvidia-demo-app"
SERVICE_NAME = "nvidia-demo-svc"
HEALTH_PATH = "/health"


def run_kubectl(args: list[str], timeout: int = 15) -> subprocess.CompletedProcess:
    """Execute a kubectl command and return the CompletedProcess."""
    return subprocess.run(
        ["kubectl"] + args,
        capture_output=True,
        text=True,
        timeout=timeout,
    )


def check_kubectl_installed() -> None:
    """Verify kubectl is available on PATH."""
    try:
        result = subprocess.run(
            ["kubectl", "version", "--client"],
            capture_output=True,
            text=True,
        )
        if result.returncode != 0:
            sys.exit("❌ kubectl is installed but returned an error. Check your configuration.")
    except FileNotFoundError:
        sys.exit(
            "❌ kubectl is not installed or not on your PATH.\n"
            "   Install it: https://kubernetes.io/docs/tasks/tools/"
        )


def get_pods(namespace: str) -> list[dict[str, Any]]:
    """Retrieve pods in the given namespace as a list of dicts."""
    result = run_kubectl(["get", "pods", "-n", namespace, "-o", "json"])
    if result.returncode != 0:
        msg = result.stderr.strip()
        if "connection refused" in msg.lower() or "no such host" in msg.lower():
            sys.exit("❌ Cannot reach the Kubernetes cluster. Is your cluster running?")
        sys.exit(f"❌ kubectl error:\n{msg}")

    data = json.loads(result.stdout)
    return data.get("items", [])


def summarize(pods: list[dict[str, Any]]) -> dict[str, int]:
    """Count pods by phase and return a summary dict."""
    counts: dict[str, int] = {}
    for pod in pods:
        phase = pod.get("status", {}).get("phase", "Unknown")
        counts[phase] = counts.get(phase, 0) + 1
    return counts


def check_http_health(namespace: str, service: str, path: str = HEALTH_PATH) -> bool:
    """Run an in-cluster curl pod against the service health endpoint."""
    pod_name = f"health-curl-{uuid.uuid4().hex[:8]}"
    url = f"http://{service}{path}"

    create = run_kubectl(
        [
            "run",
            pod_name,
            "-n",
            namespace,
            "--restart=Never",
            "--image=curlimages/curl:8.5.0",
            "--command",
            "--",
            "curl",
            "-sf",
            url,
        ],
        timeout=30,
    )
    if create.returncode != 0:
        print(f"  ❌ Failed to start health check pod: {create.stderr.strip()}")
        run_kubectl(["delete", "pod", pod_name, "-n", namespace, "--ignore-not-found"])
        return False

    for _ in range(30):
        phase_result = run_kubectl(
            ["get", "pod", pod_name, "-n", namespace, "-o", "jsonpath={.status.phase}"]
        )
        phase = phase_result.stdout.strip()
        if phase == "Succeeded":
            run_kubectl(["delete", "pod", pod_name, "-n", namespace, "--ignore-not-found"])
            return True
        if phase in ("Failed", "Unknown") and phase_result.returncode == 0:
            logs = run_kubectl(["logs", pod_name, "-n", namespace])
            if logs.stdout.strip():
                print(f"  curl output: {logs.stdout.strip()}")
            if logs.stderr.strip():
                print(f"  curl error: {logs.stderr.strip()}")
            run_kubectl(["delete", "pod", pod_name, "-n", namespace, "--ignore-not-found"])
            return False
        time.sleep(1)

    run_kubectl(["delete", "pod", pod_name, "-n", namespace, "--ignore-not-found"])
    print("  ❌ Timed out waiting for in-cluster HTTP health check.")
    return False


def format_output(namespace: str, pods: list[dict[str, Any]], http_ok: bool | None) -> bool:
    """Print a clean, human-readable summary. Returns True if all checks passed."""
    divider = "─" * 60
    all_ok = True

    print(f"\n{divider}")
    print("  NVIDIA DGX Cloud — Pod Health Report")
    print(f"  Namespace: {namespace}")
    print(f"{divider}")

    if not pods:
        print("  ⚠️  No pods found in this namespace.")
        print(f"{divider}\n")
        return False

    summary = summarize(pods)

    print(f"  {'POD NAME':<40} {'PHASE':<12} {'NODE'}")
    print(f"  {'─'*38:<40} {'─'*10:<12} {'─'*20}")

    for pod in pods:
        name = pod.get("metadata", {}).get("name", "unknown")
        phase = pod.get("status", {}).get("phase", "Unknown")
        node = pod.get("spec", {}).get("nodeName", "unassigned") or "unassigned"

        icon = "✅" if phase == "Running" else "⏳"
        print(f"  {icon} {name:<38} {phase:<12} {node}")

    print(f"\n{divider}")
    print("  SUMMARY")
    print(f"{divider}")

    total = len(pods)
    running = summary.get("Running", 0)

    for phase, count in sorted(summary.items()):
        bar = "█" * min(count * 10, 40)
        print(f"  {phase:<14} {count:>3}  {bar}")

    print(f"  {'─' * 42}")
    print(f"  {'Total':<14} {total:>3}")

    if running == total and total > 0:
        print(f"\n  🎉 All {running} pods are healthy and running!")
    else:
        print(f"\n  📊 {running}/{total} pods are in Running state.")
        all_ok = False

    print(f"\n{divider}")
    print("  HTTP HEALTH")
    print(f"{divider}")

    if http_ok is None:
        print("  ⏭️  Skipped (no Running pods for app deployment)")
        all_ok = False
    elif http_ok:
        print(f"  ✅ GET http://{SERVICE_NAME}{HEALTH_PATH} → OK")
    else:
        print(f"  ❌ GET http://{SERVICE_NAME}{HEALTH_PATH} failed")
        all_ok = False

    print(f"{divider}\n")
    return all_ok


def main() -> None:
    check_kubectl_installed()
    pods = [p for p in get_pods(NAMESPACE) if p.get("metadata", {}).get("labels", {}).get("app") == "nvidia-demo-app"]

    http_ok: bool | None = None
    if pods and all(p.get("status", {}).get("phase") == "Running" for p in pods):
        http_ok = check_http_health(NAMESPACE, SERVICE_NAME)

    all_ok = format_output(NAMESPACE, pods, http_ok)
    if not all_ok:
        sys.exit(1)


if __name__ == "__main__":
    main()
