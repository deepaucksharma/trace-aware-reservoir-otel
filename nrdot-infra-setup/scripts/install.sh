#!/bin/bash

set -e

# Directory where this script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
# Parent directory of the script
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

echo "Setting up New Relic Infrastructure Agent and NRDOT Collector dual setup..."

# Check if Docker is installed
if ! command -v docker &> /dev/null; then
    echo "Docker is not installed. Please install Docker first."
    exit 1
fi

# Check if Docker Compose is installed
if ! command -v docker-compose &> /dev/null; then
    echo "Docker Compose is not installed. Please install Docker Compose first."
    exit 1
fi

# Check if .env file exists
if [ ! -f "$ROOT_DIR/.env" ]; then
    echo "Creating .env file from example..."
    cp "$ROOT_DIR/.env.example" "$ROOT_DIR/.env"
    echo "Please edit $ROOT_DIR/.env to set your New Relic license key."
    exit 1
fi

# Source the .env file to get the license key
source "$ROOT_DIR/.env"

# Check if license key is set
if [ "$NR_LICENSE_KEY" = "your_new_relic_license_key_here" ]; then
    echo "Please edit $ROOT_DIR/.env to set your New Relic license key."
    exit 1
fi

# Pull the required Docker images
echo "Pulling Docker images..."
docker pull newrelic/infrastructure:latest
docker pull otel/opentelemetry-collector-contrib:latest

# Start the services
echo "Starting services..."
cd "$ROOT_DIR"
docker-compose up -d

echo "Installation complete. Both services are now running."
echo "Use './scripts/manage.sh status' to check the status of the services."
echo "Use './scripts/manage.sh logs' to view the logs."