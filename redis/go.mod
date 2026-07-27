module github.com/mtgo-labs/contrib/redis

go 1.26.0

require github.com/mtgo-labs/raw v0.0.0

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/redis/go-redis/v9 v9.21.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
)

replace github.com/mtgo-labs/raw => ../../mtcute-go
