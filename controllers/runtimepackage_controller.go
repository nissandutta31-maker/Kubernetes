package controllers

import (
	"context"
	"fmt"
	"strings"
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
	conditionTypeReady        = "Ready"
	conditionTypeUnavailable  = "AllPodsUnavailable"
	conditionTypeRolloutStart = "RolloutInProgress"
	requeueInterval          = 30 * time.Second

	// failureDetectionWindow is how long all installer pods must be continuously
	// unavailable before the controller transitions to Failed. Measured from the
	// LastTransitionTime of the AllPodsUnavailable condition, not DaemonSet age,
	// so a long-lived DaemonSet that experiences a short outage does not
	// immediately flip to Failed.
	failureDetectionWindow = 5 * time.Minute

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
		if !errors.IsAlreadyExists(err) {
			return ctrl.Result{}, fmt.Errorf("creating DaemonSet: %w", err)
		}
		// A concurrent reconcile already created it — fetch and sync normally.
		if fetchErr := r.Get(ctx, types.NamespacedName{Name: newDS.Name, Namespace: newDS.Namespace}, newDS); fetchErr != nil {
			return ctrl.Result{}, fetchErr
		}
		return r.syncDaemonSet(ctx, pkg, newDS, totalNodes)
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

	// Selector is immutable in Kubernetes. If spec.packageName changed, the desired
	// selector (which includes runtime.nvidia.com/package) no longer matches.
	// Delete the stale DaemonSet and requeue; createDaemonSet runs on the next
	// reconcile once the old one is fully removed.
	if !equalStringMap(ds.Spec.Selector.MatchLabels, desiredDS.Spec.Selector.MatchLabels) {
		if ds.DeletionTimestamp.IsZero() {
			log.FromContext(ctx).Info("DaemonSet selector mismatch; deleting stale DaemonSet", "name", ds.Name)
			if err := r.Delete(ctx, ds); err != nil && !errors.IsNotFound(err) {
				return ctrl.Result{}, fmt.Errorf("deleting stale DaemonSet: %w", err)
			}
		}
		return r.patchStatus(ctx, pkg, runtimev1alpha1.PackagePhaseInstalling, "", 0, totalNodes,
			fmt.Sprintf("recreating installer DaemonSet for %s", pkg.Spec.PackageName))
	}

	// The version actually deployed is whatever the running DaemonSet's pods carry,
	// not what the spec currently asks for. Reading it from the pod template keeps
	// status honest when an upgrade is gated or the installer image is pinned.
	deployedVersion := envValue(currentC.Env, "PACKAGE_VERSION")
	versionChanged := deployedVersion != pkg.Spec.Version

	// imageDrift is the observed difference between running and desired container images.
	// It is "version-driven" only when using the default NGC image (no InstallerImage
	// override) and spec.version changed — in that case autoUpgrade gates the roll.
	// All other image changes (override set/changed/cleared, package name changed) are
	// config-driven and always apply, with full env so image and PACKAGE_VERSION stay
	// in sync.
	imageDrift := currentC.Image != desiredC.Image
	// An image change is version-driven only when the currently-running image already
	// came from the NGC convention (InstallerImage was not set when the DaemonSet was
	// last rolled) AND InstallerImage is still empty. Clearing an override (custom→NGC)
	// is a config change that must always roll regardless of autoUpgrade.
	currentIsNGC := currentC.Image == fmt.Sprintf("nvcr.io/nvidia/k8s/%s-installer:%s", pkg.Spec.PackageName, deployedVersion)
	imageIsVersionDriven := versionChanged && pkg.Spec.InstallerImage == "" && currentIsNGC
	imageIsConfigDriven := imageDrift && !imageIsVersionDriven

	// configDrift: non-version fields that always roll, independent of autoUpgrade.
	// Package name, validationScript, architecture metadata, nodeSelector, and
	// config-driven image changes take effect immediately.
	configDrift := imageIsConfigDriven ||
		envValue(currentC.Env, "PACKAGE_NAME") != envValue(desiredC.Env, "PACKAGE_NAME") ||
		envValue(currentC.Env, "VALIDATION_SCRIPT") != envValue(desiredC.Env, "VALIDATION_SCRIPT") ||
		envValue(currentC.Env, "PACKAGE_ARCHITECTURES") != envValue(desiredC.Env, "PACKAGE_ARCHITECTURES") ||
		!equalStringMap(ds.Spec.Template.Spec.NodeSelector, desiredDS.Spec.Template.Spec.NodeSelector)

	shouldRollVersion := versionChanged && pkg.Spec.AutoUpgrade
	shouldRoll := configDrift || shouldRollVersion

	if shouldRoll {
		log.FromContext(ctx).Info("rolling installer DaemonSet", "image", desiredC.Image, "version", pkg.Spec.Version)

		// Record rollout-start time on a dedicated condition that the no-roll path
		// never touches. forceCondition always refreshes LastTransitionTime so
		// repeated rolls each get a fresh start timestamp.
		forceCondition(&pkg.Status.Conditions, metav1.Condition{
			Type:               conditionTypeRolloutStart,
			Status:             metav1.ConditionTrue,
			Reason:             "RollingUpdate",
			ObservedGeneration: pkg.Generation,
			LastTransitionTime: metav1.Now(),
		})

		// Config fields always apply immediately.
		ds.Spec.Template.ObjectMeta.Labels = desiredDS.Spec.Template.ObjectMeta.Labels
		ds.Spec.Template.Spec.NodeSelector = desiredDS.Spec.Template.Spec.NodeSelector
		ds.Spec.Template.Spec.Containers[0].Args = desiredC.Args

		if shouldRollVersion || !versionChanged {
			// Full update: version roll is permitted, or no version change (config drift only).
			// Apply full env so image and PACKAGE_VERSION stay consistent.
			ds.Spec.Template.Spec.Containers[0].Image = desiredC.Image
			ds.Spec.Template.Spec.Containers[0].Env = desiredC.Env
		} else if imageIsConfigDriven {
			// Config-driven image change (e.g. InstallerImage override changed or cleared)
			// with a pending version bump gated by autoUpgrade=false. Apply the new image so
			// the config change takes effect, but preserve PACKAGE_VERSION so the version
			// gate is not circumvented.
			ds.Spec.Template.Spec.Containers[0].Image = desiredC.Image
			ds.Spec.Template.Spec.Containers[0].Env = applyConfigEnv(currentC.Env, desiredC.Env)
		} else {
			// Config-only roll: non-image config changed (e.g. validationScript) while a
			// version bump is pending and autoUpgrade=false. Preserve current image and
			// PACKAGE_VERSION.
			ds.Spec.Template.Spec.Containers[0].Env = applyConfigEnv(currentC.Env, desiredC.Env)
		}

		if err := r.Update(ctx, ds); err != nil {
			return ctrl.Result{}, fmt.Errorf("updating DaemonSet: %w", err)
		}
		// Leave InstalledVersion untouched until the new pods are confirmed ready.
		// Report Upgrading only for an actual version roll; config-only rolls report Installing.
		if shouldRollVersion {
			return r.patchStatus(ctx, pkg, runtimev1alpha1.PackagePhaseUpgrading, "", ds.Status.NumberReady, totalNodes,
				fmt.Sprintf("upgrading %s to v%s", pkg.Spec.PackageName, pkg.Spec.Version))
		}
		return r.patchStatus(ctx, pkg, runtimev1alpha1.PackagePhaseInstalling, "", ds.Status.NumberReady, totalNodes,
			fmt.Sprintf("applying config update to %s", pkg.Spec.PackageName))
	}

	// No roll: track all-pods-unavailable onset for failure detection.
	// Using the AllPodsUnavailable condition's LastTransitionTime rather than
	// ds.CreationTimestamp measures how long the CURRENT unavailability run has
	// lasted, not how old the DaemonSet is.
	allUnavailable := ds.Status.DesiredNumberScheduled > 0 &&
		ds.Status.NumberUnavailable == ds.Status.DesiredNumberScheduled
	unavailCondStatus := metav1.ConditionFalse
	unavailReason := "PodsAvailable"
	if allUnavailable {
		unavailCondStatus = metav1.ConditionTrue
		unavailReason = "AllPodsUnavailable"
	}
	setCondition(&pkg.Status.Conditions, metav1.Condition{
		Type:               conditionTypeUnavailable,
		Status:             unavailCondStatus,
		Reason:             unavailReason,
		ObservedGeneration: pkg.Generation,
		LastTransitionTime: metav1.Now(),
	})

	// No roll: report readiness for the version currently deployed.
	readyNodes := ds.Status.NumberReady
	desired := ds.Status.DesiredNumberScheduled

	var phase runtimev1alpha1.PackagePhase
	var message, installedVersion string
	switch {
	case totalNodes == 0:
		phase = runtimev1alpha1.PackagePhasePending
		message = "waiting for nodes matching the node selector"

	case deployedVersion != "" &&
		desired > 0 &&
		readyNodes == desired &&
		ds.Status.UpdatedNumberScheduled == ds.Status.DesiredNumberScheduled:
		// Ready: every schedulable node has the package on the current DaemonSet revision.
		// Evaluated before the Upgrading case so a rollout that just completed and left all
		// pods ready is immediately promoted to Ready without waiting one more cycle.
		phase = runtimev1alpha1.PackagePhaseReady
		installedVersion = deployedVersion
		message = fmt.Sprintf("%s v%s installed on %d/%d node(s)", pkg.Spec.PackageName, deployedVersion, readyNodes, totalNodes)

	// Active version rollout: the DaemonSet template was advanced to a new version
	// (deployedVersion) but InstalledVersion has not been confirmed yet. Use
	// InstalledVersion != deployedVersion rather than UpdatedNumberScheduled < Desired
	// so that node scale-out (new nodes getting the SAME version) is not misreported
	// as an upgrade. Failure detection runs inside this case so a broken version
	// rollout can still reach Failed after the window expires.
	case pkg.Status.InstalledVersion != "" && pkg.Status.InstalledVersion != deployedVersion:
		since := unavailableSince(pkg.Status.Conditions)
		rollStart := rolloutStartedAt(pkg.Status.Conditions)
		// rollStalled: the roll block recorded a start time but the rollout has not
		// completed within the detection window. This catches the common rolling-update
		// failure mode where old pods stay Ready while new-revision pods crash, so
		// allUnavailable never fires yet the upgrade makes no progress.
		rollStalled := rollStart != nil && time.Since(rollStart.Time) > failureDetectionWindow
		if (allUnavailable && since != nil && time.Since(since.Time) > failureDetectionWindow) || rollStalled {
			phase = runtimev1alpha1.PackagePhaseFailed
			if allUnavailable {
				message = fmt.Sprintf("upgrade failed: installer pods unavailable on all %d scheduled node(s); check pod logs",
					ds.Status.DesiredNumberScheduled)
			} else {
				message = fmt.Sprintf("upgrade stalled: rollout to v%s did not complete within the detection window; check pod logs for updated pods",
					deployedVersion)
			}
		} else {
			phase = runtimev1alpha1.PackagePhaseUpgrading
			message = fmt.Sprintf("rolling out: %d/%d pods updated to v%s",
				ds.Status.UpdatedNumberScheduled, ds.Status.DesiredNumberScheduled, deployedVersion)
		}

	case allUnavailable:
		// All pods unavailable with no version rollout in progress: flip to Failed
		// once the detection window expires.
		since := unavailableSince(pkg.Status.Conditions)
		if since != nil && time.Since(since.Time) > failureDetectionWindow {
			phase = runtimev1alpha1.PackagePhaseFailed
			message = fmt.Sprintf("installer pods unavailable on all %d scheduled node(s); check pod logs",
				ds.Status.DesiredNumberScheduled)
		} else {
			phase = runtimev1alpha1.PackagePhaseInstalling
			message = fmt.Sprintf("waiting for pods: all %d scheduled pod(s) currently unavailable",
				ds.Status.DesiredNumberScheduled)
		}

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
	} else if phase == runtimev1alpha1.PackagePhaseFailed ||
		phase == runtimev1alpha1.PackagePhasePending ||
		phase == runtimev1alpha1.PackagePhaseInstalling {
		// Clear InstalledVersion when the package is not confirmed running:
		// Failed (broken), Pending (no matching nodes), or Installing (DaemonSet
		// being created/recreated, pods not yet ready). Upgrading is exempt because
		// it needs the previous InstalledVersion to detect upgrade progress.
		pkg.Status.InstalledVersion = ""
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

	// Clear the rollout-start marker once the rollout resolves (Ready or Failed)
	// so a future rollout starts with a fresh timer.
	if phase == runtimev1alpha1.PackagePhaseReady || phase == runtimev1alpha1.PackagePhaseFailed {
		setCondition(&pkg.Status.Conditions, metav1.Condition{
			Type:               conditionTypeRolloutStart,
			Status:             metav1.ConditionFalse,
			Reason:             string(phase),
			ObservedGeneration: pkg.Generation,
			LastTransitionTime: now,
		})
	}

	if err := r.Status().Update(ctx, pkg); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating status: %w", err)
	}

	// Always requeue so the operator detects node additions/removals and DaemonSet
	// scheduling changes even when the package is Ready — the owned-DaemonSet watch
	// covers most events but not every node-label change that affects totalNodes.
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
	// Pass target architectures as metadata so the installer script can select the
	// correct package variant (e.g. CUDA compute capability). nodeSelector controls
	// WHERE the installer runs; PACKAGE_ARCHITECTURES describes WHAT to install.
	if len(pkg.Spec.TargetArchitectures) > 0 {
		archStrs := make([]string, len(pkg.Spec.TargetArchitectures))
		for i, a := range pkg.Spec.TargetArchitectures {
			archStrs[i] = string(a)
		}
		env = append(env, corev1.EnvVar{Name: "PACKAGE_ARCHITECTURES", Value: strings.Join(archStrs, ",")})
	}

	ds := &appsv1.DaemonSet{
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

	return ds
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

// applyConfigEnv returns desired env vars with PACKAGE_VERSION copied from current.
// Used for config-only rolls (autoUpgrade=false with a pending version bump) to
// apply changes like VALIDATION_SCRIPT without advancing the deployed package version.
func applyConfigEnv(current, desired []corev1.EnvVar) []corev1.EnvVar {
	out := make([]corev1.EnvVar, len(desired))
	copy(out, desired)
	currentVersion := envValue(current, "PACKAGE_VERSION")
	for i, e := range out {
		if e.Name == "PACKAGE_VERSION" {
			out[i].Value = currentVersion
		}
	}
	return out
}

// unavailableSince returns the LastTransitionTime of the AllPodsUnavailable condition
// when it is True, or nil if it is absent or False (pods are available or recovering).
func unavailableSince(conditions []metav1.Condition) *metav1.Time {
	for _, c := range conditions {
		if c.Type == conditionTypeUnavailable && c.Status == metav1.ConditionTrue {
			return &c.LastTransitionTime
		}
	}
	return nil
}

// rolloutStartedAt returns the timestamp when the current rollout began.
// The roll block writes RolloutInProgress=True via forceCondition before
// advancing the DaemonSet template; this condition is only touched by the
// roll block and patchStatus, never by the no-roll path, so it reliably
// marks when THIS rollout started.
func rolloutStartedAt(conditions []metav1.Condition) *metav1.Time {
	for _, c := range conditions {
		if c.Type == conditionTypeRolloutStart && c.Status == metav1.ConditionTrue {
			return &c.LastTransitionTime
		}
	}
	return nil
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

// forceCondition upserts a condition into the slice always using the provided
// LastTransitionTime, even when the Status has not changed. Use this when the
// caller needs to record an event time (e.g. rollout start) rather than a
// status-change time.
func forceCondition(conditions *[]metav1.Condition, cond metav1.Condition) {
	for i, c := range *conditions {
		if c.Type == cond.Type {
			(*conditions)[i] = cond
			return
		}
	}
	*conditions = append(*conditions, cond)
}
