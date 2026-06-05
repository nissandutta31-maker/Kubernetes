package controllers

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	runtimev1alpha1 "github.com/nissandutta31-maker/kubernetes/api/v1alpha1"
)

const (
	conditionTypeReady = "Ready"
	requeueInterval    = 30 * time.Second
)

// RuntimePackageReconciler reconciles RuntimePackage objects.
//
// It ensures that for every RuntimePackage CR the cluster contains a DaemonSet
// that installs the specified GPU runtime package on every matching node. This
// mirrors how NVIDIA's GPU Operator distributes the container toolkit, DRA
// drivers, and other accelerated compute components across GPU nodes.
type RuntimePackageReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=runtime.nvidia.com,resources=runtimepackages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=runtime.nvidia.com,resources=runtimepackages/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=runtime.nvidia.com,resources=runtimepackages/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

func (r *RuntimePackageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	pkg := &runtimev1alpha1.RuntimePackage{}
	if err := r.Get(ctx, req.NamespacedName, pkg); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("reconciling", "package", pkg.Spec.PackageName, "version", pkg.Spec.Version)

	totalNodes, err := r.countMatchingNodes(ctx, pkg.Spec.NodeSelector)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("counting target nodes: %w", err)
	}

	ds := &appsv1.DaemonSet{}
	dsKey := types.NamespacedName{Name: DaemonSetName(pkg), Namespace: pkg.Namespace}

	if err := r.Get(ctx, dsKey, ds); errors.IsNotFound(err) {
		return r.createDaemonSet(ctx, pkg, totalNodes)
	} else if err != nil {
		return ctrl.Result{}, err
	}

	return r.syncDaemonSet(ctx, pkg, ds, totalNodes)
}

func (r *RuntimePackageReconciler) createDaemonSet(ctx context.Context, pkg *runtimev1alpha1.RuntimePackage, totalNodes int32) (ctrl.Result, error) {
	newDS := buildDaemonSet(pkg)
	if err := ctrl.SetControllerReference(pkg, newDS, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	log.FromContext(ctx).Info("creating installer DaemonSet", "name", newDS.Name)
	if err := r.Create(ctx, newDS); err != nil {
		return ctrl.Result{}, fmt.Errorf("creating DaemonSet: %w", err)
	}
	return r.patchStatus(ctx, pkg, runtimev1alpha1.PackagePhaseInstalling, 0, totalNodes,
		fmt.Sprintf("installing %s v%s", pkg.Spec.PackageName, pkg.Spec.Version))
}

func (r *RuntimePackageReconciler) syncDaemonSet(ctx context.Context, pkg *runtimev1alpha1.RuntimePackage, ds *appsv1.DaemonSet, totalNodes int32) (ctrl.Result, error) {
	wantImage := PackageImage(pkg)
	if len(ds.Spec.Template.Spec.Containers) > 0 && ds.Spec.Template.Spec.Containers[0].Image != wantImage {
		log.FromContext(ctx).Info("upgrading package", "image", wantImage)
		ds.Spec.Template.Spec.Containers[0].Image = wantImage
		if err := r.Update(ctx, ds); err != nil {
			return ctrl.Result{}, fmt.Errorf("updating DaemonSet for upgrade: %w", err)
		}
		return r.patchStatus(ctx, pkg, runtimev1alpha1.PackagePhaseUpgrading, ds.Status.NumberReady, totalNodes,
			fmt.Sprintf("upgrading to %s v%s", pkg.Spec.PackageName, pkg.Spec.Version))
	}

	readyNodes := ds.Status.NumberReady
	desired := ds.Status.DesiredNumberScheduled
	phase := runtimev1alpha1.PackagePhaseInstalling
	message := fmt.Sprintf("waiting for nodes: %d/%d ready", readyNodes, desired)

	if desired > 0 && readyNodes == desired {
		phase = runtimev1alpha1.PackagePhaseReady
		message = fmt.Sprintf("%s v%s installed on %d node(s)", pkg.Spec.PackageName, pkg.Spec.Version, readyNodes)
	}

	return r.patchStatus(ctx, pkg, phase, readyNodes, totalNodes, message)
}

func (r *RuntimePackageReconciler) patchStatus(
	ctx context.Context,
	pkg *runtimev1alpha1.RuntimePackage,
	phase runtimev1alpha1.PackagePhase,
	readyNodes, totalNodes int32,
	message string,
) (ctrl.Result, error) {
	now := metav1.Now()
	pkg.Status.Phase = phase
	pkg.Status.ReadyNodes = readyNodes
	pkg.Status.TotalNodes = totalNodes
	pkg.Status.LastUpdateTime = &now

	if phase == runtimev1alpha1.PackagePhaseReady {
		pkg.Status.InstalledVersion = pkg.Spec.Version
	}

	condStatus := metav1.ConditionFalse
	reason := string(phase)
	if phase == runtimev1alpha1.PackagePhaseReady {
		condStatus = metav1.ConditionTrue
		reason = "PackageInstalled"
	}

	setCondition(&pkg.Status.Conditions, metav1.Condition{
		Type:               conditionTypeReady,
		Status:             condStatus,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: pkg.Generation,
		LastTransitionTime: now,
	})

	if err := r.Status().Update(ctx, pkg); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating status: %w", err)
	}

	if phase == runtimev1alpha1.PackagePhaseReady {
		return ctrl.Result{}, nil
	}
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *RuntimePackageReconciler) countMatchingNodes(ctx context.Context, selector map[string]string) (int32, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList, client.MatchingLabels(selector)); err != nil {
		return 0, err
	}
	return int32(len(nodeList.Items)), nil
}

// SetupWithManager registers the controller and declares that it owns DaemonSets
// so reconciliation is triggered whenever a managed DaemonSet changes.
func (r *RuntimePackageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&runtimev1alpha1.RuntimePackage{}).
		Owns(&appsv1.DaemonSet{}).
		Complete(r)
}

// buildDaemonSet constructs the installer DaemonSet for a RuntimePackage.
// Each node matching the NodeSelector gets a privileged init container that
// installs the runtime package into the host filesystem, mirroring the pattern
// used by nvidia-container-toolkit-daemonset in the GPU Operator.
func buildDaemonSet(pkg *runtimev1alpha1.RuntimePackage) *appsv1.DaemonSet {
	labels := map[string]string{
		"app.kubernetes.io/name":       "runtime-package-installer",
		"app.kubernetes.io/instance":   pkg.Name,
		"app.kubernetes.io/managed-by": "nvidia-runtime-operator",
		"runtime.nvidia.com/package":   pkg.Spec.PackageName,
	}

	nodeSelector := make(map[string]string, len(pkg.Spec.NodeSelector))
	for k, v := range pkg.Spec.NodeSelector {
		nodeSelector[k] = v
	}

	gracePeriod := int64(30)
	privileged := true

	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DaemonSetName(pkg),
			Namespace: pkg.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					NodeSelector:                  nodeSelector,
					TerminationGracePeriodSeconds: &gracePeriod,
					// Run on GPU-tainted nodes without explicitly tolerating each taint.
					Tolerations: []corev1.Toleration{
						{Key: "nvidia.com/gpu", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
					},
					Containers: []corev1.Container{
						{
							Name:            "installer",
							Image:           PackageImage(pkg),
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"/bin/sh", "-c"},
							Args:            []string{"install.sh && sleep infinity"},
							Env: []corev1.EnvVar{
								{Name: "PACKAGE_NAME", Value: pkg.Spec.PackageName},
								{Name: "PACKAGE_VERSION", Value: pkg.Spec.Version},
							},
							SecurityContext: &corev1.SecurityContext{Privileged: &privileged},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("64Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "host-root", MountPath: "/host"},
								{Name: "host-run", MountPath: "/run/nvidia"},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "host-root",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{Path: "/"},
							},
						},
						{
							Name: "host-run",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{Path: "/run/nvidia"},
							},
						},
					},
				},
			},
		},
	}
}

// DaemonSetName returns the deterministic name for the installer DaemonSet
// associated with a RuntimePackage.
func DaemonSetName(pkg *runtimev1alpha1.RuntimePackage) string {
	return fmt.Sprintf("runtime-pkg-%s", pkg.Name)
}

// PackageImage returns the container image used by the installer DaemonSet.
// Images follow NVIDIA's NGC registry convention.
func PackageImage(pkg *runtimev1alpha1.RuntimePackage) string {
	return fmt.Sprintf("nvcr.io/nvidia/k8s/%s-installer:%s", pkg.Spec.PackageName, pkg.Spec.Version)
}

// setCondition upserts a condition into the slice, matching by Type.
func setCondition(conditions *[]metav1.Condition, cond metav1.Condition) {
	for i, c := range *conditions {
		if c.Type == cond.Type {
			(*conditions)[i] = cond
			return
		}
	}
	*conditions = append(*conditions, cond)
}
