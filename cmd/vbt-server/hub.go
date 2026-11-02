// hub is the daemon side of vbt-server, on the operator's machine: it
// accepts the agent's outbound connection and exposes the tunnel to
// local clients over a unix socket. Transport of the agent tunnel is
// an implementation detail (WebSocket today).
package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/DhruvBhatia0/VM-inspector/internal/proto"

	"github.com/coder/websocket"
)

type config struct {
	listenAddr string // tunnel listen address, e.g. "127.0.0.1:8765"
	token      string // shared secret required of the agent
	socketPath string // unix socket path for local clients
}

type hub struct {
	cfg config
	ctx context.Context

	seq     atomic.Uint64
	mu      sync.Mutex
	agent   *websocket.Conn
	pending map[string]chan proto.ExecResult
}

// newHub returns a hub with an empty pending table; the agent slot
// fills when an agent dials in.
func newHub(cfg config) *hub {
	return &hub{cfg: cfg, pending: map[string]chan proto.ExecResult{}}
}

// serve accepts the agent tunnel and local clients until ctx is done.
// One agent at a time; a second is rejected.
func (h *hub) serve(ctx context.Context) error {
	if h.cfg.token == "" {
		return fmt.Errorf("serve needs -token")
	}
	h.ctx = ctx

	_ = os.Remove(h.cfg.socketPath)
	ln, err := net.Listen("unix", h.cfg.socketPath)
	if err != nil {
		return err
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go h.serveClient(conn)
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/tunnel", h.serveAgent)
	srv := &http.Server{Addr: h.cfg.listenAddr, Handler: mux}
	errC := make(chan error, 1)
	go func() { errC <- srv.ListenAndServe() }()
	log.Printf("vbt-server: tunnel on %s, exec on %s", h.cfg.listenAddr, h.cfg.socketPath)

	select {
	case err := <-errC:
		return err
	case <-ctx.Done():
		srv.Close()
		return nil
	}
}

// serveAgent is the WebSocket endpoint the agent dials out to.
func (h *hub) serveAgent(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("token")), []byte(h.cfg.token)) != 1 {
		log.Printf("vbt-server: agent rejected (bad token) from %s", r.RemoteAddr)
		http.Error(w, "bad token", http.StatusForbidden)
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(-1)

	h.mu.Lock()
	if h.agent != nil {
		h.mu.Unlock()
		conn.Close(1008, "another agent is connected")
		return
	}
	h.agent = conn
	h.mu.Unlock()
	log.Printf("vbt-server: agent connected from %s", r.RemoteAddr)
	defer func() {
		h.mu.Lock()
		if h.agent == conn {
			h.agent = nil
		}
		h.mu.Unlock()
		conn.CloseNow()
		log.Printf("vbt-server: agent gone")
	}()

	for {
		_, data, err := conn.Read(h.ctx)
		if err != nil {
			return
		}
		var f proto.Frame
		if json.Unmarshal(data, &f) != nil {
			continue
		}
		if f.Kind == proto.ExecOutput && f.Result != nil {
			h.mu.Lock()
			ch := h.pending[f.ID]
			delete(h.pending, f.ID)
			h.mu.Unlock()
			if ch != nil {
				ch <- *f.Result
			}
		}
	}
}

// exec sends one command to the connected agent and waits for its
// result. Errors if no agent is connected.
func (h *hub) exec(ctx context.Context, req proto.ExecRequest) (proto.ExecResult, error) {
	h.mu.Lock()
	agent := h.agent
	h.mu.Unlock()
	if agent == nil {
		return proto.ExecResult{}, fmt.Errorf("no agent connected")
	}

	id := strconv.FormatUint(h.seq.Add(1), 10)
	ch := make(chan proto.ExecResult, 1)
	h.mu.Lock()
	h.pending[id] = ch
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.pending, id)
		h.mu.Unlock()
	}()

	frame, err := json.Marshal(proto.Frame{Kind: proto.TerminalCommands, ID: id, Exec: &req})
	if err != nil {
		return proto.ExecResult{}, err
	}
	if err := agent.Write(ctx, websocket.MessageText, frame); err != nil {
		return proto.ExecResult{}, fmt.Errorf("tunnel write: %w", err)
	}

	select {
	case res := <-ch:
		return res, nil
	case <-ctx.Done():
		return proto.ExecResult{}, ctx.Err()
	}
}

// serveClient handles one local client: one request in, one reply out.
func (h *hub) serveClient(conn net.Conn) {
	defer conn.Close()
	var f proto.Frame
	if json.NewDecoder(conn).Decode(&f) != nil {
		return
	}
	if f.Kind != proto.TerminalCommands || f.Exec == nil {
		return
	}
	res, err := h.exec(h.ctx, *f.Exec)
	if err != nil {
		res = proto.ExecResult{ExitCode: -1, Stderr: "vbt-server: " + err.Error()}
	}
	json.NewEncoder(conn).Encode(proto.Frame{Kind: proto.ExecOutput, ID: f.ID, Result: &res})
}
