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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/DhruvBhatia0/VM-inspector/internal/proto"
)

const defaultSocket = "/tmp/vbt.sock"

// main dispatches the serve/exec subcommands and maps errors to exit
// codes (an exitError exits with the VM command's own status).
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
		var ec exitError
		if errors.As(err, &ec) {
			os.Exit(ec.code)
		}
		fmt.Fprintln(os.Stderr, "vbt-server:", err)
		os.Exit(1)
	}
}

// usage prints the CLI summary to stderr.
func usage() {
	fmt.Fprint(os.Stderr, strings.TrimSpace(`
usage:
  vbt-server serve [-addr 127.0.0.1:8765] [-socket path] -token T
  vbt-server exec [-socket path] [-json] [-timeout 30] [-cwd dir] -- cmd args...
`)+"\n")
}

// cmdServe parses serve flags and runs the daemon until interrupted.
func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:8765", "tunnel listen address")
	socket := fs.String("socket", defaultSocket, "unix socket path for local clients")
	token := fs.String("token", os.Getenv("VBT_TOKEN"), "shared secret required of the agent")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return newHub(config{
		listenAddr: *addr,
		token:      *token,
		socketPath: *socket,
	}).serve(ctx)
}

// exitError propagates the VM-side exit code to the human caller's $?.
type exitError struct{ code int }

// Error implements error.
func (e exitError) Error() string { return "exit status " + strconv.Itoa(e.code) }

// cmdExec parses exec flags, sends the command through the daemon,
// then prints the result as JSON (--json) or raw with the command's
// exit code propagated to $?.
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

	// client waits a little longer than the VM-side kill so a timeout
	// surfaces as timed_out, not as a lost reply
	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration((*timeout+10)*float64(time.Second)))
	defer cancel()

	res, err := newClient(*socket).exec(ctx, proto.ExecRequest{
		Argv:        argv,
		Cwd:         *cwd,
		TimeoutSecs: *timeout,
	})
	if err != nil {
		return err
	}
	if *jsonOut {
		return json.NewEncoder(os.Stdout).Encode(res)
	}
	os.Stdout.WriteString(res.Stdout)
	os.Stderr.WriteString(res.Stderr)
	if res.TimedOut {
		return errors.New("command timed out")
	}
	if res.ExitCode < 0 {
		return errors.New("command did not run")
	}
	if res.ExitCode > 0 {
		return exitError{res.ExitCode}
	}
	return nil
}
