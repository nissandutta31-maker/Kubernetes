//go:build integration

// Package controllers integration test.
//
// Unlike the unit tests (which use a fake in-memory client), this suite runs
// against a REAL Kubernetes API server started by envtest — the same
// kube-apiserver + etcd binaries that power a production control plane. It
// installs the real CRD, so the kubebuilder validation markers are actually
// enforced, the /status subresource behaves like production, and the
// RuntimePackage objects go through genuine admission and storage.
//
// There is no kubelet or DaemonSet controller in envtest, so DaemonSet pods
// never actually schedule. We therefore simulate the DaemonSet becoming ready
// (by patching its status) to drive the RuntimePackage to its Ready phase —
// this is the standard envtest technique for testing a controller's reaction
// to state it does not itself own.
//
// Run with:  make test-integration
package controllers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	runtimev1alpha1 "github.com/nissandutta31-maker/kubernetes/api/v1alpha1"
)

var (
	testEnv    *envtest.Environment
	restCfg    *rest.Config
	k8sClient  client.Client
	testScheme *runtime.Scheme
)

func TestMain(m *testing.M) {
	testScheme = runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(testScheme); err != nil {
		panic(err)
	}
	if err := runtimev1alpha1.AddToScheme(testScheme); err != nil {
		panic(err)
	}

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd")},
		ErrorIfCRDPathMissing: true,
		Scheme:                testScheme,
	}

	var err error
	restCfg, err = testEnv.Start()
	if err != nil {
		panic("failed to start envtest control plane: " + err.Error())
	}

	k8sClient, err = client.New(restCfg, client.Options{Scheme: testScheme})
	if err != nil {
		panic("failed to build client: " + err.Error())
	}

	code := m.Run()

	// Stop the control plane before exiting (cannot defer before os.Exit).
	_ = testEnv.Stop()
	os.Exit(code)
}

// TestIntegration_FullLifecycle drives a RuntimePackage from creation through
// to the Ready phase against a real API server.
func TestIntegration_FullLifecycle(t *testing.T) {
	ctx := context.Background()
	ns := "nvidia-system"

	mustCreateNamespace(t, ctx, ns)

	// Two GPU nodes that the package should target.
	for _, name := range []string{"gpu-node-a", "gpu-node-b"} {
		mustCreateGPUNode(t, ctx, name)
	}

	pkg := &runtimev1alpha1.RuntimePackage{
		ObjectMeta: metav1.ObjectMeta{Name: "container-toolkit", Namespace: ns},
		Spec: runtimev1alpha1.RuntimePackageSpec{
			PackageName:         "nvidia-container-toolkit",
			Version:             "1.14.6",
			TargetArchitectures: []runtimev1alpha1.GPUArchitecture{runtimev1alpha1.ArchGB200},
			NodeSelector:        map[string]string{"nvidia.com/gpu.present": "true"},
			AutoUpgrade:         true,
		},
	}
	if err := k8sClient.Create(ctx, pkg); err != nil {
		t.Fatalf("create RuntimePackage: %v", err)
	}
	t.Logf("created RuntimePackage %s/%s (version %s)", ns, pkg.Name, pkg.Spec.Version)

	r := &RuntimePackageReconciler{Client: k8sClient, Scheme: testScheme}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: pkg.Name, Namespace: ns}}

	// --- Reconcile #1: should create the installer DaemonSet ---
	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("reconcile #1: %v", err)
	}

	ds := &appsv1.DaemonSet{}
	dsKey := types.NamespacedName{Name: DaemonSetName(pkg), Namespace: ns}
	if err := k8sClient.Get(ctx, dsKey, ds); err != nil {
		t.Fatalf("installer DaemonSet not created: %v", err)
	}
	wantImage := PackageImage(pkg)
	if got := ds.Spec.Template.Spec.Containers[0].Image; got != wantImage {
		t.Fatalf("DaemonSet image: want %q got %q", wantImage, got)
	}
	t.Logf("✓ reconcile #1 created DaemonSet %q with image %q", ds.Name, wantImage)

	// Owner reference must point back at the RuntimePackage (for GC).
	if len(ds.OwnerReferences) == 0 || ds.OwnerReferences[0].Name != pkg.Name {
		t.Fatalf("DaemonSet missing owner reference to RuntimePackage")
	}
	t.Logf("✓ DaemonSet is owned by the RuntimePackage (cascade delete wired)")

	// Status should report Installing and have counted both GPU nodes.
	got := mustGet(t, ctx, pkg.Name, ns)
	if got.Status.Phase != runtimev1alpha1.PackagePhaseInstalling {
		t.Fatalf("phase: want Installing got %q", got.Status.Phase)
	}
	if got.Status.TotalNodes != 2 {
		t.Fatalf("TotalNodes: want 2 got %d", got.Status.TotalNodes)
	}
	t.Logf("✓ status: phase=%s totalNodes=%d (real /status subresource)", got.Status.Phase, got.Status.TotalNodes)

	// --- Simulate the DaemonSet's pods becoming Ready on both nodes ---
	ds.Status.DesiredNumberScheduled = 2
	ds.Status.NumberReady = 2
	ds.Status.NumberAvailable = 2
	if err := k8sClient.Status().Update(ctx, ds); err != nil {
		t.Fatalf("simulate DaemonSet readiness: %v", err)
	}
	t.Logf("(simulated kubelet: DaemonSet now 2/2 ready)")

	// --- Reconcile #2: should flip the package to Ready ---
	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("reconcile #2: %v", err)
	}
	got = mustGet(t, ctx, pkg.Name, ns)
	if got.Status.Phase != runtimev1alpha1.PackagePhaseReady {
		t.Fatalf("phase after readiness: want Ready got %q", got.Status.Phase)
	}
	if got.Status.InstalledVersion != "1.14.6" {
		t.Fatalf("InstalledVersion: want 1.14.6 got %q", got.Status.InstalledVersion)
	}
	readyCond := findCondition(got.Status.Conditions, conditionTypeReady)
	if readyCond == nil || readyCond.Status != metav1.ConditionTrue {
		t.Fatalf("Ready condition not True: %+v", readyCond)
	}
	t.Logf("✓ reconcile #2: phase=%s installedVersion=%s readyNodes=%d/%d",
		got.Status.Phase, got.Status.InstalledVersion, got.Status.ReadyNodes, got.Status.TotalNodes)

	// --- Upgrade: bump the version, expect the DaemonSet image to roll ---
	got.Spec.Version = "1.15.0"
	if err := k8sClient.Update(ctx, got); err != nil {
		t.Fatalf("bump version: %v", err)
	}
	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("reconcile #3 (upgrade): %v", err)
	}
	if err := k8sClient.Get(ctx, dsKey, ds); err != nil {
		t.Fatalf("get DaemonSet after upgrade: %v", err)
	}
	wantUpgraded := "nvcr.io/nvidia/k8s/nvidia-container-toolkit-installer:1.15.0"
	if got := ds.Spec.Template.Spec.Containers[0].Image; got != wantUpgraded {
		t.Fatalf("upgraded image: want %q got %q", wantUpgraded, got)
	}
	pkgAfter := mustGet(t, ctx, pkg.Name, ns)
	if pkgAfter.Status.Phase != runtimev1alpha1.PackagePhaseUpgrading {
		t.Fatalf("phase after upgrade: want Upgrading got %q", pkgAfter.Status.Phase)
	}
	// InstalledVersion must NOT jump to 1.15.0 until the new pods are confirmed ready.
	if pkgAfter.Status.InstalledVersion != "1.14.6" {
		t.Fatalf("InstalledVersion advanced prematurely during rollout: got %q want 1.14.6", pkgAfter.Status.InstalledVersion)
	}
	t.Logf("✓ reconcile #3: rolled DaemonSet to %q, phase=Upgrading, installedVersion still 1.14.6", wantUpgraded)

	// --- Simulate the upgraded pods becoming ready, then reconcile to completion ---
	if err := k8sClient.Get(ctx, dsKey, ds); err != nil {
		t.Fatalf("get DaemonSet: %v", err)
	}
	ds.Status.DesiredNumberScheduled = 2
	ds.Status.NumberReady = 2
	ds.Status.NumberAvailable = 2
	if err := k8sClient.Status().Update(ctx, ds); err != nil {
		t.Fatalf("simulate upgraded readiness: %v", err)
	}
	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("reconcile #4: %v", err)
	}
	final := mustGet(t, ctx, pkg.Name, ns)
	if final.Status.Phase != runtimev1alpha1.PackagePhaseReady || final.Status.InstalledVersion != "1.15.0" {
		t.Fatalf("after upgrade completion: want Ready/1.15.0 got %s/%s", final.Status.Phase, final.Status.InstalledVersion)
	}
	t.Logf("✓ reconcile #4: upgrade complete, phase=Ready installedVersion=1.15.0")

	t.Log("PASS — full install → ready → gated upgrade → ready lifecycle verified against a real kube-apiserver")
}

// TestIntegration_CRDValidation proves the kubebuilder validation markers in the
// CRD are enforced by the real API server (a fake client would NOT catch this).
func TestIntegration_CRDValidation(t *testing.T) {
	ctx := context.Background()
	ns := "validation-ns"
	mustCreateNamespace(t, ctx, ns)

	// Version "not-a-semver" violates the +kubebuilder:validation:Pattern marker.
	bad := &runtimev1alpha1.RuntimePackage{
		ObjectMeta: metav1.ObjectMeta{Name: "bad-version", Namespace: ns},
		Spec: runtimev1alpha1.RuntimePackageSpec{
			PackageName:         "nvidia-container-toolkit",
			Version:             "not-a-semver",
			TargetArchitectures: []runtimev1alpha1.GPUArchitecture{runtimev1alpha1.ArchH100},
		},
	}
	err := k8sClient.Create(ctx, bad)
	if err == nil {
		t.Fatal("expected API server to REJECT invalid version, but it was accepted")
	}
	t.Logf("✓ API server rejected invalid version as expected: %v", err)

	// Empty targetArchitectures violates +kubebuilder:validation:MinItems=1.
	bad2 := &runtimev1alpha1.RuntimePackage{
		ObjectMeta: metav1.ObjectMeta{Name: "no-arch", Namespace: ns},
		Spec: runtimev1alpha1.RuntimePackageSpec{
			PackageName:         "nvidia-container-toolkit",
			Version:             "1.14.6",
			TargetArchitectures: []runtimev1alpha1.GPUArchitecture{},
		},
	}
	if err := k8sClient.Create(ctx, bad2); err == nil {
		t.Fatal("expected API server to REJECT empty targetArchitectures, but it was accepted")
	}
	t.Logf("✓ API server rejected empty targetArchitectures as expected")
}

// TestIntegration_InstallerImagePreserved proves the installerImage field is
// part of the CRD schema and is NOT pruned by the API server, and that the
// controller threads it through to the DaemonSet. A schema typo would cause the
// field to silently vanish on write — this catches that.
func TestIntegration_InstallerImagePreserved(t *testing.T) {
	ctx := context.Background()
	ns := "override-ns"
	mustCreateNamespace(t, ctx, ns)

	pkg := &runtimev1alpha1.RuntimePackage{
		ObjectMeta: metav1.ObjectMeta{Name: "mirror-pkg", Namespace: ns},
		Spec: runtimev1alpha1.RuntimePackageSpec{
			PackageName:         "nvidia-container-toolkit",
			Version:             "1.14.6",
			TargetArchitectures: []runtimev1alpha1.GPUArchitecture{runtimev1alpha1.ArchH100},
			InstallerImage:      "registry.internal.example/mirror/toolkit:1.14.6",
		},
	}
	if err := k8sClient.Create(ctx, pkg); err != nil {
		t.Fatalf("create: %v", err)
	}

	got := mustGet(t, ctx, pkg.Name, ns)
	if got.Spec.InstallerImage != "registry.internal.example/mirror/toolkit:1.14.6" {
		t.Fatalf("installerImage was pruned/lost by the API server: got %q", got.Spec.InstallerImage)
	}
	t.Logf("✓ installerImage survived the API-server round-trip (schema is correct)")

	r := &RuntimePackageReconciler{Client: k8sClient, Scheme: testScheme}
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: pkg.Name, Namespace: ns}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	ds := &appsv1.DaemonSet{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: DaemonSetName(pkg), Namespace: ns}, ds); err != nil {
		t.Fatalf("get DaemonSet: %v", err)
	}
	if got := ds.Spec.Template.Spec.Containers[0].Image; got != "registry.internal.example/mirror/toolkit:1.14.6" {
		t.Fatalf("DaemonSet did not use the override image: got %q", got)
	}
	t.Logf("✓ controller used the override image for the installer DaemonSet")
}

// --- helpers ---

func mustCreateNamespace(t *testing.T, ctx context.Context, name string) {
	t.Helper()
	nsObj := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if err := k8sClient.Create(ctx, nsObj); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create namespace %s: %v", name, err)
	}
}

func mustCreateGPUNode(t *testing.T, ctx context.Context, name string) {
	t.Helper()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"nvidia.com/gpu.present": "true"},
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				"nvidia.com/gpu": resource.MustParse("8"),
			},
		},
	}
	if err := k8sClient.Create(ctx, node); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create node %s: %v", name, err)
	}
}

func mustGet(t *testing.T, ctx context.Context, name, ns string) *runtimev1alpha1.RuntimePackage {
	t.Helper()
	pkg := &runtimev1alpha1.RuntimePackage{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, pkg); err != nil {
		t.Fatalf("get RuntimePackage %s/%s: %v", ns, name, err)
	}
	return pkg
}

func findCondition(conds []metav1.Condition, t string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == t {
			return &conds[i]
		}
	}
	return nil
}
