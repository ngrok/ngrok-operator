package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"k8s.io/utils/ptr"

	ingressv1alpha1 "github.com/ngrok/ngrok-operator/api/ingress/v1alpha1"
)

const (
	// DefaultConfigPath is the default path where the config is mounted
	DefaultConfigPath = "/etc/operator"
	// ConfigMapName is the name of the ConfigMap containing operator configuration
	ConfigMapName = "ngrok-operator-cmd-params-cm"
)

// LoadAndValidateConfig loads configuration from multiple sources and validates it:
// 1. ConfigMap mounted at path (if configPath is provided and exists)
// 2. Environment variables (with NGROK_OPERATOR_ prefix)
// 3. Default values
func LoadAndValidateConfig(configPath string) (*OperatorConfig, error) {
	config := NewDefaultConfig()

	// Initialize viper
	v := viper.New()
	v.SetEnvPrefix("NGROK_OPERATOR")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Load from ConfigMap mounted as volume if specified and exists
	if configPath != "" {
		if err := loadFromConfigMapVolume(v, configPath); err != nil {
			fmt.Printf("Warning: failed to load config from path %s: %v. Using defaults.\n", configPath, err)
		}
	}

	// Unmarshal into config struct
	if err := v.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Set runtime environment values
	var err error
	config.Namespace, err = GetNamespace()
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace: %w", err)
	}

	// Post-process defaults and validate configuration
	if err := processDefaultsAndValidate(config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return config, nil
}

// loadFromConfigMapVolume loads configuration from a ConfigMap mounted as a volume
// Each key in the ConfigMap becomes a file in the mounted directory
func loadFromConfigMapVolume(v *viper.Viper, configPath string) error {
	// Check if the directory exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("config directory %s does not exist", configPath)
	}

	// Read all files in the directory (each file represents a ConfigMap key)
	entries, err := os.ReadDir(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue // Skip subdirectories
		}

		key := entry.Name()
		filePath := filepath.Join(configPath, key)

		// Read the file content
		content, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Printf("Warning: failed to read config file %s: %v\n", filePath, err)
			continue
		}

		// Set the value in viper (trim any trailing newlines from mounted files)
		value := strings.TrimSpace(string(content))
		v.Set(key, value)
	}

	return nil
}

// processDefaultsAndValidate handles computed values and validates configuration
func processDefaultsAndValidate(config *OperatorConfig) error {
	// Handle computed/internal defaults only (not Helm values)
	// Note: We trust Helm values.yaml for user-configurable defaults
	
	// Only set internal defaults that aren't from Helm values
	if config.API.IngressControllerName == "" {
		config.API.IngressControllerName = "k8s.ngrok.com/ingress-controller"
	}
	if config.Bindings.IngressEndpoint == "" {
		config.Bindings.IngressEndpoint = "kubernetes-binding-ingress.ngrok.io:443"
	}

	// Handle special conversions for Helm array values
	if len(config.Bindings.EndpointSelectors) == 0 && config.Bindings.EndpointSelectors != nil {
		// Only default if it was set but empty, not if it was never set
		config.Bindings.EndpointSelectors = []string{"true"}
	}

	// Parse and validate API URL if provided
	if config.APIURLString != "" {
		parsedURL, err := url.Parse(config.APIURLString)
		if err != nil {
			return fmt.Errorf("invalid API URL %s: %w", config.APIURLString, err)
		}
		config.APIURL = parsedURL
	}

	// Validate domain reclaim policy
	if err := validateDomainReclaimPolicy(config.API.DefaultDomainReclaimPolicy); err != nil {
		return fmt.Errorf("invalid domain reclaim policy: %w", err)
	}

	// Validate log configuration
	if config.Log.Level == "" {
		return fmt.Errorf("log.level cannot be empty")
	}
	if config.Log.Format != "json" && config.Log.Format != "console" {
		return fmt.Errorf("log.format must be 'json' or 'console'")
	}

	return nil
}

// validateDomainReclaimPolicy validates the domain reclaim policy value
func validateDomainReclaimPolicy(policy string) error {
	switch policy {
	case string(ingressv1alpha1.DomainReclaimPolicyDelete):
		return nil
	case string(ingressv1alpha1.DomainReclaimPolicyRetain):
		return nil
	default:
		return fmt.Errorf("invalid policy %s. Allowed values: %v",
			policy,
			[]string{
				string(ingressv1alpha1.DomainReclaimPolicyDelete),
				string(ingressv1alpha1.DomainReclaimPolicyRetain),
			},
		)
	}
}

// GetDomainReclaimPolicy returns the parsed domain reclaim policy
func GetDomainReclaimPolicy(policy string) *ingressv1alpha1.DomainReclaimPolicy {
	switch policy {
	case string(ingressv1alpha1.DomainReclaimPolicyRetain):
		return ptr.To(ingressv1alpha1.DomainReclaimPolicyRetain)
	default:
		return ptr.To(ingressv1alpha1.DomainReclaimPolicyDelete)
	}
}

// GetNgrokAPIKey returns the ngrok API key from environment
func GetNgrokAPIKey() (string, error) {
	apiKey, ok := os.LookupEnv("NGROK_API_KEY")
	if !ok {
		return "", fmt.Errorf("NGROK_API_KEY environment variable should be set, but was not")
	}
	return apiKey, nil
}
