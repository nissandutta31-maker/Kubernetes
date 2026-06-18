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

	// defaultGPUNodeLabel scopes the installer to GPU nodes when a RuntimePackage
	// omits an explicit nodeSelector. Without this, an empty selector would let the
	// privileged, host-mounting installer DaemonSet schedule on every node.
	defaultGPUNodeLabelKey   = "nvidia.com/gpu.present"
	defaultGPUNodeLabelValue = "true"
)

// effectiveNodeSelector returns the node selector the operator actually targets:
// the user's selector when set, otherwise a safe default restricting installs to
// GPU nodes. The returned map is always a fresh copy.
func effectiveNodeSelector(pkg *runtimev1alpha1.RuntimePackage) map[string]string {
	if len(pkg.Spec.NodeSelector) == 0 {
		return map[string]string{defaultGPUNodeLabelKey: defaultGPUNodeLabelValue}
	}
	out := make(map[string]string, len(pkg.Spec.NodeSelector))
	for k, v := range pkg.Spec.NodeSelector {
		out[k] = v
	}
	return out
}

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

	totalNodes, err := r.countMatchingNodes(ctx, effectiveNodeSelector(pkg))
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
	// With no matching nodes yet, the package is Pending rather than Installing —
	// there is nothing to install until target nodes join.
	if totalNodes == 0 {
		return r.patchStatus(ctx, pkg, runtimev1alpha1.PackagePhasePending, "", 0, 0,
			"waiting for nodes matching the node selector")
	}
	return r.patchStatus(ctx, pkg, runtimev1alpha1.PackagePhaseInstalling, "", 0, totalNodes,
		fmt.Sprintf("installing %s v%s", pkg.Spec.PackageName, pkg.Spec.Version))
}

func (r *RuntimePackageReconciler) syncDaemonSet(ctx context.Context, pkg *runtimev1alpha1.RuntimePackage, ds *appsv1.DaemonSet, totalNodes int32) (ctrl.Result, error) {
	if len(ds.Spec.Template.Spec.Containers) == 0 {
		return ctrl.Result{}, fmt.Errorf("installer DaemonSet %q has no containers", ds.Name)
	}

	desiredDS := buildDaemonSet(pkg)
	desiredC := desiredDS.Spec.Template.Spec.Containers[0]
	currentC := ds.Spec.Template.Spec.Containers[0]

	// The version actually deployed is whatever the running DaemonSet's pods carry,
	// not what the spec currently asks for. Reading it from the pod template (rather
	// than assuming spec.Version) keeps status honest when an upgrade is gated or the
	// installer image is pinned via spec.installerImage.
	deployedVersion := envValue(currentC.Env, "PACKAGE_VERSION")
	versionChanged := deployedVersion != pkg.Spec.Version

	// Detect drift across every field we manage — image, env (carries the version and
	// validation script), and node targeting — not just the image.
	drift := currentC.Image != desiredC.Image ||
		!equalEnv(currentC.Env, desiredC.Env) ||
		!equalStringMap(ds.Spec.Template.Spec.NodeSelector, desiredDS.Spec.Template.Spec.NodeSelector)

	// Roll the DaemonSet only when there is drift AND either it is not a version change
	// or auto-upgrade is enabled. This honors spec.autoUpgrade=false (no surprise rolls).
	if drift && (!versionChanged || pkg.Spec.AutoUpgrade) {
		log.FromContext(ctx).Info("rolling installer DaemonSet", "image", desiredC.Image, "version", pkg.Spec.Version)
		ds.Spec.Template.Spec.Containers[0].Image = desiredC.Image
		ds.Spec.Template.Spec.Containers[0].Env = desiredC.Env
		ds.Spec.Template.Spec.Containers[0].Args = desiredC.Args
		ds.Spec.Template.Spec.NodeSelector = desiredDS.Spec.Template.Spec.NodeSelector
		if err := r.Update(ctx, ds); err != nil {
			return ctrl.Result{}, fmt.Errorf("updating DaemonSet: %w", err)
		}
		// Leave InstalledVersion untouched (empty) until the new pods are confirmed
		// ready on a later reconcile — the old version is still what's running.
		return r.patchStatus(ctx, pkg, runtimev1alpha1.PackagePhaseUpgrading, "", ds.Status.NumberReady, totalNodes,
			fmt.Sprintf("rolling out %s v%s", pkg.Spec.PackageName, pkg.Spec.Version))
	}

	// No roll: report readiness for the version currently deployed.
	readyNodes := ds.Status.NumberReady
	desired := ds.Status.DesiredNumberScheduled

	var phase runtimev1alpha1.PackagePhase
	var message, installedVersion string
	switch {
	case totalNodes == 0:
		phase = runtimev1alpha1.PackagePhasePending
		message = "waiting for nodes matching the node selector"
	case desired == totalNodes && readyNodes == totalNodes:
		// Ready only when every targeted node — not just every *scheduled* one — has
		// the package. desired can be < totalNodes if some targeted nodes can't run
		// the installer (e.g. taints), which must not read as Ready.
		phase = runtimev1alpha1.PackagePhaseReady
		installedVersion = deployedVersion
		message = fmt.Sprintf("%s v%s installed on %d node(s)", pkg.Spec.PackageName, deployedVersion, readyNodes)
	default:
		phase = runtimev1alpha1.PackagePhaseInstalling
		message = fmt.Sprintf("waiting for nodes: %d/%d ready (%d scheduled)", readyNodes, totalNodes, desired)
	}

	// Surface a gated upgrade so it is not silently ignored.
	if versionChanged && !pkg.Spec.AutoUpgrade {
		message += fmt.Sprintf("; upgrade to v%s available (autoUpgrade disabled)", pkg.Spec.Version)
	}

	return r.patchStatus(ctx, pkg, phase, installedVersion, readyNodes, totalNodes, message)
}

// patchStatus writes the package status. installedVersion is only applied when
// non-empty (i.e. when readiness at a concrete version has been confirmed), so a
// rollout in progress does not prematurely advance the reported installed version.
func (r *RuntimePackageReconciler) patchStatus(
	ctx context.Context,
	pkg *runtimev1alpha1.RuntimePackage,
	phase runtimev1alpha1.PackagePhase,
	installedVersion string,
	readyNodes, totalNodes int32,
	message string,
) (ctrl.Result, error) {
	now := metav1.Now()
	pkg.Status.Phase = phase
	pkg.Status.ReadyNodes = readyNodes
	pkg.Status.TotalNodes = totalNodes
	pkg.Status.LastUpdateTime = &now

	if installedVersion != "" {
		pkg.Status.InstalledVersion = installedVersion
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

	// Ready is terminal; every other phase requeues. Pending in particular requeues
	// so the operator notices when matching nodes join later (it watches its own
	// DaemonSets, not Node objects).
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

	nodeSelector := effectiveNodeSelector(pkg)

	gracePeriod := int64(30)
	privileged := true
	hostRootType := corev1.HostPathDirectory
	hostRunType := corev1.HostPathDirectoryOrCreate

	env := []corev1.EnvVar{
		{Name: "PACKAGE_NAME", Value: pkg.Spec.PackageName},
		{Name: "PACKAGE_VERSION", Value: pkg.Spec.Version},
	}
	// The optional post-install validation script (e.g. an nvidia-smi smoke test)
	// is passed in as an env var and executed by the installer command below.
	if pkg.Spec.ValidationScript != "" {
		env = append(env, corev1.EnvVar{Name: "VALIDATION_SCRIPT", Value: pkg.Spec.ValidationScript})
	}

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
							// Run the package's install script if the image provides one,
							// then the optional validation script, then hold the pod open
							// as a per-node readiness sentinel. A failure in either step
							// exits non-zero so the pod is NOT reported Ready (the kubelet
							// restarts it). The script-optional form keeps the DaemonSet
							// functional with stand-in images during local testing.
							Args: []string{
								`set -e; ` +
									`if command -v install.sh >/dev/null 2>&1; then install.sh; fi; ` +
									`if [ -n "$VALIDATION_SCRIPT" ]; then printf '%s\n' "$VALIDATION_SCRIPT" | sh; fi; ` +
									`echo "[$PACKAGE_NAME $PACKAGE_VERSION] runtime package ready on $(hostname)"; ` +
									`exec sleep infinity`,
							},
							Env:             env,
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
								HostPath: &corev1.HostPathVolumeSource{Path: "/", Type: &hostRootType},
							},
						},
						{
							Name: "host-run",
							VolumeSource: corev1.VolumeSource{
								// DirectoryOrCreate: /run/nvidia may not exist on a fresh node.
								HostPath: &corev1.HostPathVolumeSource{Path: "/run/nvidia", Type: &hostRunType},
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
// If the RuntimePackage specifies an explicit InstallerImage (e.g. a private
// mirror, an air-gapped registry, or a stand-in image for local testing) it is
// used verbatim; otherwise the image follows NVIDIA's NGC registry convention.
func PackageImage(pkg *runtimev1alpha1.RuntimePackage) string {
	if pkg.Spec.InstallerImage != "" {
		return pkg.Spec.InstallerImage
	}
	return fmt.Sprintf("nvcr.io/nvidia/k8s/%s-installer:%s", pkg.Spec.PackageName, pkg.Spec.Version)
}

// envValue returns the value of the named environment variable, or "" if absent.
func envValue(env []corev1.EnvVar, name string) string {
	for _, e := range env {
		if e.Name == name {
			return e.Value
		}
	}
	return ""
}

// equalEnv reports whether two env-var slices hold the same name/value pairs,
// independent of ordering.
func equalEnv(a, b []corev1.EnvVar) bool {
	if len(a) != len(b) {
		return false
	}
	am := make(map[string]string, len(a))
	for _, e := range a {
		am[e.Name] = e.Value
	}
	for _, e := range b {
		if v, ok := am[e.Name]; !ok || v != e.Value {
			return false
		}
	}
	return true
}

// equalStringMap reports whether two string maps are equal (nil == empty).
func equalStringMap(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// setCondition upserts a condition into the slice, matching by Type.
// LastTransitionTime is preserved when the Status has not changed, per the
// Kubernetes API conventions: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties
func setCondition(conditions *[]metav1.Condition, cond metav1.Condition) {
	for i, c := range *conditions {
		if c.Type == cond.Type {
			if c.Status == cond.Status {
				cond.LastTransitionTime = c.LastTransitionTime
			}
			(*conditions)[i] = cond
			return
		}
	}
	*conditions = append(*conditions, cond)
}
