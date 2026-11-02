# VM_bypassed_terminal

An exec channel into a VM that has **no inbound access**. The agent
inside the VM dials out to your server (the ws-term trick), and the
tunnel then runs commands on the VM and returns stdout/stderr/exit
codes — structured, so LLMs can explore the box without a terminal.

## Pieces

| | runs where | role |
|---|---|---|
| `vbt-server serve` | your machine | daemon: accepts the tunnel + a local unix socket |
| `vbt-server exec` | your machine | client: one command, result to stdout |
| the ledger | your machine | append-only JSONL log of every command/response |

## Build

    go build -o bin/vbt-server ./cmd/vbt-server
    GOOS=linux GOARCH=arm64 go build -o bin/vbt-agent ./cmd/vbt-agent

## Run

1. Start the daemon (the endpoint the agent dials; behind ngrok if the
   VM can only reach the public internet):

       bin/vbt-server serve -token "$(openssl rand -hex 16)"

2. Start the agent inside the VM — however you get code run there. For a
   hosted-agent sandbox, ask the sandbox's own agent to run it:

       vbt-agent -url ws://<server>/tunnel -token <same token>

3. Run commands from your machine:

### For an LLM

       vbt-server exec --json -- ls -la /memory

    One JSON object per call:

       {"stdout":"...\n","stderr":"","exit_code":0,"timed_out":false,"duration_ms":31}

    Branch on `exit_code` (the ground truth); read `stdout`/`stderr`;
    compose on the VM itself (`... -- grep -rn x /memory | head`) to keep
    responses small. `-timeout N` seconds, `-cwd dir` working directory.

### For a human

       vbt-server exec -- ls -la

    Raw stdout to your terminal, stderr to stderr, and the command's
    exit code becomes `$?`.

## Test harness

    ./test.sh

Builds both binaries, runs `vbt-agent` inside the Lima VM via an ssh -R
reverse forward (the stand-in for the production ngrok path), and runs
seven checks: ls roundtrip, grep sequencing + cwd, exit-code propagation,
stderr separation, timeout kill, bad-token rejection, ledger recording. ~15s.

## Ledger

The daemon maintains a ledger on its own: every exec roundtrip is appended
as one JSON line — argv, cwd, timeout, exit code, timed_out, duration, and
captured stdout/stderr (capped at 64 KiB per stream, truncation flagged).
Transport failures are recorded too (exit_code −1, the error in stderr).
A `session_start` line marks each daemon run; the file persists across
sessions.

    vbt-server serve -ledger /tmp/vbt.ledger.jsonl   # default path
    jq -c '{ts, argv, exit_code}' /tmp/vbt.ledger.jsonl   # read it back
