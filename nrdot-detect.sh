#!/bin/bash
# NR-DOT detection and auto-configuration script
# This script detects existing NR-DOT installations and configures the reservoir sampler accordingly

# Color codes for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print section header
section() {
  echo ""
  echo -e "${BLUE}==>${NC} ${YELLOW}$1${NC}"
  echo ""
}

# Detect NR-DOT in Kubernetes environment
detect_nrdot_kubernetes() {
  section "Detecting NR-DOT in Kubernetes"
  
  # Define common namespaces to check
  NAMESPACES=("default" "monitoring" "observability" "newrelic" "opentelemetry")
  
  # Check each namespace for NR-DOT
  for ns in "${NAMESPACES[@]}"; do
    echo "Checking namespace: $ns"
    
    # Check if namespace exists
    if kubectl get namespace $ns &>/dev/null; then
      # Check for NR-DOT Helm release
      if helm list -n $ns | grep -q "nrdot"; then
        echo -e "${GREEN}✓ Found NR-DOT Helm release in namespace $ns${NC}"
        NRDOT_NAMESPACE=$ns
        NRDOT_DETECTED=true
        
        # Get the release name
        NRDOT_RELEASE=$(helm list -n $ns | grep nrdot | awk '{print $1}')
        echo "Release name: $NRDOT_RELEASE"
        
        # Check if reservoir sampler is already configured
        POD=$(kubectl get pods -n $ns -l app.kubernetes.io/name=nrdot-collector -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
        if [ -n "$POD" ]; then
          if kubectl exec -n $ns $POD -- grep -q "reservoir_sampler" /etc/otelcol-config.yaml 2>/dev/null; then
            echo -e "${YELLOW}Reservoir sampler already configured in this installation${NC}"
            RESERVOIR_CONFIGURED=true
          else
            echo -e "${GREEN}This NR-DOT installation can be extended with reservoir sampling${NC}"
          fi
        fi
        
        # No need to check other namespaces if we found one
        break
      fi
      
      # Check for NR-DOT pods directly
      if kubectl get pods -n $ns -l app.kubernetes.io/name=nrdot-collector &>/dev/null; then
        echo -e "${GREEN}✓ Found NR-DOT pods in namespace $ns${NC}"
        NRDOT_NAMESPACE=$ns
        NRDOT_DETECTED=true
        
        # Get the pod
        POD=$(kubectl get pods -n $ns -l app.kubernetes.io/name=nrdot-collector -o jsonpath='{.items[0].metadata.name}')
        echo "Pod name: $POD"
        
        # Check if reservoir sampler is already configured
        if kubectl exec -n $ns $POD -- grep -q "reservoir_sampler" /etc/otelcol-config.yaml 2>/dev/null; then
          echo -e "${YELLOW}Reservoir sampler already configured in this installation${NC}"
          RESERVOIR_CONFIGURED=true
        else
          echo -e "${GREEN}This NR-DOT installation can be extended with reservoir sampling${NC}"
        fi
        
        # No need to check other namespaces if we found one
        break
      fi
    fi
  done
  
  if [ -z "$NRDOT_DETECTED" ]; then
    echo -e "${YELLOW}No existing NR-DOT installation detected in common namespaces${NC}"
    return 1
  fi
  
  return 0
}

# Detect NR-DOT in Docker environment
detect_nrdot_docker() {
  section "Detecting NR-DOT in Docker"
  
  # Check for running NR-DOT containers
  if docker ps | grep -q "nrdot-collector"; then
    echo -e "${GREEN}✓ Found running NR-DOT container${NC}"
    NRDOT_DETECTED=true
    
    # Get the container ID
    CONTAINER_ID=$(docker ps | grep nrdot-collector | awk '{print $1}')
    echo "Container ID: $CONTAINER_ID"
    
    # Check if reservoir sampler is already configured
    if docker exec $CONTAINER_ID grep -q "reservoir_sampler" /etc/otelcol-config.yaml 2>/dev/null; then
      echo -e "${YELLOW}Reservoir sampler already configured in this container${NC}"
      RESERVOIR_CONFIGURED=true
    else
      echo -e "${GREEN}This NR-DOT container can be extended with reservoir sampling${NC}"
    fi
    
    return 0
  else
    echo -e "${YELLOW}No running NR-DOT containers detected${NC}"
    
    # Check for NR-DOT images
    if docker images | grep -q "nrdot-collector"; then
      echo -e "${GREEN}✓ Found NR-DOT images${NC}"
      NRDOT_IMAGE=$(docker images | grep nrdot-collector | head -1 | awk '{print $1":"$2}')
      echo "Image: $NRDOT_IMAGE"
      
      NRDOT_DETECTED=true
      return 0
    else
      echo -e "${YELLOW}No NR-DOT images found${NC}"
    fi
  fi
  
  return 1
}

# Detect NR-DOT license key from various sources
detect_license_key() {
  section "Detecting New Relic license key"
  
  # First, check environment variable
  if [ -n "$NR_LICENSE_KEY" ]; then
    echo -e "${GREEN}✓ Found NR_LICENSE_KEY environment variable${NC}"
    return 0
  fi
  
  # Check for license key in Kubernetes secrets
  if [ -n "$NRDOT_NAMESPACE" ]; then
    # Check common secret names
    SECRET_NAMES=("newrelic-secrets" "nr-license-key" "otel-collector-secrets")
    
    for secret in "${SECRET_NAMES[@]}"; do
      if kubectl get secret -n $NRDOT_NAMESPACE $secret &>/dev/null; then
        echo -e "${GREEN}✓ Found potential New Relic license key in secret: $secret${NC}"
        echo "You can use this existing secret in your configuration"
        NR_LICENSE_SECRET=$secret
        return 0
      fi
    done
  fi
  
  # Check for license key in common config files
  CONFIG_FILES=("~/.newrelic/config" "/etc/newrelic/config.yaml" "./newrelic.yml")
  
  for config in "${CONFIG_FILES[@]}"; do
    if [ -f "$config" ]; then
      if grep -q "license_key" "$config" 2>/dev/null; then
        echo -e "${GREEN}✓ Found potential New Relic license key in config file: $config${NC}"
        echo "You can extract this key for your configuration"
        return 0
      fi
    fi
  done
  
  echo -e "${YELLOW}No New Relic license key detected${NC}"
  echo "You'll need to provide a license key during installation"
  return 1
}

# Generate a configuration report
generate_report() {
  section "NR-DOT Installation Report"
  
  # Create report file
  REPORT_FILE="nrdot-detection-report.txt"
  
  echo "NR-DOT Detection Report" > $REPORT_FILE
  echo "Generated at: $(date)" >> $REPORT_FILE
  echo "" >> $REPORT_FILE
  
  # Add detection results to report
  if [ "$NRDOT_DETECTED" = true ]; then
    echo "✅ NR-DOT Detected: Yes" >> $REPORT_FILE
    
    if [ -n "$NRDOT_NAMESPACE" ]; then
      echo "   Installation Type: Kubernetes" >> $REPORT_FILE
      echo "   Namespace: $NRDOT_NAMESPACE" >> $REPORT_FILE
      
      if [ -n "$NRDOT_RELEASE" ]; then
        echo "   Helm Release: $NRDOT_RELEASE" >> $REPORT_FILE
      fi
      
      if [ -n "$POD" ]; then
        echo "   Pod: $POD" >> $REPORT_FILE
      fi
    else
      echo "   Installation Type: Docker" >> $REPORT_FILE
      
      if [ -n "$CONTAINER_ID" ]; then
        echo "   Container ID: $CONTAINER_ID" >> $REPORT_FILE
      fi
      
      if [ -n "$NRDOT_IMAGE" ]; then
        echo "   Image: $NRDOT_IMAGE" >> $REPORT_FILE
      fi
    fi
    
    if [ "$RESERVOIR_CONFIGURED" = true ]; then
      echo "   Reservoir Sampler: Already configured" >> $REPORT_FILE
    else
      echo "   Reservoir Sampler: Not yet configured (can be added)" >> $REPORT_FILE
    fi
  else
    echo "❌ NR-DOT Detected: No" >> $REPORT_FILE
    echo "   No existing NR-DOT installation found" >> $REPORT_FILE
    echo "   A new installation will be required" >> $REPORT_FILE
  fi
  
  echo "" >> $REPORT_FILE
  echo "License Key Detection:" >> $REPORT_FILE
  
  if [ -n "$NR_LICENSE_KEY" ]; then
    echo "✅ License Key: Found in environment variables" >> $REPORT_FILE
  elif [ -n "$NR_LICENSE_SECRET" ]; then
    echo "✅ License Key: Found in Kubernetes secret ($NR_LICENSE_SECRET)" >> $REPORT_FILE
  else
    echo "❌ License Key: Not detected" >> $REPORT_FILE
    echo "   You'll need to provide a license key during installation" >> $REPORT_FILE
  fi
  
  echo "" >> $REPORT_FILE
  echo "Next Steps:" >> $REPORT_FILE
  
  if [ "$NRDOT_DETECTED" = true ] && [ "$RESERVOIR_CONFIGURED" != true ]; then
    echo "1. Modify the existing NR-DOT configuration to add reservoir sampler" >> $REPORT_FILE
    echo "2. Use './install.sh --extend-existing' to add reservoir sampler to existing installation" >> $REPORT_FILE
  elif [ "$NRDOT_DETECTED" = true ] && [ "$RESERVOIR_CONFIGURED" = true ]; then
    echo "1. Reservoir sampler is already configured in your NR-DOT installation" >> $REPORT_FILE
    echo "2. You may want to update the reservoir sampler configuration if needed" >> $REPORT_FILE
  else
    echo "1. Install a new NR-DOT instance with reservoir sampler using './install.sh'" >> $REPORT_FILE
    echo "2. Configure with appropriate settings for your environment" >> $REPORT_FILE
  fi
  
  echo "" >> $REPORT_FILE
  echo "For more information, see the NR-DOT documentation at:" >> $REPORT_FILE
  echo "https://docs.newrelic.com/docs/more-integrations/open-source-telemetry-integrations/opentelemetry/opentelemetry-setup/" >> $REPORT_FILE
  
  # Display the report path
  echo -e "${GREEN}Report generated: $REPORT_FILE${NC}"
  echo "You can view this report to help plan your installation"
}

# Main detection function
detect_nrdot() {
  echo "Detecting NR-DOT installation in your environment..."
  
  # Initialize variables
  NRDOT_DETECTED=false
  RESERVOIR_CONFIGURED=false
  
  # Check for kubectl to determine if we're in a Kubernetes environment
  if command -v kubectl &>/dev/null && command -v helm &>/dev/null; then
    detect_nrdot_kubernetes
    KUBECTL_AVAILABLE=$?
  else
    echo "Kubernetes tools not available, skipping Kubernetes detection"
    KUBECTL_AVAILABLE=1
  fi
  
  # If not found in Kubernetes or kubectl not available, check Docker
  if [ $KUBECTL_AVAILABLE -ne 0 ] || [ "$NRDOT_DETECTED" != true ]; then
    if command -v docker &>/dev/null; then
      detect_nrdot_docker
      DOCKER_AVAILABLE=$?
    else
      echo "Docker not available, skipping Docker detection"
      DOCKER_AVAILABLE=1
    fi
  fi
  
  # Try to detect license key
  detect_license_key
  
  # Generate a report
  generate_report
  
  # Provide a recommendation
  section "Recommendation"
  
  if [ "$NRDOT_DETECTED" = true ]; then
    if [ "$RESERVOIR_CONFIGURED" = true ]; then
      echo -e "${GREEN}NR-DOT with reservoir sampling is already configured in your environment${NC}"
      echo "You may want to update the configuration if needed"
      echo ""
      echo "To update, use:"
      echo "./install.sh --update-existing"
    else
      echo -e "${YELLOW}NR-DOT detected, but reservoir sampling is not configured${NC}"
      echo "You can extend your existing installation with:"
      echo "./install.sh --extend-existing"
    fi
  else
    echo -e "${YELLOW}No existing NR-DOT installation detected${NC}"
    echo "You can perform a new installation with:"
    echo "./install.sh"
  fi
  
  echo ""
  echo "See $REPORT_FILE for more details"
}

# Run the detection function if this script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  detect_nrdot
fi