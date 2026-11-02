// vbt-agent is the VM_bypassed_terminal agent: it runs inside the VM,
// dials out to the operator's server, and executes the commands it is
// sent. The dial-out transport is an implementation detail (WebSocket
// today); the agent's job is exec, not transport.
//
// Usage (once implemented):
//
//	vbt-agent -url … -token …
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/DhruvBhatia0/VM-inspector/internal/proto"
)

func main() {
	url := flag.String("url", os.Getenv("VBT_URL"), "server endpoint (as given by the server)")
	token := flag.String("token", os.Getenv("VBT_TOKEN"), "shared secret")
	flag.Parse()

	_ = url
	_ = token

	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "vbt-agent:", err)
		os.Exit(1)
	}
}

func run() error {
	return fmt.Errorf("not implemented")
}

// runner executes commands on the VM with this process's permissions.
type runner struct{}

// exec runs req, capturing stdout/stderr and the exit status.
// No TTY, no streaming: the result is delivered whole.
func (runner) exec(req proto.ExecRequest) (proto.ExecResult, error) {
	return proto.ExecResult{}, fmt.Errorf("not implemented")
}
