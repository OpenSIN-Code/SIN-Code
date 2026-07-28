// SPDX-License-Identifier: MIT
// Purpose: package-level hook variables used by tests to mock IO, time, and DB calls.
package autonomy

import (
	"database/sql"
	"os"
	"time"
)

var (
	_dbOpen = sql.Open

	_dbExec         = (*sql.DB).Exec
	_dbExecContext  = (*sql.DB).ExecContext
	_dbQueryContext = (*sql.DB).QueryContext

	_dbBeginTx = (*sql.DB).BeginTx

	_txQueryRowContext = (*sql.Tx).QueryRowContext
	_txExecContext     = (*sql.Tx).ExecContext
	_txCommit          = (*sql.Tx).Commit

	_userHomeDir = os.UserHomeDir
	_mkdirAll    = os.MkdirAll

	_timeNow       = time.Now
	_timeSince     = time.Since
	_newTicker     = time.NewTicker
	_parseDuration = time.ParseDuration
	_fingerprint   = fingerprint

	_dirEntryInfo = os.DirEntry.Info
)
