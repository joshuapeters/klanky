package cliutil

import (
	"encoding/json"
	"io"
)

// PrintJSONLine writes a single-line JSON encoding of data to w, terminated
// by a newline. This is the planning-agent-facing output contract for
// `feature new` and `task add` — the agent parses this to chain calls.
func PrintJSONLine(w io.Writer, data any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(data)
}
