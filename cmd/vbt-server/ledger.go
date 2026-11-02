// ledger is the daemon's append-only audit log: one JSON object per line,
// recording every command executed through the tunnel and the response
// the VM returned. A session_start line marks each daemon run; history
// persists across sessions.
//
// Output streams are capped per entry (maxStreamBytes) so chunked bulk
// transfers don't bloat the file; truncation is flagged per stream.
package main

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/DhruvBhatia0/VM-inspector/internal/proto"
)

// maxStreamBytes caps how much of each output stream is recorded per entry.
const maxStreamBytes = 64 << 10

// Entry is one ledger line. Type distinguishes session_start from exec
// records; exec records leave it empty.
type Entry struct {
	Type string `json:"type,omitempty"`
	Ts   string `json:"ts"`

	ID      string   `json:"id,omitempty"`
	Argv    []string `json:"argv,omitempty"`
	Cwd     string   `json:"cwd,omitempty"`
	Timeout float64  `json:"timeout_s,omitempty"`

	ExitCode        int    `json:"exit_code"`
	TimedOut        bool   `json:"timed_out"`
	DurationMs      int64  `json:"duration_ms"`
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	StdoutTruncated bool   `json:"stdout_truncated,omitempty"`
	StderrTruncated bool   `json:"stderr_truncated,omitempty"`
}

// ledger serializes appends to one file. A nil *ledger records nothing.
type ledger struct {
	mu sync.Mutex
	f  *os.File
}

// openLedger opens (creating if needed) the ledger for appending and writes
// a session_start line. Failure to open yields a nil ledger, and the daemon
// runs without recording rather than refusing to serve.
func openLedger(path string) *ledger {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil
	}
	l := &ledger{f: f}
	l.write(Entry{Type: "session_start", Ts: nowISO()})
	return l
}

// record appends one exec roundtrip, capping the output streams.
func (l *ledger) record(id string, req proto.ExecRequest, res proto.ExecResult) {
	stdout, outTrunc := capStream(res.Stdout)
	stderr, errTrunc := capStream(res.Stderr)
	l.write(Entry{
		Ts:              nowISO(),
		ID:              id,
		Argv:            req.Argv,
		Cwd:             req.Cwd,
		Timeout:         req.TimeoutSecs,
		ExitCode:        res.ExitCode,
		TimedOut:        res.TimedOut,
		DurationMs:      res.DurationMs,
		Stdout:          stdout,
		Stderr:          stderr,
		StdoutTruncated: outTrunc,
		StderrTruncated: errTrunc,
	})
}

// write appends one entry; a nil ledger (or a marshal failure) is a no-op.
func (l *ledger) write(e Entry) {
	if l == nil || l.f == nil {
		return
	}
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.f.Write(append(data, '\n'))
}

// nowISO returns the current UTC time in RFC 3339.
func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// capStream truncates s to maxStreamBytes, reporting whether it did.
func capStream(s string) (string, bool) {
	if len(s) <= maxStreamBytes {
		return s, false
	}
	return s[:maxStreamBytes], true
}
