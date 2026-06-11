#!/usr/bin/env python3
"""
DGX Cloud Runtime Health Check

Queries the Kubernetes API for pods in a namespace and reports their
running status. Exits with code 1 when any pod is not Running.
"""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from typing import Any


def run_kubectl(args: list[str]) -> subprocess.CompletedProcess[str]:
    """Execute a kubectl command and return the CompletedProcess."""
    return subprocess.run(
        ["kubectl", *args],
        capture_output=True,
        text=True,
        timeout=15,
        check=False,
    )


def check_kubectl_installed() -> None:
    """Verify kubectl is available on PATH."""
    try:
        result = subprocess.run(
            ["kubectl", "version", "--client"],
            capture_output=True,
            text=True,
            check=False,
        )
    except FileNotFoundError:
        sys.exit(
            "kubectl is not installed or not on your PATH.\n"
            "Install it: https://kubernetes.io/docs/tasks/tools/"
        )

    if result.returncode != 0:
        sys.exit("kubectl is installed but returned an error. Check your configuration.")


def get_pods(namespace: str) -> list[dict[str, Any]]:
    """Retrieve pods in the given namespace as a list of dicts."""
    result = run_kubectl(["get", "pods", "-n", namespace, "-o", "json"])
    if result.returncode != 0:
        msg = result.stderr.strip()
        lowered = msg.lower()
        if "connection refused" in lowered or "no such host" in lowered:
            sys.exit("Cannot reach the Kubernetes cluster. Is your cluster running?")
        sys.exit(f"kubectl error:\n{msg}")

    data = json.loads(result.stdout)
    return data.get("items", [])


def summarize(pods: list[dict[str, Any]]) -> dict[str, int]:
    """Count pods by phase and return a summary dict."""
    counts: dict[str, int] = {}
    for pod in pods:
        phase = pod.get("status", {}).get("phase", "Unknown")
        counts[phase] = counts.get(phase, 0) + 1
    return counts


def format_output(namespace: str, pods: list[dict[str, Any]]) -> tuple[int, int]:
    """Print a human-readable summary and return (running, total) counts."""
    divider = "-" * 60

    print(f"\n{divider}")
    print("  NVIDIA DGX Cloud — Pod Health Report")
    print(f"  Namespace: {namespace}")
    print(divider)

    if not pods:
        print("  WARNING: No pods found in this namespace.")
        print(f"{divider}\n")
        return 0, 0

    summary = summarize(pods)

    print(f"  {'POD NAME':<40} {'PHASE':<12} {'NODE'}")
    print(f"  {'-' * 38:<40} {'-' * 10:<12} {'-' * 20}")

    for pod in pods:
        name = pod.get("metadata", {}).get("name", "unknown")
        phase = pod.get("status", {}).get("phase", "Unknown")
        node = pod.get("spec", {}).get("nodeName") or "unassigned"
        marker = "OK" if phase == "Running" else ".."
        print(f"  [{marker}] {name:<36} {phase:<12} {node}")

    print(f"\n{divider}")
    print("  SUMMARY")
    print(divider)

    total = len(pods)
    running = summary.get("Running", 0)

    for phase, count in sorted(summary.items()):
        bar = "#" * min(count * 10, 40)
        print(f"  {phase:<14} {count:>3}  {bar}")

    print(f"  {'-' * 42}")
    print(f"  {'Total':<14} {total:>3}")

    if running == total and total > 0:
        print(f"\n  All {running} pods are healthy and running.")
    else:
        print(f"\n  {running}/{total} pods are in Running state.")

    print(f"{divider}\n")
    return running, total


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Validate Kubernetes pod health.")
    parser.add_argument(
        "--namespace",
        default="nvidia-runtime-demo",
        help="Kubernetes namespace to inspect (default: nvidia-runtime-demo)",
    )
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    check_kubectl_installed()
    pods = get_pods(args.namespace)
    running, total = format_output(args.namespace, pods)

    if total == 0 or running != total:
        sys.exit(1)


if __name__ == "__main__":
    main()
