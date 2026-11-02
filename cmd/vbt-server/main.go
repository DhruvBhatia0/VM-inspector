// vbt-server is the VM_bypassed_terminal server, on the operator's
// machine. It has two modes:
//
//	serve - the daemon: accept the agent tunnel, expose exec over a unix socket
//	exec  - the client: run one command on the VM, print the result
//
// The exec mode is the LLM-facing surface; with --json its output is
// machine-parseable (exit code, captured bytes, no terminal involved).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/DhruvBhatia0/VM-inspector/internal/proto"
)

const defaultSocket = "/tmp/vbt.sock"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = cmdServe(os.Args[2:])
	case "exec":
		err = cmdExec(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "vbt-server:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, strings.TrimSpace(`
usage:
  vbt-server serve [-addr 127.0.0.1:8765] [-socket path] -token T
  vbt-server exec [-socket path] [-json] [-timeout 30] [-cwd dir] -- cmd args...
`)+"\n")
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:8765", "tunnel listen address")
	socket := fs.String("socket", defaultSocket, "unix socket path for local clients")
	token := fs.String("token", os.Getenv("VBT_TOKEN"), "shared secret required of the agent")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return newHub(config{
		listenAddr: *addr,
		token:      *token,
		socketPath: *socket,
	}).serve(context.Background())
}

func cmdExec(args []string) error {
	fs := flag.NewFlagSet("exec", flag.ExitOnError)
	socket := fs.String("socket", defaultSocket, "unix socket path of the daemon")
	jsonOut := fs.Bool("json", false, "print the result as JSON")
	timeout := fs.Float64("timeout", 30, "command timeout in seconds")
	cwd := fs.String("cwd", "", "working directory on the VM")
	if err := fs.Parse(args); err != nil {
		return err
	}
	argv := fs.Args()
	if len(argv) == 0 {
		return fmt.Errorf("no command given (use: vbt-server exec -- cmd args...)")
	}
	_ = jsonOut // output formatting lands with the CLI phase
	_, err := newClient(*socket).exec(context.Background(), proto.ExecRequest{
		Argv:        argv,
		Cwd:         *cwd,
		TimeoutSecs: *timeout,
	})
	return err
}
