module github.com/mtgo-labs/contrib/middleware

go 1.26.0

require (
	github.com/mtgo-labs/contrib/retry v0.0.0
	github.com/mtgo-labs/raw v0.0.0
)


// replace directives for local development (remove before tagging)
replace (
	github.com/mtgo-labs/raw => ../../mtcute-go
	github.com/mtgo-labs/contrib/retry => ../retry
)
