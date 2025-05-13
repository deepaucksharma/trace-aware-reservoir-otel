package helmmgr

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// InstallChart installs a Helm chart
func InstallChart(
	releaseName string,
	namespace string,
	chartPath string,
	values map[string]interface{},
	createNamespace bool,
) error {
	// Create a temporary values file
	valuesFile, err := createValuesFile(values)
	if err != nil {
		return fmt.Errorf("failed to create values file: %w", err)
	}
	defer os.Remove(valuesFile)

	// Create namespace if needed
	if createNamespace {
		cmd := exec.Command("kubectl", "create", "namespace", namespace, "--dry-run=client", "-o", "yaml")
		output, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("failed to generate namespace manifest: %w", err)
		}

		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(string(output))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to create namespace: %w", err)
		}
	}

	// Install the chart
	args := []string{
		"upgrade", "--install",
		"--namespace", namespace,
		"--values", valuesFile,
		releaseName,
		chartPath,
	}

	cmd := exec.Command("helm", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install chart: %w", err)
	}

	return nil
}

// InstallChartWithValuesFile installs a Helm chart with values from a file
func InstallChartWithValuesFile(
	releaseName string,
	namespace string,
	chartPath string,
	values map[string]interface{},
	valuesFile string,
	createNamespace bool,
) error {
	// Create a temporary values file for the dynamic values
	tempValuesFile, err := createValuesFile(values)
	if err != nil {
		return fmt.Errorf("failed to create temp values file: %w", err)
	}
	defer os.Remove(tempValuesFile)

	// Create namespace if needed
	if createNamespace {
		cmd := exec.Command("kubectl", "create", "namespace", namespace, "--dry-run=client", "-o", "yaml")
		output, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("failed to generate namespace manifest: %w", err)
		}

		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(string(output))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to create namespace: %w", err)
		}
	}

	// Install the chart with both values files
	args := []string{
		"upgrade", "--install",
		"--namespace", namespace,
		"--values", valuesFile,
		"--values", tempValuesFile,
		releaseName,
		chartPath,
	}

	cmd := exec.Command("helm", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install chart: %w", err)
	}

	return nil
}

// UninstallChart uninstalls a Helm release
func UninstallChart(releaseName string, namespace string) error {
	cmd := exec.Command("helm", "uninstall", "--namespace", namespace, releaseName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to uninstall chart: %w", err)
	}

	return nil
}

// createValuesFile creates a temporary YAML file with the provided values
func createValuesFile(values map[string]interface{}) (string, error) {
	// Convert map to YAML
	var yaml strings.Builder
	for key, value := range values {
		if err := writeYAML(&yaml, key, value, 0); err != nil {
			return "", err
		}
	}

	// Create temporary file
	file, err := os.CreateTemp("", "helm-values-*.yaml")
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Write YAML to file
	if _, err := file.WriteString(yaml.String()); err != nil {
		return "", err
	}

	return file.Name(), nil
}

// writeYAML recursively writes a map as YAML
func writeYAML(sb *strings.Builder, key string, value interface{}, indent int) error {
	indentStr := strings.Repeat("  ", indent)
	
	switch v := value.(type) {
	case map[string]interface{}:
		sb.WriteString(fmt.Sprintf("%s%s:\n", indentStr, key))
		
		for k, val := range v {
			if err := writeYAML(sb, k, val, indent+1); err != nil {
				return err
			}
		}
		
	case map[string]string:
		sb.WriteString(fmt.Sprintf("%s%s:\n", indentStr, key))
		
		for k, val := range v {
			sb.WriteString(fmt.Sprintf("%s  %s: %s\n", indentStr, k, formatYAMLValue(val)))
		}
		
	case []string:
		sb.WriteString(fmt.Sprintf("%s%s:\n", indentStr, key))
		
		for _, val := range v {
			sb.WriteString(fmt.Sprintf("%s  - %s\n", indentStr, formatYAMLValue(val)))
		}
		
	case []interface{}:
		sb.WriteString(fmt.Sprintf("%s%s:\n", indentStr, key))
		
		for _, val := range v {
			sb.WriteString(fmt.Sprintf("%s  - ", indentStr))
			
			if m, ok := val.(map[string]interface{}); ok {
				sb.WriteString("\n")
				for k, mval := range m {
					if err := writeYAML(sb, k, mval, indent+2); err != nil {
						return err
					}
				}
			} else {
				sb.WriteString(fmt.Sprintf("%s\n", formatYAMLValue(val)))
			}
		}
		
	default:
		sb.WriteString(fmt.Sprintf("%s%s: %s\n", indentStr, key, formatYAMLValue(value)))
	}
	
	return nil
}

// formatYAMLValue formats a value for YAML
func formatYAMLValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		if strings.Contains(v, ":") || strings.Contains(v, "#") || strings.HasPrefix(v, "{") {
			return fmt.Sprintf("%q", v)
		}
		return v
		
	case bool:
		return fmt.Sprintf("%t", v)
		
	case int, int32, int64:
		return fmt.Sprintf("%d", v)
		
	case float32, float64:
		return fmt.Sprintf("%g", v)
		
	default:
		return fmt.Sprintf("%v", v)
	}
}
