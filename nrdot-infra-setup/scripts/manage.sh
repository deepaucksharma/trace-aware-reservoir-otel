#!/bin/bash

# Directory where this script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
# Parent directory of the script
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$ROOT_DIR"

# Function to show usage
usage() {
    echo "Usage: $0 {start|stop|restart|status|logs|update}"
    echo "  start      - Start all services"
    echo "  stop       - Stop all services"
    echo "  restart    - Restart all services"
    echo "  status     - Show the status of all services"
    echo "  logs [svc] - Show logs for all services or a specific service (newrelic-infra or nrdot-collector)"
    echo "  update     - Update Docker images and restart services"
    exit 1
}

# Check if at least one argument is provided
if [ $# -lt 1 ]; then
    usage
fi

case "$1" in
    start)
        echo "Starting services..."
        docker-compose up -d
        ;;
    stop)
        echo "Stopping services..."
        docker-compose down
        ;;
    restart)
        echo "Restarting services..."
        docker-compose restart
        ;;
    status)
        echo "Services status:"
        docker-compose ps
        ;;
    logs)
        if [ $# -eq 2 ]; then
            echo "Showing logs for $2..."
            docker-compose logs -f "$2"
        else
            echo "Showing logs for all services..."
            docker-compose logs -f
        fi
        ;;
    update)
        echo "Updating Docker images..."
        docker-compose down
        docker pull newrelic/infrastructure:latest
        docker pull otel/opentelemetry-collector-contrib:latest
        docker-compose up -d
        echo "Services updated and restarted."
        ;;
    *)
        usage
        ;;
esac