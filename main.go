package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Kubernetes showcase")
		fmt.Println("Use: go run . generate-manifest [flags]")
		return
	}

	switch os.Args[1] {
	case "generate-manifest":
		runGenerateManifest(os.Args[2:])
	default:
		fmt.Printf("unknown command %q\n", os.Args[1])
		fmt.Println("Supported command: generate-manifest")
		os.Exit(1)
	}
}

type workloadConfig struct {
	Name      string
	Namespace string
	Image     string
	Replicas  int
	CPU       string
	Memory    string
	GPUs      int
	Port      int
}

func defaultWorkloadConfig() workloadConfig {
	return workloadConfig{
		Name:      "nvidia-gpu-demo",
		Namespace: "default",
		Image:     "nvcr.io/nvidia/k8s/cuda-sample:vectoradd-cuda12.5.0",
		Replicas:  1,
		CPU:       "500m",
		Memory:    "1Gi",
		GPUs:      1,
		Port:      8080,
	}
}

func runGenerateManifest(args []string) {
	cfg := defaultWorkloadConfig()
	fs := flag.NewFlagSet("generate-manifest", flag.ExitOnError)
	fs.StringVar(&cfg.Name, "name", cfg.Name, "workload name")
	fs.StringVar(&cfg.Namespace, "namespace", cfg.Namespace, "kubernetes namespace")
	fs.StringVar(&cfg.Image, "image", cfg.Image, "container image")
	fs.IntVar(&cfg.Replicas, "replicas", cfg.Replicas, "replica count")
	fs.StringVar(&cfg.CPU, "cpu", cfg.CPU, "cpu request")
	fs.StringVar(&cfg.Memory, "memory", cfg.Memory, "memory request")
	fs.IntVar(&cfg.GPUs, "gpus", cfg.GPUs, "nvidia.com/gpu limits and requests")
	fs.IntVar(&cfg.Port, "port", cfg.Port, "service and container port")
	outputPath := fs.String("out", "", "optional output path for YAML")
	_ = fs.Parse(args)

	if cfg.Replicas < 1 || cfg.GPUs < 1 || cfg.Port < 1 {
		fmt.Println("replicas, gpus, and port must be >= 1")
		os.Exit(1)
	}

	manifest := generateManifest(cfg)
	summary := measurableSummary(cfg)

	if *outputPath != "" {
		if err := os.WriteFile(*outputPath, []byte(manifest), 0o644); err != nil {
			fmt.Printf("failed to write manifest to %s: %v\n", *outputPath, err)
			os.Exit(1)
		}
		fmt.Printf("manifest written to %s\n", *outputPath)
	}

	fmt.Println(manifest)
	fmt.Println(summary)
}

func generateManifest(cfg workloadConfig) string {
	var b strings.Builder
	b.WriteString("apiVersion: apps/v1\n")
	b.WriteString("kind: Deployment\n")
	b.WriteString("metadata:\n")
	b.WriteString(fmt.Sprintf("  name: %s\n", cfg.Name))
	b.WriteString(fmt.Sprintf("  namespace: %s\n", cfg.Namespace))
	b.WriteString("spec:\n")
	b.WriteString(fmt.Sprintf("  replicas: %d\n", cfg.Replicas))
	b.WriteString("  selector:\n")
	b.WriteString("    matchLabels:\n")
	b.WriteString(fmt.Sprintf("      app: %s\n", cfg.Name))
	b.WriteString("  template:\n")
	b.WriteString("    metadata:\n")
	b.WriteString("      labels:\n")
	b.WriteString(fmt.Sprintf("        app: %s\n", cfg.Name))
	b.WriteString("    spec:\n")
	b.WriteString("      containers:\n")
	b.WriteString("        - name: gpu-worker\n")
	b.WriteString(fmt.Sprintf("          image: %s\n", cfg.Image))
	b.WriteString("          resources:\n")
	b.WriteString("            requests:\n")
	b.WriteString(fmt.Sprintf("              cpu: %s\n", cfg.CPU))
	b.WriteString(fmt.Sprintf("              memory: %s\n", cfg.Memory))
	b.WriteString(fmt.Sprintf("              nvidia.com/gpu: \"%d\"\n", cfg.GPUs))
	b.WriteString("            limits:\n")
	b.WriteString(fmt.Sprintf("              nvidia.com/gpu: \"%d\"\n", cfg.GPUs))
	b.WriteString("          ports:\n")
	b.WriteString(fmt.Sprintf("            - containerPort: %d\n", cfg.Port))
	b.WriteString("---\n")
	b.WriteString("apiVersion: v1\n")
	b.WriteString("kind: Service\n")
	b.WriteString("metadata:\n")
	b.WriteString(fmt.Sprintf("  name: %s-svc\n", cfg.Name))
	b.WriteString(fmt.Sprintf("  namespace: %s\n", cfg.Namespace))
	b.WriteString("spec:\n")
	b.WriteString("  selector:\n")
	b.WriteString(fmt.Sprintf("    app: %s\n", cfg.Name))
	b.WriteString("  ports:\n")
	b.WriteString("    - protocol: TCP\n")
	b.WriteString(fmt.Sprintf("      port: %d\n", cfg.Port))
	b.WriteString(fmt.Sprintf("      targetPort: %d\n", cfg.Port))
	b.WriteString("  type: ClusterIP\n")
	return b.String()
}

func measurableSummary(cfg workloadConfig) string {
	totalGPUs := cfg.Replicas * cfg.GPUs
	return fmt.Sprintf(
		"=== Showcase Metrics ===\nReplicas: %d\nCPU per pod: %s\nMemory per pod: %s\nGPU per pod: %d\nTotal GPUs requested: %d\nImage: %s\n",
		cfg.Replicas,
		cfg.CPU,
		cfg.Memory,
		cfg.GPUs,
		totalGPUs,
		cfg.Image,
	)
}
