#!/bin/bash
# Script to install the reservoir sampler dashboard to New Relic

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

# Check for New Relic API Key
if [ -z "$NR_API_KEY" ]; then
  echo -e "${RED}Error: NR_API_KEY environment variable is required${NC}"
  echo "Please set your New Relic API key:"
  echo "export NR_API_KEY=NRAK-XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
  exit 1
fi

# Check for optional account ID
if [ -z "$NR_ACCOUNT_ID" ]; then
  echo -e "${YELLOW}Warning: NR_ACCOUNT_ID not set${NC}"
  echo "The dashboard will be installed to the account associated with your API key."
  echo "To specify an account, set:"
  echo "export NR_ACCOUNT_ID=your_account_id"
  echo ""
  read -p "Do you want to continue? (y/n) " -n 1 -r
  echo ""
  if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    exit 1
  fi
else
  echo "Using New Relic account ID: $NR_ACCOUNT_ID"
fi

# Prepare dashboard payload
section "Installing reservoir sampler dashboard"

DASHBOARD_FILE="$(dirname $0)/reservoir-dashboard.json"

if [ ! -f "$DASHBOARD_FILE" ]; then
  echo -e "${RED}Error: Dashboard file not found: $DASHBOARD_FILE${NC}"
  exit 1
fi

# Read the dashboard file
DASHBOARD_JSON=$(cat "$DASHBOARD_FILE")

# Replace account ID in dashboard JSON if specified
if [ -n "$NR_ACCOUNT_ID" ]; then
  DASHBOARD_JSON=$(echo "$DASHBOARD_JSON" | sed "s/\"accountId\": 0/\"accountId\": $NR_ACCOUNT_ID/g")
fi

# Create temporary file for the dashboard payload
TEMP_FILE=$(mktemp)
echo "{\"dashboard\": $DASHBOARD_JSON}" > "$TEMP_FILE"

# Install dashboard to New Relic
curl -X POST "https://api.newrelic.com/graphql" \
  -H "Content-Type: application/json" \
  -H "API-Key: $NR_API_KEY" \
  -d @"$TEMP_FILE" \
  -o /dev/null

if [ $? -eq 0 ]; then
  echo -e "${GREEN}✓ Dashboard installed successfully${NC}"
else
  echo -e "${RED}Failed to install dashboard${NC}"
  exit 1
fi

# Clean up
rm "$TEMP_FILE"

echo -e "${GREEN}✓ Reservoir Sampler dashboard has been installed to your New Relic account${NC}"
echo "You can find it in New Relic One under Dashboards with the name:"
echo "Trace-Aware Reservoir Sampling Dashboard"