package main

import (
	"strings"
	"testing"
)

func TestGenerateManifestIncludesGPUResources(t *testing.T) {
	cfg := workloadConfig{
		Name:      "gpu-demo",
		Namespace: "ai",
		Image:     "nvcr.io/example/image:latest",
		Replicas:  2,
		CPU:       "1000m",
		Memory:    "2Gi",
		GPUs:      2,
		Port:      9090,
	}

	manifest := generateManifest(cfg)

	expectedSnippets := []string{
		"kind: Deployment",
		"name: gpu-demo",
		"namespace: ai",
		"replicas: 2",
		"image: nvcr.io/example/image:latest",
		"nvidia.com/gpu: \"2\"",
		"containerPort: 9090",
		"kind: Service",
		"name: gpu-demo-svc",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(manifest, snippet) {
			t.Fatalf("expected manifest to contain %q", snippet)
		}
	}
}

func TestMeasurableSummaryCalculatesTotalGPU(t *testing.T) {
	cfg := workloadConfig{
		Name:      "gpu-demo",
		Namespace: "default",
		Image:     "nvcr.io/example/image:latest",
		Replicas:  3,
		CPU:       "500m",
		Memory:    "1Gi",
		GPUs:      2,
		Port:      8080,
	}

	summary := measurableSummary(cfg)

	if !strings.Contains(summary, "Total GPUs requested: 6") {
		t.Fatalf("expected total GPU calculation in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "Replicas: 3") {
		t.Fatalf("expected replica value in summary, got: %s", summary)
	}
}

func TestValidateOutputPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "valid relative path", path: "demo.yaml", wantErr: false},
		{name: "valid nested relative path", path: "artifacts/demo.yaml", wantErr: false},
		{name: "reject absolute path", path: "/tmp/demo.yaml", wantErr: true},
		{name: "reject traversal path", path: "../demo.yaml", wantErr: true},
		{name: "reject nested traversal path", path: "subdir/../../../demo.yaml", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateOutputPath(tt.path)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for path %q", tt.path)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("did not expect error for path %q, got: %v", tt.path, err)
			}
		})
	}
}
