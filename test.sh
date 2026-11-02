#!/usr/bin/env bash
# VM_bypassed_terminal end-to-end harness.
#
#   Mac:  vbt-server (daemon + client)      Lima VM "jfsbench": vbt-agent
#   The agent dials OUT through an ssh -R reverse forward (the ngrok
#   stand-in from the blog: the sandbox has no inbound, so the tunnel is
#   always VM -> operator).
#
# Six checks. ~15s. Real binaries, real VM, no mocks.
set -u
cd "$(dirname "$0")"

TOKEN="vbt-harness-$$"
SOCK="/tmp/vbt-harness-$$.sock"
VM=jfsbench
RPORT=18999
FAILS=0
SERVER_PID= SSH_PID=

cleanup() {
  [ -n "${SERVER_PID:-}" ] && kill "$SERVER_PID" 2>/dev/null
  [ -n "${SSH_PID:-}" ] && kill "$SSH_PID" 2>/dev/null
  limactl shell "$VM" -- pkill vbt-agent 2>/dev/null
  rm -f "$SOCK" bin/vbt-agent
  return 0
}
trap cleanup EXIT

say() { printf '%s\n' "$*"; }

check() {
  local name=$1; shift
  if "$@" >/dev/null 2>&1; then say "  PASS  $name"; else say "  FAIL  $name"; FAILS=$((FAILS+1)); fi
}

say "== build"
go build -o bin/vbt-server ./cmd/vbt-server || exit 1
GOOS=linux GOARCH=arm64 go build -o bin/vbt-agent ./cmd/vbt-agent || exit 1

say "== deploy agent to VM ($VM)"
limactl copy bin/vbt-agent "$VM:/tmp/vbt-agent" || exit 1

say "== start daemon, tunnel, agent"
bin/vbt-server serve -addr 127.0.0.1:8765 -socket "$SOCK" -token "$TOKEN" &
SERVER_PID=$!
for _ in $(seq 50); do [ -S "$SOCK" ] && break; sleep 0.1; done
[ -S "$SOCK" ] || { say "daemon socket never appeared"; exit 1; }

ssh -F ~/.lima/$VM/ssh.config -o ControlMaster=no -o ControlPath=none -N -R ${RPORT}:localhost:8765 lima-$VM &
SSH_PID=$!
sleep 1

limactl shell "$VM" -- /tmp/vbt-agent -url ws://localhost:${RPORT}/tunnel -token "$TOKEN" &

wait_ready() {
  for _ in $(seq 50); do
    bin/vbt-server exec -socket "$SOCK" -- true >/dev/null 2>&1 && return 0
    sleep 0.2
  done
  return 1
}
wait_ready || { say "agent never connected"; exit 1; }
say "  agent connected"

V() { bin/vbt-server exec -socket "$SOCK" --json "$@"; }

say "== checks"
c_ls()      { V ls / | jq -e '.exit_code == 0 and (.stdout | contains("tmp"))'; }
c_grep()    {
  V sh -c 'echo vbt-needle > /tmp/vbt-fx.txt' >/dev/null &&
  bin/vbt-server exec -socket "$SOCK" --json -cwd /tmp grep needle vbt-fx.txt |
    jq -e '.exit_code == 0 and (.stdout | contains("needle"))'
}
c_exit()    { V sh -c 'exit 42' | jq -e '.exit_code == 42'; }
c_fail()    { V ls /definitely/not-there | jq -e '.exit_code != 0 and (.stdout | length == 0) and (.stderr | length > 0)'; }
c_timeout() { bin/vbt-server exec -socket "$SOCK" --json -timeout 1 sleep 60 | jq -e '.timed_out == true'; }
c_badauth() { ! limactl shell "$VM" -- timeout 8 /tmp/vbt-agent -url ws://localhost:${RPORT}/tunnel -token wrong >/dev/null 2>&1; }

check "ls: stdout roundtrip"      c_ls
check "grep: sequencing + cwd"    c_grep
check "exit code propagation"     c_exit
check "failure: stderr separation" c_fail
check "timeout kill"              c_timeout
check "bad token rejected"        c_badauth

say ""
say "agent reports: $(V uname -srm | jq -r .stdout | tr -d '\n')"
say ""
if [ "$FAILS" -eq 0 ]; then say "ALL PASS"; else say "$FAILS CHECK(S) FAILED"; exit 1; fi
