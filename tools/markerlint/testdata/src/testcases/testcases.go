package testcases

// Test file for markerlint analyzer

// Good markers - should not trigger any warnings
// +kubebuilder:validation:Required
// +kubebuilder:validation:Enum=a;b;c
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type GoodSpec struct {
	// +kubebuilder:validation:Required
	Field string
}

// Bad markers - should trigger warnings

// +kube:validation:Required // want `did you mean "\+kubebuilder:".*missing "builder"`
type BadPrefix struct {
	// +kube:validation:Optional // want `did you mean "\+kubebuilder:".*missing "builder"`
	Field string
}

// +kuberbuilder:validation:Required // want `did you mean "\+kubebuilder:".*extra 'r'`
type ExtraR struct{}

// +kubebilder:validation:Required // want `did you mean "\+kubebuilder:".*missing 'u'`
type MissingU struct{}

// +kubebuidler:validation:Required // want `did you mean "\+kubebuilder:".*transposed`
type Transposed struct{}

// +kubebuilder:validtion:Required // want `unknown kubebuilder marker.*did you mean`
type ValidationTypo struct{}

// +kubebuilder:validation:Optional2 // want `unknown kubebuilder marker.*did you mean`
type InvalidMarker struct{}
