package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const jsonRPCVersion = "2.0"

// Standard JSON-RPC 2.0 and LSP error codes.
const (
	CodeParseError           = -32700
	CodeInvalidRequest       = -32600
	CodeMethodNotFound       = -32601
	CodeInvalidParams        = -32602
	CodeInternalError        = -32603
	CodeServerNotInitialized = -32002
)

// IncomingMessage represents an incoming JSON-RPC 2.0 request or notification.
type IncomingMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// IsRequest returns true if the message includes an ID field (distinguishing it from a notification).
func (m *IncomingMessage) IsRequest() bool {
	return len(m.ID) > 0 && string(m.ID) != "null"
}

// ServerRequestMessage represents an outgoing JSON-RPC 2.0 request from server to client
// (e.g. "client/registerCapability").
type ServerRequestMessage struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// ResponseMessage represents an outgoing JSON-RPC 2.0 response.
type ResponseMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *ResponseError  `json:"error,omitempty"`
}

// MarshalJSON ensures that "result": null is explicitly serialized when Error is nil and Result is nil,
// as required by JSON-RPC 2.0 (e.g., for the "shutdown" response).
func (r ResponseMessage) MarshalJSON() ([]byte, error) {
	if r.Error != nil {
		return json.Marshal(struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Error   *ResponseError  `json:"error"`
		}{
			JSONRPC: r.JSONRPC,
			ID:      r.ID,
			Error:   r.Error,
		})
	}
	return json.Marshal(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  any             `json:"result"`
	}{
		JSONRPC: r.JSONRPC,
		ID:      r.ID,
		Result:  r.Result,
	})
}

// NotificationMessage represents an outgoing JSON-RPC 2.0 notification.
type NotificationMessage struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// ResponseError represents a JSON-RPC 2.0 error object.
type ResponseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// ReadMessage reads a single Content-Length framed JSON-RPC message from r.
func ReadMessage(r *bufio.Reader) ([]byte, error) {
	contentLength := -1

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			// Empty line marks the end of headers.
			break
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid header line: %q", line)
		}

		headerName := strings.TrimSpace(parts[0])
		headerValue := strings.TrimSpace(parts[1])

		if strings.EqualFold(headerName, "Content-Length") {
			n, err := strconv.Atoi(headerValue)
			if err != nil || n < 0 {
				return nil, fmt.Errorf("invalid Content-Length value: %q", headerValue)
			}
			contentLength = n
		}
	}

	if contentLength < 0 {
		return nil, errors.New("missing Content-Length header")
	}

	payload := make([]byte, contentLength)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, fmt.Errorf("reading message body: %w", err)
	}

	return payload, nil
}

// WriteMessage writes a single Content-Length framed JSON-RPC message to w.
func WriteMessage(w io.Writer, msg any) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling message: %w", err)
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := io.WriteString(w, header); err != nil {
		return fmt.Errorf("writing header: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("writing body: %w", err)
	}
	return nil
}
