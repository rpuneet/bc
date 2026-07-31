//go:build !cgo

package db

// On non-CGO builds (the Windows release sets CGO_ENABLED=0) the C-backed
// github.com/mattn/go-sqlite3 driver compiles to a stub that registers no
// driver, so sql.Open("sqlite3", …) would fail at runtime. We substitute the
// pure-Go modernc.org/sqlite driver and register it under the SAME name
// ("sqlite3") so every sql.Open("sqlite3", …) call works unchanged on Windows.
//
// modernc's package init already registers itself under "sqlite"; we add a
// second registration under "sqlite3". Driver.Open only reads its (nil) maps,
// so a zero-value &sqlite.Driver{} is safe to use here.

import (
	"database/sql"

	sqlite "modernc.org/sqlite"
)

func init() {
	sql.Register("sqlite3", &sqlite.Driver{})
}
