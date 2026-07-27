module github.com/mtgo-labs/contrib/opentelemetry

go 1.26.0

require (
	github.com/mtgo-labs/raw v0.0.0
	go.opentelemetry.io/otel v1.44.0
	go.opentelemetry.io/otel/trace v1.44.0
)

require github.com/cespare/xxhash/v2 v2.3.0 // indirect


// replace directives for local development (remove before tagging)
replace (
	github.com/mtgo-labs/raw => ../../mtcute-go
)
