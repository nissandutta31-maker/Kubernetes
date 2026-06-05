package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GPUArchitecture enumerates supported NVIDIA GPU architectures.
type GPUArchitecture string

const (
	ArchA100  GPUArchitecture = "A100"
	ArchH100  GPUArchitecture = "H100"
	ArchGB200 GPUArchitecture = "GB200"
	ArchGB300 GPUArchitecture = "GB300"
)

// PackagePhase represents the lifecycle phase of a RuntimePackage.
type PackagePhase string

const (
	PackagePhasePending    PackagePhase = "Pending"
	PackagePhaseInstalling PackagePhase = "Installing"
	PackagePhaseReady      PackagePhase = "Ready"
	PackagePhaseUpgrading  PackagePhase = "Upgrading"
	PackagePhaseFailed     PackagePhase = "Failed"
)

// RuntimePackageSpec defines the desired state of a RuntimePackage.
type RuntimePackageSpec struct {
	// PackageName is the name of the GPU runtime package (e.g. nvidia-container-toolkit).
	// +kubebuilder:validation:MinLength=1
	PackageName string `json:"packageName"`

	// Version is the desired semver version of the package.
	// +kubebuilder:validation:Pattern=`^\d+\.\d+\.\d+.*$`
	Version string `json:"version"`

	// TargetArchitectures lists the GPU architectures this package supports.
	// +kubebuilder:validation:MinItems=1
	TargetArchitectures []GPUArchitecture `json:"targetArchitectures"`

	// NodeSelector restricts installation to nodes matching all listed labels.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// AutoUpgrade enables in-place upgrades when the Version field changes.
	// +optional
	AutoUpgrade bool `json:"autoUpgrade,omitempty"`

	// ValidationScript is an optional shell script executed post-install to
	// verify the package works correctly (e.g. nvidia-smi checks).
	// +optional
	ValidationScript string `json:"validationScript,omitempty"`
}

// RuntimePackageStatus defines the observed state of a RuntimePackage.
type RuntimePackageStatus struct {
	// Phase is the current lifecycle phase.
	// +optional
	Phase PackagePhase `json:"phase,omitempty"`

	// InstalledVersion tracks the version last successfully installed.
	// +optional
	InstalledVersion string `json:"installedVersion,omitempty"`

	// ReadyNodes is the count of nodes that have the package installed and healthy.
	// +optional
	ReadyNodes int32 `json:"readyNodes,omitempty"`

	// TotalNodes is the total count of nodes targeted by NodeSelector.
	// +optional
	TotalNodes int32 `json:"totalNodes,omitempty"`

	// Conditions represent the latest observations of the package's state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastUpdateTime is the last time the operator wrote to this status.
	// +optional
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=rtpkg,categories=nvidia
// +kubebuilder:printcolumn:name="Package",type=string,JSONPath=`.spec.packageName`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyNodes`
// +kubebuilder:printcolumn:name="Total",type=integer,JSONPath=`.status.totalNodes`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// RuntimePackage manages the lifecycle of a GPU runtime package across Kubernetes nodes.
// The operator ensures that each targeted node runs the desired version of the package
// using an installer DaemonSet, mirroring how NVIDIA's GPU Operator distributes
// the container toolkit, DRA drivers, and other accelerated compute components.
type RuntimePackage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RuntimePackageSpec   `json:"spec,omitempty"`
	Status RuntimePackageStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RuntimePackageList contains a list of RuntimePackage resources.
type RuntimePackageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RuntimePackage `json:"items"`
}
