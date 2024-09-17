package ngrok

// Note: There is no controller for kind: OperatorConfiguration
// This is where we place kubebuilder annotations for the CRD

// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=operatorconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=operatorconfigurations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ngrok.k8s.ngrok.com,resources=operatorconfigurations/finalizers,verbs=update
