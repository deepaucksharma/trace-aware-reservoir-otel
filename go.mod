module github.com/deepaksharma/trace-aware-reservoir-otel

go 1.23.0

toolchain go1.24.2

replace github.com/deepaksharma/trace-aware-reservoir-otel => ./

require (
	github.com/boltdb/bolt v1.3.1
	github.com/cespare/xxhash/v2 v2.3.0
	github.com/golang/protobuf v1.5.4
	github.com/google/uuid v1.6.0
	github.com/robfig/cron/v3 v3.0.1
	github.com/stretchr/testify v1.10.0
	go.opentelemetry.io/collector/component v1.31.0
	go.opentelemetry.io/collector/consumer v1.31.0
	go.opentelemetry.io/collector/consumer/consumertest v0.125.0
	go.opentelemetry.io/collector/pdata v1.31.0
	go.opentelemetry.io/collector/processor v1.31.0
	go.opentelemetry.io/collector/processor/processortest v0.125.0
	go.opentelemetry.io/otel v1.35.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.35.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.35.0
	go.opentelemetry.io/otel/metric v1.35.0
	go.opentelemetry.io/otel/sdk v1.35.0
	go.opentelemetry.io/otel/trace v1.35.0
	go.uber.org/atomic v1.11.0
	go.uber.org/zap v1.27.0
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.26.1 // indirect
	github.com/hashicorp/go-version v1.7.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/collector/component/componentstatus v0.125.0 // indirect
	go.opentelemetry.io/collector/component/componenttest v0.125.0 // indirect
	go.opentelemetry.io/collector/consumer/xconsumer v0.125.0 // indirect
	go.opentelemetry.io/collector/featuregate v1.31.0 // indirect
	go.opentelemetry.io/collector/internal/telemetry v0.125.0 // indirect
	go.opentelemetry.io/collector/pdata/pprofile v0.125.0 // indirect
	go.opentelemetry.io/collector/pdata/testdata v0.125.0 // indirect
	go.opentelemetry.io/collector/pipeline v0.125.0 // indirect
	go.opentelemetry.io/collector/processor/xprocessor v0.125.0 // indirect
	go.opentelemetry.io/contrib/bridges/otelzap v0.10.0 // indirect
	go.opentelemetry.io/otel/log v0.11.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.35.0 // indirect
	go.opentelemetry.io/proto/otlp v1.5.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/net v0.39.0 // indirect
	golang.org/x/sys v0.32.0 // indirect
	golang.org/x/text v0.24.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250218202821-56aae31c358a // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250218202821-56aae31c358a // indirect
	google.golang.org/grpc v1.72.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
