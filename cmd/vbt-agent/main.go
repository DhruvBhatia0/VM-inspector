// vbt-agent is the VM_bypassed_terminal agent: it runs inside the VM,
// dials out to the operator's server, and executes the commands it is
// sent. The dial-out transport is an implementation detail (WebSocket
// today); the agent's job is exec, not transport.
//
// Usage:
//
//	vbt-agent -url ws://host:port/tunnel -token T
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/DhruvBhatia0/VM-inspector/internal/proto"

	"github.com/coder/websocket"
)

// main parses -url/-token (or VBT_URL/VBT_TOKEN), traps Ctrl-C, and
// exits nonzero on error.
func main() {
	urlFlag := flag.String("url", os.Getenv("VBT_URL"), "server tunnel endpoint (ws://host:port/tunnel)")
	token := flag.String("token", os.Getenv("VBT_TOKEN"), "shared secret")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, *urlFlag, *token); err != nil {
		fmt.Fprintln(os.Stderr, "vbt-agent:", err)
		os.Exit(1)
	}
}

// run appends the token to the URL, dials out, then loops reading
// TerminalCommands frames and writing back ExecOutput frames until the
// tunnel drops.
func run(ctx context.Context, rawURL, token string) error {
	if rawURL == "" || token == "" {
		return fmt.Errorf("need -url and -token (or VBT_URL / VBT_TOKEN)")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()

	conn, _, err := websocket.Dial(ctx, u.String(), nil)
	if err != nil {
		return err
	}
	defer conn.CloseNow()
	conn.SetReadLimit(-1)
	fmt.Fprintf(os.Stderr, "vbt-agent: connected to %s\n", u.Host)

	r := runner{}
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		var f proto.Frame
		if json.Unmarshal(data, &f) != nil {
			continue
		}
		if f.Kind != proto.TerminalCommands || f.Exec == nil {
			continue
		}
		res := r.exec(ctx, *f.Exec)
		out, err := json.Marshal(proto.Frame{Kind: proto.ExecOutput, ID: f.ID, Result: &res})
		if err != nil {
			continue
		}
		if err := conn.Write(ctx, websocket.MessageText, out); err != nil {
			return err
		}
	}
}

// runner executes commands on the VM with this process's permissions.
type runner struct{}

// exec runs req, capturing stdout/stderr and the exit status.
// No TTY, no streaming: the result is delivered whole.
func (runner) exec(pctx context.Context, req proto.ExecRequest) proto.ExecResult {
	start := time.Now()
	res := proto.ExecResult{ExitCode: -1}
	if len(req.Argv) == 0 {
		res.Stderr = "vbt-agent: empty argv"
		return res
	}

	ctx := pctx
	if req.TimeoutSecs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(pctx, time.Duration(req.TimeoutSecs*float64(time.Second)))
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, req.Argv[0], req.Argv[1:]...)
	if req.Cwd != "" {
		cmd.Dir = req.Cwd
	}
	if len(req.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range req.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	err := cmd.Run()
	res.Stdout, res.Stderr = stdout.String(), stderr.String()
	res.DurationMs = time.Since(start).Milliseconds()

	if ctx.Err() == context.DeadlineExceeded {
		res.TimedOut = true // killed by SIGKILL; exit stays -1
		return res
	}
	if err == nil {
		res.ExitCode = 0
		return res
	}
	if ee, ok := err.(*exec.ExitError); ok {
		res.ExitCode = ee.ExitCode() // -1 if signaled
	}
	return res
}
