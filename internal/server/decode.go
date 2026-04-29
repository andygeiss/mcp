package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/andygeiss/mcp/internal/protocol"
)

// decodeResult carries the outcome of an async decode operation.
type decodeResult struct {
	err      error
	exceeded bool
	msg      protocol.Request
	routed   bool // true if the message was a response routed to the pending map
}

// errShutdown is a sentinel indicating clean shutdown from ctx.Done or EOF.
var errShutdown = errors.New("shutdown")

// runInFlight handles one iteration of the dispatch loop while a tool handler
// is in flight. Returns errShutdown for clean exit, a real error for fatal
// conditions, or nil to continue the loop.
func (s *Server) runInFlight(ctx context.Context, decodeCh chan decodeResult, startDecode func()) error {
	// Prioritize handler completion over new messages to avoid rejecting
	// requests that arrive after the handler has already finished.
	select {
	case ifr := <-s.inFlightCh:
		return s.processInFlightResult(ifr)
	default:
	}

	select {
	case ifr := <-s.inFlightCh:
		return s.processInFlightResult(ifr)

	case dr := <-decodeCh:
		if dr.routed {
			startDecode()
			return nil
		}
		err := s.handleDecodeResultDuringInFlight(ctx, dr)
		if err != nil {
			return err
		}
		if (dr.err != nil && !isNonFatalDecodeError(dr.err)) || dr.exceeded {
			return errShutdown
		}
		startDecode()
		return nil

	case <-ctx.Done():
		s.cancelAndAwaitInFlight()
		return errShutdown
	}
}

// runIdle handles one iteration of the dispatch loop when no handler is in
// flight. Returns errShutdown for clean exit, a real error for fatal
// conditions, or nil to continue the loop.
func (s *Server) runIdle(ctx context.Context, decodeCh chan decodeResult, startDecode func()) error {
	select {
	case dr := <-decodeCh:
		if dr.routed {
			startDecode()
			return nil
		}
		err := s.handleDecodeResultIdle(ctx, dr)
		if err != nil {
			return err
		}
		if (dr.err != nil && !isNonFatalDecodeError(dr.err)) || dr.exceeded {
			return errShutdown
		}
		startDecode()
		return nil

	case <-ctx.Done():
		return errShutdown
	}
}

// isNonFatalDecodeError reports whether err is a decode-time failure that the
// dispatch loop should swallow rather than treat as connection-fatal. Today
// that is limited to *protocol.StructuralLimitError (M1a key-count and
// string-length caps).
func isNonFatalDecodeError(err error) bool {
	var sle *protocol.StructuralLimitError
	return errors.As(err, &sle)
}

// handleDecodeResultDuringInFlight processes a decode result that arrived while
// a tool handler is in flight. On decode error: waits for the handler, sends its
// response, then returns the decode error. On success: processes the message.
func (s *Server) handleDecodeResultDuringInFlight(ctx context.Context, dr decodeResult) error {
	if dr.err != nil || dr.exceeded {
		return s.handleDecodeErrorDuringInFlight(dr)
	}

	s.requestCount.Add(1)
	if s.trace {
		if raw, err := json.Marshal(dr.msg); err != nil {
			s.logger.Warn("trace_marshal_request", "error", err)
		} else {
			s.logger.Debug("trace_request", "direction", "←", "message", string(raw))
		}
	}

	// Check if handler completed concurrently with the decode.
	select {
	case ifr := <-s.inFlightCh:
		if err := s.processInFlightResult(ifr); err != nil {
			return err
		}
		return s.handleDecodeResultIdle(ctx, decodeResult{msg: dr.msg})
	default:
	}

	if err := s.handleMessageDuringInFlight(dr.msg); err != nil {
		s.cancelAndAwaitInFlight()
		return err
	}
	return nil
}

// handleDecodeErrorDuringInFlight processes a decode error while a tool handler
// is in flight. It waits for the handler to complete and sends its response
// before returning the decode error.
func (s *Server) handleDecodeErrorDuringInFlight(dr decodeResult) error {
	decErr := dr.err
	if dr.exceeded {
		decErr = fmt.Errorf("decode message: %w", errMessageTooLarge)
	}
	decodeRunErr := s.handleDecodeError(decErr)
	// Wait for handler. Use 2x the handler budget to outlast the internal safety timer.
	timer := time.NewTimer(2 * (s.handlerTimeout + s.safetyMargin))
	select {
	case ifr := <-s.inFlightCh:
		timer.Stop()
		if !s.inFlightCancelled.Load() {
			if ifr.isError {
				s.errorCount.Add(1)
			}
			if encErr := s.encodeResponse(ifr.resp); encErr != nil {
				decodeRunErr = errors.Join(decodeRunErr, encErr)
			}
		}
	case <-timer.C:
		s.logger.Warn("handler_abandoned", "request_id", string(s.inFlightID))
	}
	s.clearInFlight()
	return decodeRunErr
}

// handleDecodeResultIdle processes a decode result when no handler is in flight.
// Returns a non-nil error for fatal conditions.
func (s *Server) handleDecodeResultIdle(ctx context.Context, dr decodeResult) error {
	if dr.err != nil || dr.exceeded {
		decErr := dr.err
		if dr.exceeded {
			decErr = fmt.Errorf("decode message: %w", errMessageTooLarge)
		}
		return s.handleDecodeError(decErr)
	}

	s.requestCount.Add(1)
	if s.trace {
		if raw, err := json.Marshal(dr.msg); err != nil {
			s.logger.Warn("trace_marshal_request", "error", err)
		} else {
			s.logger.Debug("trace_request", "direction", "←", "message", string(raw))
		}
	}

	// Intercept tools/call for async dispatch when ready.
	if dr.msg.Method == protocol.MethodToolsCall && s.state == stateReady && len(dr.msg.ID) > 0 {
		if vErr := protocol.Validate(dr.msg); vErr != nil {
			return s.encodeResponse(s.errorResponse(dr.msg.ID, vErr))
		}
		errResp, started := s.startToolCallAsync(ctx, dr.msg)
		if !started {
			return s.encodeResponse(errResp)
		}
		return nil
	}

	// Normal synchronous dispatch for all other messages.
	resp, respond := s.dispatch(ctx, dr.msg)
	if respond {
		return s.encodeResponse(resp)
	}
	return nil
}

// handleMessageDuringInFlight processes messages that arrive while a tool
// handler is executing. Ping is answered immediately; notifications (including
// cancellation) are handled normally; all other requests are rejected with
// -32600 because dispatch is sequential.
func (s *Server) handleMessageDuringInFlight(msg protocol.Request) error {
	isNotification := len(msg.ID) == 0

	if vErr := protocol.Validate(msg); vErr != nil {
		if isNotification {
			return nil
		}
		return s.encodeResponse(s.errorResponse(msg.ID, vErr))
	}

	if isNotification {
		s.handleNotification(msg)
		return nil
	}

	if msg.Method == protocol.MethodPing {
		resp, err := protocol.NewResultResponse(msg.ID, json.RawMessage("{}"))
		if err != nil {
			return s.encodeResponse(s.errorResponse(msg.ID, protocol.ErrInternalError("failed to marshal ping result")))
		}
		return s.encodeResponse(resp)
	}

	return s.encodeResponse(s.errorResponse(msg.ID, protocol.ErrServerError("server busy: request in flight (sequential dispatch)")))
}

// handleDecodeError processes errors from the decoder, returning nil for clean
// shutdown (EOF) or an error for fatal conditions. The wire message is a
// sanitized, well-known string — raw decoder errors (which may echo user
// input or reveal stdlib internals) are confined to the stderr log.
func (s *Server) handleDecodeError(err error) error {
	// Check for size limit BEFORE EOF — the countingReader returns
	// errMessageTooLarge which errors.Is can match through any wrapping.
	isTooLarge := errors.Is(err, errMessageTooLarge)

	if !isTooLarge && (errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)) {
		return nil
	}

	s.errorCount.Add(1)

	// M1a: structural-limit breaches (key-count / string-length) are
	// non-fatal — emit a -32001 response (ServerTimeout reused per AC) and
	// let the loop continue.
	var sle *protocol.StructuralLimitError
	if errors.As(err, &sle) {
		s.logger.Warn("decode_structural_limit", "limit", sle.Limit, "actual", sle.Actual, "max", sle.Max)
		resp := protocol.NewErrorResponse(protocol.NullID(), protocol.ServerTimeout, sle.Error())
		s.stdoutMu.Lock()
		encErr := protocol.Encode(s.enc, resp)
		s.stdoutMu.Unlock()
		if encErr != nil {
			s.logger.Error("encode_error", "error", encErr)
		}
		return nil
	}

	var wire string
	switch {
	case isTooLarge:
		wire = "message exceeds 4MB size limit"
	case errors.Is(err, protocol.ErrBatchNotSupported):
		wire = protocol.ErrBatchNotSupported.Error()
	case errors.Is(err, protocol.ErrJSONDepthExceeded):
		wire = protocol.ErrJSONDepthExceeded.Error()
	default:
		wire = "parse error"
	}

	s.logger.Error("decode_error", "error", err)
	resp := protocol.NewErrorResponse(protocol.NullID(), protocol.ParseError, wire)
	s.stdoutMu.Lock()
	encErr := protocol.Encode(s.enc, resp)
	s.stdoutMu.Unlock()
	if encErr != nil {
		s.logger.Error("encode_error", "error", encErr)
	}
	return fmt.Errorf("fatal decode error: %w", err)
}
