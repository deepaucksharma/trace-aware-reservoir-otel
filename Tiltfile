# Tiltfile for trace-aware-reservoir-otel
# This enables hot-reloading during development

# Load environment variables
load('ext://dotenv', 'dotenv')
dotenv()

# Use the license key from environment, or provide a default for development
default_license_key = os.getenv('NEW_RELIC_KEY', 'development-key')

# Set up the Docker build
docker_build(
    'ghcr.io/deepaucksharma/nrdot-reservoir:dev',
    '.',
    dockerfile='Dockerfile.multistage',
    build_args={
        'NRDOT_VERSION': 'v0.91.0',
        'RS_VERSION': 'dev',
    },
    live_update=[
        sync('./internal', '/go/src/github.com/deepaucksharma/trace-aware-reservoir-otel/internal'),
        sync('./cmd', '/go/src/github.com/deepaucksharma/trace-aware-reservoir-otel/cmd'),
        run('cd /go/src/github.com/deepaucksharma/trace-aware-reservoir-otel && go install ./cmd/...'),
    ]
)

# Deploy with Helm
k8s_yaml(helm(
    'charts/reservoir',
    name='otel-reservoir',
    namespace='otel',
    values=['./charts/reservoir/values.yaml'],
    set=[
        'global.licenseKey=' + default_license_key,
        'global.cluster=development',
        'image.repository=ghcr.io/deepaucksharma/nrdot-reservoir',
        'image.tag=dev',
    ]
))

# Port forwards for debugging
k8s_resource(
    'otel-collector',
    port_forwards=[
        '8888:8888',  # Metrics
        '4317:4317',  # OTLP gRPC
        '4318:4318',  # OTLP HTTP
    ],
    resource_deps=[]
)

# Watch for changes in these directories
watch_file('./internal')
watch_file('./cmd')
watch_file('./charts')
