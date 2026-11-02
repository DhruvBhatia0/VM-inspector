// client is the local CLI side of vbt-server: it talks to the hub
// daemon over its unix socket. This is the surface an LLM (or a script)
// calls.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	"github.com/DhruvBhatia0/VM-inspector/internal/proto"
)

type client struct{ socketPath string }

// newClient returns a client that talks to the daemon at socketPath.
func newClient(socketPath string) *client { return &client{socketPath: socketPath} }

// exec sends one command to the hub and waits for its result.
func (c *client) exec(ctx context.Context, req proto.ExecRequest) (proto.ExecResult, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", c.socketPath)
	if err != nil {
		return proto.ExecResult{}, fmt.Errorf("daemon not running? (%w)", err)
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(proto.Frame{Kind: proto.TerminalCommands, ID: "1", Exec: &req}); err != nil {
		return proto.ExecResult{}, err
	}
	var reply proto.Frame
	if err := json.NewDecoder(conn).Decode(&reply); err != nil {
		return proto.ExecResult{}, err
	}
	if reply.Kind != proto.ExecOutput || reply.Result == nil {
		return proto.ExecResult{}, fmt.Errorf("bad reply from daemon")
	}
	return *reply.Result, nil
}
