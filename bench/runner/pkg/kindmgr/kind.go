package kindmgr

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// EnsureCluster creates a Kind cluster if it doesn't exist
func EnsureCluster(name, configPath string) error {
	// Check if cluster already exists
	cmd := exec.Command("kind", "get", "clusters")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to get clusters: %w", err)
	}

	// Check if our cluster is in the list
	clusters := strings.Split(string(output), "\n")
	for _, cluster := range clusters {
		if strings.TrimSpace(cluster) == name {
			fmt.Printf("Kind cluster %s already exists\n", name)
			return nil
		}
	}

	// Create the cluster
	fmt.Printf("Creating Kind cluster %s...\n", name)
	
	configArg := []string{}
	if configPath != "" {
		// Resolve the path relative to the current directory
		absConfigPath, err := filepath.Abs(configPath)
		if err != nil {
			return fmt.Errorf("failed to resolve config path: %w", err)
		}
		
		// Check if the config file exists
		if _, err := os.Stat(absConfigPath); err != nil {
			return fmt.Errorf("config file not found: %s", absConfigPath)
		}
		
		configArg = []string{"--config", absConfigPath}
	}
	
	args := append([]string{"create", "cluster", "--name", name}, configArg...)
	cmd = exec.Command("kind", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create cluster: %w", err)
	}
	
	// Wait for cluster to be ready
	time.Sleep(5 * time.Second)
	
	return nil
}

// DeleteCluster deletes a Kind cluster
func DeleteCluster(name string) error {
	fmt.Printf("Deleting Kind cluster %s...\n", name)
	
	cmd := exec.Command("kind", "delete", "cluster", "--name", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete cluster: %w", err)
	}
	
	return nil
}

// LoadImage loads a Docker image into a Kind cluster
func LoadImage(clusterName, image string) error {
	fmt.Printf("Loading image %s into Kind cluster %s...\n", image, clusterName)
	
	cmd := exec.Command("kind", "load", "docker-image", "--name", clusterName, image)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to load image: %w", err)
	}
	
	return nil
}

// GetKubeconfig returns the kubeconfig path for a Kind cluster
func GetKubeconfig(clusterName string) (string, error) {
	tempDir, err := os.MkdirTemp("", "kind-kubeconfig")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	
	kubeconfigPath := filepath.Join(tempDir, "kubeconfig")
	
	cmd := exec.Command("kind", "get", "kubeconfig", "--name", clusterName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get kubeconfig: %w", err)
	}
	
	if err := os.WriteFile(kubeconfigPath, output, 0644); err != nil {
		return "", fmt.Errorf("failed to write kubeconfig: %w", err)
	}
	
	return kubeconfigPath, nil
}
