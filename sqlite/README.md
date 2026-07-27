# sqlite

SQLite-backed session store for `mtgo-labs/raw`.

```go
import "github.com/mtgo-labs/contrib/sqlite"
```

## Overview

- Persists session state (auth keys, salts, DC configuration) in a local SQLite database
- Pure Go via `modernc.org/sqlite` — zero CGO dependencies
- Matches the `@mtcute/sqlite` storage API
- Exposes repository types for direct use: `AuthKeys`, `KV`, `Peers`, `RefMessages`

## Usage

```go
store, err := sqlite.NewStore("session.db")
defer store.Close()

client, err := raw.NewClient(raw.Config{
    Store: store,
})

// Direct repository access
store.AuthKeys.Set(dcID, key)
if err := store.KV.Set("my_key", value); err != nil {
    log.Fatal(err)
}
```

Dependencies: `modernc.org/sqlite`.
