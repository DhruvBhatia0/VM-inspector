// hub is the daemon side of vbt-server, on the operator's machine: it
// accepts the agent's outbound connection and exposes the tunnel to
// local clients over a unix socket. Transport of the agent tunnel is
// an implementation detail (WebSocket today).
package main

import (
	"context"
	"fmt"

	"github.com/DhruvBhatia0/VM-inspector/internal/proto"
)

type config struct {
	listenAddr string // tunnel listen address, e.g. "127.0.0.1:8765"
	token      string // shared secret required of the agent
	socketPath string // unix socket path for local clients
}

type hub struct{ cfg config }

func newHub(cfg config) *hub { return &hub{cfg: cfg} }

// serve accepts the agent tunnel and local clients until ctx is done.
// Only one agent connection at a time is meaningful; a second is rejected.
func (h *hub) serve(ctx context.Context) error {
	return fmt.Errorf("not implemented")
}

// exec sends one command to the connected agent and waits for its
// result. Errors if no agent is connected.
func (h *hub) exec(ctx context.Context, req proto.ExecRequest) (proto.ExecResult, error) {
	return proto.ExecResult{}, fmt.Errorf("not implemented")
}
