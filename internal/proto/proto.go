// Package proto defines the frames exchanged between the agent (inside
// the VM) and the server (on the operator's machine) over the tunnel.
// The tunnel's transport is an implementation detail (WebSocket today).
//
// Scope decision: exec only. There is deliberately no PTY/stream frame;
// if one is ever added, it gets a new PayloadKind here and nowhere else.
package proto

// PayloadKind identifies what a frame on the wire carries.
type PayloadKind string

const (
	TerminalCommands PayloadKind = "terminalCommands" // client → agent: run commands
	ExecOutput       PayloadKind = "execOutput"       // agent → client: their outcome
)

// Frame is the envelope for every message. ID correlates a result with
// its request.
type Frame struct {
	Kind   PayloadKind `json:"type"`
	ID     string       `json:"id"`
	Exec   *ExecRequest `json:"exec,omitempty"`
	Result *ExecResult  `json:"result,omitempty"`
}

// ExecRequest asks the agent to run one command with no TTY attached.
type ExecRequest struct {
	Argv        []string          `json:"argv"`                 // e.g. ["ls","-la","/memory"]
	Cwd         string            `json:"cwd,omitempty"`        // "" = agent default
	Env         map[string]string `json:"env,omitempty"`        // applied on top of agent env
	TimeoutSecs float64          `json:"timeout_s,omitempty"`  // 0 = agent default
}

// ExecResult is the exact outcome of an ExecRequest: raw captured bytes
// plus the exit status. ExitCode is the ground truth for LLM consumers.
type ExecResult struct {
	Stdout      string `json:"stdout"`
	Stderr      string `json:"stderr"`
	ExitCode    int    `json:"exit_code"` // -1 if killed by signal or not run
	TimedOut    bool   `json:"timed_out"`
	DurationMs  int64  `json:"duration_ms"`
}
