package controllers

import (
	"context"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	runtimev1alpha1 "github.com/nissandutta31-maker/kubernetes/api/v1alpha1"
)

// bumpVersion updates spec.version on the named RuntimePackage via the client.
func bumpVersion(t *testing.T, ctx context.Context, c client.Client, key types.NamespacedName, version string) {
	t.Helper()
	cur := &runtimev1alpha1.RuntimePackage{}
	if err := c.Get(ctx, key, cur); err != nil {
		t.Fatalf("get for version bump: %v", err)
	}
	cur.Spec.Version = version
	if err := c.Update(ctx, cur); err != nil {
		t.Fatalf("update version: %v", err)
	}
}

// enableAutoUpgrade flips spec.autoUpgrade to true on the named RuntimePackage.
func enableAutoUpgrade(t *testing.T, ctx context.Context, c client.Client, key types.NamespacedName) {
	t.Helper()
	cur := &runtimev1alpha1.RuntimePackage{}
	if err := c.Get(ctx, key, cur); err != nil {
		t.Fatalf("get for autoUpgrade: %v", err)
	}
	cur.Spec.AutoUpgrade = true
	if err := c.Update(ctx, cur); err != nil {
		t.Fatalf("update autoUpgrade: %v", err)
	}
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("add clientgo scheme: %v", err)
	}
	if err := runtimev1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("add runtimev1alpha1 scheme: %v", err)
	}
	return s
}

func TestReconcile_NotFound(t *testing.T) {
	s := newTestScheme(t)
	r := &RuntimePackageReconciler{
		Client: fake.NewClientBuilder().WithScheme(s).Build(),
		Scheme: s,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "ghost", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Error("expected no requeue for missing resource")
	}
}

func TestReconcile_CreatesInstallerDaemonSet(t *testing.T) {
	s := newTestScheme(t)

	pkg := &runtimev1alpha1.RuntimePackage{
		ObjectMeta: metav1.ObjectMeta{Name: "nct", Namespace: "nvidia-system"},
		Spec: runtimev1alpha1.RuntimePackageSpec{
			PackageName:         "nvidia-container-toolkit",
			Version:             "1.14.6",
			TargetArchitectures: []runtimev1alpha1.GPUArchitecture{runtimev1alpha1.ArchH100},
			NodeSelector:        map[string]string{"nvidia.com/gpu.present": "true"},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(pkg).
		WithStatusSubresource(pkg).
		Build()

	r := &RuntimePackageReconciler{Client: fakeClient, Scheme: s}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "nct", Namespace: "nvidia-system"},
	})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	// Should requeue until all nodes are ready.
	if result.RequeueAfter == 0 {
		t.Error("expected requeue while installing")
	}

	// DaemonSet must exist with the correct image.
	ds := &appsv1.DaemonSet{}
	if err := fakeClient.Get(context.Background(),
		types.NamespacedName{Name: DaemonSetName(pkg), Namespace: "nvidia-system"}, ds); err != nil {
		t.Fatalf("DaemonSet not created: %v", err)
	}
	wantImage := PackageImage(pkg)
	if ds.Spec.Template.Spec.Containers[0].Image != wantImage {
		t.Errorf("image: want %s, got %s", wantImage, ds.Spec.Template.Spec.Containers[0].Image)
	}
}

func TestReconcile_IdempotentOnSecondCall(t *testing.T) {
	s := newTestScheme(t)

	pkg := &runtimev1alpha1.RuntimePackage{
		ObjectMeta: metav1.ObjectMeta{Name: "nct", Namespace: "nvidia-system"},
		Spec: runtimev1alpha1.RuntimePackageSpec{
			PackageName:         "nvidia-container-toolkit",
			Version:             "1.14.6",
			TargetArchitectures: []runtimev1alpha1.GPUArchitecture{runtimev1alpha1.ArchGB200},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(pkg).
		WithStatusSubresource(pkg).
		Build()

	r := &RuntimePackageReconciler{Client: fakeClient, Scheme: s}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "nct", Namespace: "nvidia-system"}}

	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	// Second call must not return an error (DaemonSet already exists).
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
}

func TestDaemonSetName(t *testing.T) {
	pkg := &runtimev1alpha1.RuntimePackage{ObjectMeta: metav1.ObjectMeta{Name: "my-pkg"}}
	if got := DaemonSetName(pkg); got != "runtime-pkg-my-pkg" {
		t.Errorf("got %q", got)
	}
}

func TestPackageImage(t *testing.T) {
	pkg := &runtimev1alpha1.RuntimePackage{
		Spec: runtimev1alpha1.RuntimePackageSpec{
			PackageName: "nvidia-container-toolkit",
			Version:     "1.14.6",
		},
	}
	want := "nvcr.io/nvidia/k8s/nvidia-container-toolkit-installer:1.14.6"
	if got := PackageImage(pkg); got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestPackageImage_Override(t *testing.T) {
	pkg := &runtimev1alpha1.RuntimePackage{
		Spec: runtimev1alpha1.RuntimePackageSpec{
			PackageName:    "nvidia-container-toolkit",
			Version:        "1.14.6",
			InstallerImage: "registry.example.com/mirror/toolkit:1.14.6",
		},
	}
	// An explicit InstallerImage must be used verbatim (mirror / air-gap / testing).
	if got := PackageImage(pkg); got != "registry.example.com/mirror/toolkit:1.14.6" {
		t.Errorf("override not honored: got %q", got)
	}

	// With no override, fall back to the NGC convention.
	pkg.Spec.InstallerImage = ""
	want := "nvcr.io/nvidia/k8s/nvidia-container-toolkit-installer:1.14.6"
	if got := PackageImage(pkg); got != want {
		t.Errorf("fallback image: want %q got %q", want, got)
	}
}

func TestReconcile_AutoUpgradeGating(t *testing.T) {
	s := newTestScheme(t)
	ctx := context.Background()
	pkg := &runtimev1alpha1.RuntimePackage{
		ObjectMeta: metav1.ObjectMeta{Name: "nct", Namespace: "nvidia-system"},
		Spec: runtimev1alpha1.RuntimePackageSpec{
			PackageName:         "nvidia-container-toolkit",
			Version:             "1.14.6",
			TargetArchitectures: []runtimev1alpha1.GPUArchitecture{runtimev1alpha1.ArchH100},
			AutoUpgrade:         false,
		},
	}
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(pkg).WithStatusSubresource(pkg).Build()
	r := &RuntimePackageReconciler{Client: fc, Scheme: s}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "nct", Namespace: "nvidia-system"}}
	dsKey := types.NamespacedName{Name: DaemonSetName(pkg), Namespace: "nvidia-system"}

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("reconcile (create): %v", err)
	}

	// Bump the version while autoUpgrade is false — the DaemonSet must NOT roll.
	bumpVersion(t, ctx, fc, req.NamespacedName, "1.15.0")
	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("reconcile (gated): %v", err)
	}
	ds := &appsv1.DaemonSet{}
	if err := fc.Get(ctx, dsKey, ds); err != nil {
		t.Fatalf("get ds: %v", err)
	}
	if got := envValue(ds.Spec.Template.Spec.Containers[0].Env, "PACKAGE_VERSION"); got != "1.14.6" {
		t.Errorf("autoUpgrade=false must not roll: PACKAGE_VERSION = %q, want 1.14.6", got)
	}

	// Enable autoUpgrade — now the roll should happen.
	enableAutoUpgrade(t, ctx, fc, req.NamespacedName)
	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("reconcile (roll): %v", err)
	}
	if err := fc.Get(ctx, dsKey, ds); err != nil {
		t.Fatalf("get ds: %v", err)
	}
	if got := envValue(ds.Spec.Template.Spec.Containers[0].Env, "PACKAGE_VERSION"); got != "1.15.0" {
		t.Errorf("autoUpgrade=true must roll: PACKAGE_VERSION = %q, want 1.15.0", got)
	}
}

func TestReconcile_PinnedImageVersionBumpStillRolls(t *testing.T) {
	s := newTestScheme(t)
	ctx := context.Background()
	pkg := &runtimev1alpha1.RuntimePackage{
		ObjectMeta: metav1.ObjectMeta{Name: "nct", Namespace: "nvidia-system"},
		Spec: runtimev1alpha1.RuntimePackageSpec{
			PackageName:         "nvidia-container-toolkit",
			Version:             "1.14.6",
			TargetArchitectures: []runtimev1alpha1.GPUArchitecture{runtimev1alpha1.ArchH100},
			InstallerImage:      "busybox:1.36", // pinned: does NOT change with version
			AutoUpgrade:         true,
		},
	}
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(pkg).WithStatusSubresource(pkg).Build()
	r := &RuntimePackageReconciler{Client: fc, Scheme: s}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "nct", Namespace: "nvidia-system"}}
	dsKey := types.NamespacedName{Name: DaemonSetName(pkg), Namespace: "nvidia-system"}

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("reconcile (create): %v", err)
	}
	bumpVersion(t, ctx, fc, req.NamespacedName, "1.15.0")
	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("reconcile (roll): %v", err)
	}
	ds := &appsv1.DaemonSet{}
	if err := fc.Get(ctx, dsKey, ds); err != nil {
		t.Fatalf("get ds: %v", err)
	}
	// Image stays pinned, but the version-bearing env must have rolled — otherwise a
	// version change with a pinned image would be silently ignored.
	if ds.Spec.Template.Spec.Containers[0].Image != "busybox:1.36" {
		t.Errorf("pinned image changed unexpectedly: %q", ds.Spec.Template.Spec.Containers[0].Image)
	}
	if got := envValue(ds.Spec.Template.Spec.Containers[0].Env, "PACKAGE_VERSION"); got != "1.15.0" {
		t.Errorf("version bump with pinned image must still roll env: PACKAGE_VERSION = %q", got)
	}
}

func TestReconcile_NoMatchingNodesPending(t *testing.T) {
	s := newTestScheme(t)
	pkg := &runtimev1alpha1.RuntimePackage{
		ObjectMeta: metav1.ObjectMeta{Name: "nct", Namespace: "nvidia-system"},
		Spec: runtimev1alpha1.RuntimePackageSpec{
			PackageName:         "nvidia-container-toolkit",
			Version:             "1.14.6",
			TargetArchitectures: []runtimev1alpha1.GPUArchitecture{runtimev1alpha1.ArchH100},
			NodeSelector:        map[string]string{"nvidia.com/gpu.present": "true"},
		},
	}
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(pkg).WithStatusSubresource(pkg).Build()
	r := &RuntimePackageReconciler{Client: fc, Scheme: s}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "nct", Namespace: "nvidia-system"}}

	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	got := &runtimev1alpha1.RuntimePackage{}
	if err := fc.Get(context.Background(), req.NamespacedName, got); err != nil {
		t.Fatalf("get pkg: %v", err)
	}
	// No matching nodes → Pending, not stuck in Installing.
	if got.Status.Phase != runtimev1alpha1.PackagePhasePending {
		t.Errorf("phase with zero matching nodes: want Pending got %q", got.Status.Phase)
	}
}

func TestBuildDaemonSet_ValidationScript(t *testing.T) {
	pkg := &runtimev1alpha1.RuntimePackage{
		ObjectMeta: metav1.ObjectMeta{Name: "nct", Namespace: "nvidia-system"},
		Spec: runtimev1alpha1.RuntimePackageSpec{
			PackageName:         "nvidia-container-toolkit",
			Version:             "1.14.6",
			TargetArchitectures: []runtimev1alpha1.GPUArchitecture{runtimev1alpha1.ArchH100},
			ValidationScript:    "nvidia-smi -L",
		},
	}
	ds := buildDaemonSet(pkg)
	c := ds.Spec.Template.Spec.Containers[0]
	if got := envValue(c.Env, "VALIDATION_SCRIPT"); got != "nvidia-smi -L" {
		t.Errorf("VALIDATION_SCRIPT env: want %q got %q", "nvidia-smi -L", got)
	}
	// The installer command must actually execute the validation script.
	if len(c.Args) == 0 || !strings.Contains(c.Args[0], "VALIDATION_SCRIPT") {
		t.Errorf("installer command does not reference VALIDATION_SCRIPT: %v", c.Args)
	}
}

func TestSetCondition_Upsert(t *testing.T) {
	conditions := []metav1.Condition{}
	cond := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "Installing",
		Message:            "in progress",
		LastTransitionTime: metav1.Now(),
	}
	setCondition(&conditions, cond)
	if len(conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(conditions))
	}

	// Upsert same type — must replace, not append.
	cond.Status = metav1.ConditionTrue
	cond.Reason = "PackageInstalled"
	setCondition(&conditions, cond)
	if len(conditions) != 1 {
		t.Fatalf("expected 1 condition after upsert, got %d", len(conditions))
	}
	if conditions[0].Status != metav1.ConditionTrue {
		t.Error("condition was not updated")
	}
}

func TestSetCondition_PreservesTransitionTime(t *testing.T) {
	original := metav1.Time{Time: time.Now().Add(-time.Hour)}
	conditions := []metav1.Condition{{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "Installing",
		LastTransitionTime: original,
	}}

	// Same Status — LastTransitionTime must be preserved.
	setCondition(&conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "StillInstalling",
		LastTransitionTime: metav1.Now(),
	})
	if !conditions[0].LastTransitionTime.Equal(&original) {
		t.Error("LastTransitionTime should not change when Status is unchanged")
	}

	// Changed Status — LastTransitionTime must update.
	newTime := metav1.Now()
	setCondition(&conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "PackageInstalled",
		LastTransitionTime: newTime,
	})
	if conditions[0].LastTransitionTime.Equal(&original) {
		t.Error("LastTransitionTime should update when Status changes")
	}
}

func TestBuildDaemonSet_GPUToleration(t *testing.T) {
	pkg := &runtimev1alpha1.RuntimePackage{
		ObjectMeta: metav1.ObjectMeta{Name: "nct", Namespace: "nvidia-system"},
		Spec: runtimev1alpha1.RuntimePackageSpec{
			PackageName: "nvidia-container-toolkit",
			Version:     "1.14.6",
			TargetArchitectures: []runtimev1alpha1.GPUArchitecture{
				runtimev1alpha1.ArchGB300,
			},
		},
	}
	ds := buildDaemonSet(pkg)
	for _, tol := range ds.Spec.Template.Spec.Tolerations {
		if tol.Key == "nvidia.com/gpu" {
			return // toleration found
		}
	}
	t.Error("DaemonSet missing nvidia.com/gpu toleration")
}
