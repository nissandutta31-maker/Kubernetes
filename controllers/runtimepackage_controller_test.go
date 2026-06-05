package controllers

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	runtimev1alpha1 "github.com/nissandutta31-maker/kubernetes/api/v1alpha1"
)

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
