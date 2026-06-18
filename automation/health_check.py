#!/usr/bin/env python3
"""
DGX Cloud Runtime Health Check

Queries the Kubernetes API for all pods in the nvidia-runtime-demo
namespace and reports their running status. Designed to validate
cluster state after a deployment.
"""

import json
import subprocess
import sys
from typing import Any


def run_kubectl(args: list[str]) -> subprocess.CompletedProcess:
    """Execute a kubectl command and return the CompletedProcess."""
    return subprocess.run(
        ["kubectl"] + args,
        capture_output=True,
        text=True,
        timeout=15,
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


def format_output(namespace: str, pods: list[dict[str, Any]]) -> None:
    """Print a clean, human-readable summary."""
    divider = "─" * 60

    print(f"\n{divider}")
    print(f"  NVIDIA DGX Cloud — Pod Health Report")
    print(f"  Namespace: {namespace}")
    print(f"{divider}")

    if not pods:
        print("  ⚠️  No pods found in this namespace.")
        print(f"{divider}\n")
        sys.exit(1)

    summary = summarize(pods)

    # Print per-pod details
    print(f"  {'POD NAME':<40} {'PHASE':<12} {'NODE'}")
    print(f"  {'─'*38:<40} {'─'*10:<12} {'─'*20}")

    for pod in pods:
        name = pod.get("metadata", {}).get("name", "unknown")
        phase = pod.get("status", {}).get("phase", "Unknown")
        node = pod.get("spec", {}).get("nodeName", "unassigned") or "unassigned"

        icon = "✅" if phase == "Running" else "⏳"
        print(f"  {icon} {name:<38} {phase:<12} {node}")

    # Print summary counts
    print(f"\n{divider}")
    print(f"  SUMMARY")
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
        sys.exit(1)

    print(f"{divider}\n")


def main() -> None:
    namespace = "nvidia-runtime-demo"

    check_kubectl_installed()
    pods = get_pods(namespace)
    format_output(namespace, pods)


if __name__ == "__main__":
    main()
