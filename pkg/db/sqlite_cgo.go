//go:build cgo

package db

// On CGO builds (macOS and Linux releases) we use the C-backed
// github.com/mattn/go-sqlite3 driver, which is faster than the pure-Go
// implementation. Importing it for its side effects registers the driver
// under the name "sqlite3", so sql.Open("sqlite3", …) works unchanged.
import _ "github.com/mattn/go-sqlite3"
