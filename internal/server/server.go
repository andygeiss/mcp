// Package server implements the MCP server lifecycle, dispatch, and resilience.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andygeiss/mcp/internal/prompts"
	"github.com/andygeiss/mcp/internal/protocol"
	"github.com/andygeiss/mcp/internal/resources"
	"github.com/andygeiss/mcp/internal/tools"
)

const (
	defaultHandlerTimeout = 30 * time.Second
	defaultSafetyMargin   = time.Second
	// maxPendingRequests caps the server-to-client request correlation map to
	// prevent unbounded memory growth from misbehaving handlers.
	maxPendingRequests = 1024
)

// Compile-time guarantee that *Server satisfies protocol.Peer. Any signature
// drift breaks the build before tests run.
var _ protocol.Peer = (*Server)(nil)

// Server states. stateUninitialized must be iota 0 so the zero value of
// Server.state matches the uninitialized lifecycle stage.
const (
	stateUninitialized = iota
	stateInitializing
	stateReady
)

// pendingEntry tracks a single in-flight outbound (server-to-client) request.
// respChan is buffer-1 and is NEVER closed (Invariant I3 — closing races the
// cancel path; cleanup is `delete(map, id)` and the unreferenced channel is GC'd).
type pendingEntry struct {
	createdAt time.Time
	method    string
	respChan  chan *protocol.Response
}

// Server is the MCP server that communicates over stdin/stdout using JSON-RPC 2.0.
// It manages a three-state lifecycle: uninitialized → initializing (after
// initialize request) → ready (after notifications/initialized). Methods
// other than ping and initialize are rejected until the server reaches the
// ready state.
type Server struct {
	cancelInFlight    context.CancelFunc
	clientCaps        atomic.Pointer[protocol.ClientCapabilities]
	counting          *countingReader
	dec               *json.Decoder
	done              chan struct{}
	enc               *json.Encoder
	errorCount        atomic.Int64
	handlerTimeout    time.Duration
	inFlightCancelled atomic.Bool
	inFlightCh        chan inFlightResult
	inFlightID        json.RawMessage
	instructions      string
	logLevel          *slog.LevelVar
	logger            *slog.Logger
	name              string
	outboundIDCounter atomic.Int64
	pending           map[string]pendingEntry
	pendingMu         sync.Mutex
	prompts           *prompts.Registry
	registry          *tools.Registry
	requestCount      atomic.Int64
	resources         *resources.Registry
	safetyMargin      time.Duration
	state             int
	stdoutMu          sync.Mutex
	trace             bool
	version           string
}

// Option configures the Server.
type Option func(*Server)

// WithHandlerTimeout sets the maximum duration for tool handler execution.
// The default is 30 seconds.
func WithHandlerTimeout(d time.Duration) Option {
	return func(s *Server) {
		s.handlerTimeout = d
	}
}

// WithInstructions sets the free-form usage guidance returned in the
// initialize response. Clients may surface this text to the user.
func WithInstructions(instructions string) Option {
	return func(s *Server) {
		s.instructions = instructions
	}
}

// WithPrompts attaches a prompt registry to the server. When set, the
// server advertises the prompts capability and dispatches prompts/ methods.
func WithPrompts(p *prompts.Registry) Option {
	return func(s *Server) {
		s.prompts = p
	}
}

// WithResources attaches a resource registry to the server. When set, the
// server advertises the resources capability and dispatches resources/ methods.
func WithResources(r *resources.Registry) Option {
	return func(s *Server) {
		s.resources = r
	}
}

// WithSafetyMargin sets the additional time after handler timeout before the
// safety timer fires to force-fail unresponsive handlers. The default is 1s.
func WithSafetyMargin(d time.Duration) Option {
	return func(s *Server) {
		s.safetyMargin = d
	}
}

// WithTrace enables protocol trace mode. When enabled, every incoming request
// and outgoing response is logged to stderr at debug level. Zero overhead
// when disabled. Enabling trace elevates the logger's level to debug so the
// trace entries are actually emitted — any subsequent logging/setLevel call
// from the client may lower it again.
func WithTrace(enabled bool) Option {
	return func(s *Server) {
		s.trace = enabled
	}
}

// NewServer creates a new MCP server. The name and version identify the server
// in initialize responses. The registry holds all registered tools. Pass
// io.Reader/io.Writer (not *os.File) so tests can inject bytes.Buffer for
// stdin/stdout/stderr. Protocol output goes to stdout exclusively; all
// diagnostics are logged to stderr via slog.JSONHandler.
func NewServer(name, version string, registry *tools.Registry, stdin io.Reader, stdout, stderr io.Writer, opts ...Option) *Server {
	cr := newCountingReader(stdin, maxMessageSize)
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	levelVar := new(slog.LevelVar)
	levelVar.Set(slog.LevelInfo)
	s := &Server{
		counting:       cr,
		dec:            json.NewDecoder(cr),
		done:           make(chan struct{}),
		enc:            enc,
		handlerTimeout: defaultHandlerTimeout,
		logLevel:       levelVar,
		logger:         slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: levelVar})),
		name:           name,
		pending:        make(map[string]pendingEntry),
		registry:       registry,
		safetyMargin:   defaultSafetyMargin,
		state:          stateUninitialized,
		version:        version,
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.trace {
		s.logLevel.Set(slog.LevelDebug)
	}
	return s
}

// Run starts the dispatch loop. It decodes messages from stdin and dispatches
// them to the appropriate handler. Tool calls are dispatched asynchronously so
// the decode loop can continue reading messages for cancellation and ping
// support while a handler is in flight. The server advertises maxInFlight: 1,
// so only one tool handler runs at a time; additional requests while a handler
// is in flight are rejected with -32600.
//
// Run returns nil for clean shutdown (EOF or context cancellation) or an error
// for fatal decode conditions (after sending a -32700 response).
func (s *Server) Run(ctx context.Context) error {
	startTime := time.Now()
	s.logger.Info("server_started",
		"name", s.name,
		"protocol_version", protocol.MCPVersion,
		"tools", s.registry.Names(),
		"version", s.version,
	)
	var runErr error
	defer func() {
		close(s.done)
		s.logShutdown(ctx, runErr, startTime)
	}()

	decodeCh := make(chan decodeResult, 1)
	startDecode := func() {
		go func() {
			s.counting.Reset()
			incoming, err := protocol.DecodeMessage(s.dec)
			exceeded := s.counting.Exceeded()
			if err == nil && incoming.IsResponse {
				respCopy := incoming.Response
				s.routeResponse(&respCopy)
				decodeCh <- decodeResult{routed: true, exceeded: exceeded}
				return
			}
			decodeCh <- decodeResult{msg: incoming.Request, err: err, exceeded: exceeded}
		}()
	}

	startDecode()

	for {
		var loopErr error
		if s.inFlightCh != nil {
			loopErr = s.runInFlight(ctx, decodeCh, startDecode)
		} else {
			loopErr = s.runIdle(ctx, decodeCh, startDecode)
		}
		if errors.Is(loopErr, errShutdown) {
			return runErr
		}
		if loopErr != nil {
			runErr = loopErr
			return runErr
		}
	}
}

// SendRequest sends a JSON-RPC 2.0 request to the client and waits for the
// response. Enables server-to-client capabilities like sampling, elicitation,
// and roots/list. Safe for concurrent use from handler goroutines.
//
// Cancellation semantics (AI7 + ADR-003 §Failure Modes):
//   - Client response → return *Response, nil.
//   - Server shutdown (s.done closed) → return nil, protocol.ErrServerShutdown.
//   - Handler ctx cancel → emit notifications/cancelled with the outbound ID
//     BEFORE pending-entry cleanup, then return ctx.Err(). Shutdown takes
//     priority over ctx.Done via a post-select manual check.
func (s *Server) SendRequest(ctx context.Context, method string, params any) (*protocol.Response, error) {
	// AI9 capability gate: side-effect-free synchronous reject when the client
	// did not advertise the capability needed by the method.
	if needed, gated := protocol.MethodCapability(method); gated {
		if !s.clientCaps.Load().Has(needed) {
			return nil, &protocol.CapabilityNotAdvertisedError{Capability: needed, Method: method}
		}
	}

	id := s.outboundIDCounter.Add(1)
	idJSON := strconv.AppendInt(nil, id, 10)

	respChan, cleanup, err := s.registerPending(id, method)
	if err != nil {
		return nil, err
	}

	raw, err := json.Marshal(params)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("marshal request params: %w", err)
	}

	req := protocol.Request{
		ID:      json.RawMessage(idJSON),
		JSONRPC: protocol.Version,
		Method:  method,
		Params:  json.RawMessage(raw),
	}

	s.stdoutMu.Lock()
	err = s.enc.Encode(&req)
	s.stdoutMu.Unlock()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("encode request: %w", err)
	}

	select {
	case resp := <-respChan:
		cleanup()
		return resp, nil
	case <-s.done:
		// Cleanup is fine here too; the channel is GC'd either way.
		cleanup()
		return nil, protocol.ErrServerShutdown
	case <-ctx.Done():
		// Fall through to the priority check + A7 emission below.
	}

	// Priority check: shutdown wins over ctx.Done — never emit
	// notifications/cancelled to a client we are no longer talking to.
	select {
	case <-s.done:
		cleanup()
		return nil, protocol.ErrServerShutdown
	default:
	}

	// A7: emit cancel notification BEFORE removing the pending entry.
	s.emitOutboundCancel(req.ID, ctx.Err())
	cleanup()
	return nil, ctx.Err()
}

// emitOutboundCancel sends notifications/cancelled with the abandoned outbound
// ID. Best-effort: a writer error is logged at warn and not returned. Per AI7
// server→client symmetry, this MUST be invoked BEFORE the pending-entry
// cleanup so that observable state (still-present entry while cancel is in
// flight) is consistent.
func (s *Server) emitOutboundCancel(id json.RawMessage, cause error) {
	reason := "context_canceled"
	if cause != nil {
		reason = cause.Error()
	}
	params := struct {
		Reason    string          `json:"reason,omitempty"`
		RequestID json.RawMessage `json:"requestId"`
	}{
		Reason:    reason,
		RequestID: id,
	}
	if err := s.sendNotification(protocol.NotificationCancelled, params); err != nil {
		s.logger.Warn("outbound_cancel_emit_failed", "id", string(id), "error", err)
	}
}

// registerPending atomically reserves a slot in the pending map and returns
// the response channel + a cleanup closure. Callers MUST invoke cleanup once
// the outbound request is no longer in flight (response received, ctx
// cancelled, or shutdown). cleanup is idempotent and never closes respChan
// (Invariant I3). Returns protocol.ErrPendingRequestsFull when the map is at
// capacity (NFR-R3 scenario d). This is the SOLE insert site for s.pending.
func (s *Server) registerPending(id int64, method string) (<-chan *protocol.Response, func(), error) {
	key := strconv.FormatInt(id, 10)
	ch := make(chan *protocol.Response, 1)
	s.pendingMu.Lock()
	if len(s.pending) >= maxPendingRequests {
		s.pendingMu.Unlock()
		return nil, nil, protocol.ErrPendingRequestsFull
	}
	s.pending[key] = pendingEntry{createdAt: time.Now(), method: method, respChan: ch}
	s.pendingMu.Unlock()
	cleanup := func() {
		s.pendingMu.Lock()
		delete(s.pending, key)
		s.pendingMu.Unlock()
	}
	return ch, cleanup, nil
}

// routeResponse delivers a client response to the pending request map.
// If no pending request matches the response ID, the response is silently
// dropped (logged at warn). A non-blocking send under the lock prevents a
// duplicate response from deadlocking the decode goroutine if the buffered
// channel is full.
func (s *Server) routeResponse(resp *protocol.Response) {
	s.pendingMu.Lock()
	entry, ok := s.pending[string(resp.ID)]
	if ok {
		delete(s.pending, string(resp.ID))
	}
	s.pendingMu.Unlock()
	if !ok {
		s.logger.Warn("late_outbound_response", "id", string(resp.ID))
		return
	}
	select {
	case entry.respChan <- resp:
	default:
	}
}

// logShutdown emits the structured shutdown log entry.
func (s *Server) logShutdown(ctx context.Context, runErr error, startTime time.Time) {
	var reason string
	switch {
	case runErr != nil:
		reason = "fatal_error"
	case ctx.Err() != nil:
		reason = "context_cancelled"
		if c := context.Cause(ctx); c != nil {
			reason = c.Error()
		}
	default:
		reason = "eof"
	}
	s.logger.Info("server_stopped",
		"errors", s.errorCount.Load(),
		"reason", reason,
		"requests", s.requestCount.Load(),
		"uptime_ms", time.Since(startTime).Milliseconds(),
	)
}
