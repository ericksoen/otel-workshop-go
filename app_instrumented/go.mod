module oteldemo.com/demo/go

go 1.13

require (
	github.com/joho/godotenv v1.3.0
	go.opentelemetry.io/otel v0.4.2
	go.opentelemetry.io/otel/exporters/metric/prometheus v0.4.2 // indirect
	go.opentelemetry.io/otel/exporters/trace/zipkin v0.4.2
)
