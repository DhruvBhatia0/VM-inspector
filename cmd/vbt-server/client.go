// client is the local CLI side of vbt-server: it talks to the hub
// daemon over its unix socket. This is the surface an LLM (or a script)
// calls.
package main

import (
	"context"
	"fmt"

	"github.com/DhruvBhatia0/VM-inspector/internal/proto"
)

type client struct{ socketPath string }

func newClient(socketPath string) *client { return &client{socketPath: socketPath} }

// exec runs one command on the VM via the hub and returns its result.
func (c *client) exec(ctx context.Context, req proto.ExecRequest) (proto.ExecResult, error) {
	return proto.ExecResult{}, fmt.Errorf("not implemented")
}
