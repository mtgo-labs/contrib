// Package sqlite provides a SQLite-backed session store for mtgo-labs/raw,
// matching the @mtcute/sqlite storage API.
// The store persists session state (auth keys, salts, DC configuration)
// in a local SQLite database using modernc.org/sqlite — a pure-Go SQLite
// implementation with zero CGO dependencies.
//
// Usage:
//
//	import (
//	    raw "github.com/mtgo-labs/raw"
//	    "github.com/mtgo-labs/contrib/sqlite"
//	)
//
//	store, err := sqlite.NewStore("session.db")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer store.Close()
//
//	client, err := raw.NewClient(raw.Config{
//	    APIID:    12345,
//	    APIHash:  "...",
//	    BotToken: "...",
//	    Store:    store,
//	})
//
// The package also exposes repository types for direct use:
//
//	store.AuthKeys.Set(dcID, key)
//	if err := store.KV.Set("my_key", value); err != nil {
//	    log.Fatal(err)
//	}
package sqlite
