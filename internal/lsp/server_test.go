package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func writeFramedJSON(t *testing.T, buf *bytes.Buffer, v any) {
	t.Helper()
	if err := WriteMessage(buf, v); err != nil {
		t.Fatalf("failed to frame message: %v", err)
	}
}

func readFramedResponse(t *testing.T, r *bufio.Reader) ResponseMessage {
	t.Helper()
	payload, err := ReadMessage(r)
	if err != nil {
		t.Fatalf("failed to read response message: %v", err)
	}
	var resp ResponseMessage
	if err := json.Unmarshal(payload, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v (raw=%s)", err, string(payload))
	}
	return resp
}

func TestServerLifecycle(t *testing.T) {
	var in bytes.Buffer
	var out bytes.Buffer

	// 1. initialize request
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"rootUri": "file:///workspace",
			"clientInfo": map[string]any{
				"name":    "test-editor",
				"version": "1.0.0",
			},
		},
	})
	// 2. initialized notification
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"method":  "initialized",
		"params":  map[string]any{},
	})
	// 3. unknown request (should return MethodNotFound)
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "textDocument/unknownFeature",
	})
	// 4. shutdown request
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "shutdown",
	})
	// 5. exit notification
	writeFramedJSON(t, &in, map[string]any{
		"jsonrpc": "2.0",
		"method":  "exit",
	})

	srv := NewServer(&in, &out, nil)
	if err := srv.Run(); err != nil {
		t.Fatalf("Server.Run() returned unexpected error: %v", err)
	}

	outReader := bufio.NewReader(&out)

	// Verify response 1: initialize
	resp1 := readFramedResponse(t, outReader)
	if string(resp1.ID) != "1" {
		t.Errorf("resp1 ID = %s, want 1", string(resp1.ID))
	}
	if resp1.Error != nil {
		t.Fatalf("resp1 unexpected error: %+v", resp1.Error)
	}
	resultBytes, err := json.Marshal(resp1.Result)
	if err != nil {
		t.Fatalf("marshaling resp1.Result: %v", err)
	}
	var initResult InitializeResult
	if err := json.Unmarshal(resultBytes, &initResult); err != nil {
		t.Fatalf("unmarshaling InitializeResult: %v", err)
	}
	if initResult.ServerInfo == nil || initResult.ServerInfo.Name != ServerName {
		t.Errorf("ServerInfo = %+v, want Name=%q", initResult.ServerInfo, ServerName)
	}
	if initResult.Capabilities.TextDocumentSync != SyncFull {
		t.Errorf("TextDocumentSync = %v, want %v", initResult.Capabilities.TextDocumentSync, SyncFull)
	}

	// Verify response 2: unknown method -> CodeMethodNotFound
	resp2 := readFramedResponse(t, outReader)
	if string(resp2.ID) != "2" {
		t.Errorf("resp2 ID = %s, want 2", string(resp2.ID))
	}
	if resp2.Error == nil || resp2.Error.Code != CodeMethodNotFound {
		t.Errorf("resp2 Error = %+v, want Code=%d", resp2.Error, CodeMethodNotFound)
	}

	// Verify response 3: shutdown -> result: null, error: nil
	resp3 := readFramedResponse(t, outReader)
	if string(resp3.ID) != "3" {
		t.Errorf("resp3 ID = %s, want 3", string(resp3.ID))
	}
	if resp3.Error != nil {
		t.Errorf("resp3 unexpected error: %+v", resp3.Error)
	}
	if resp3.Result != nil {
		t.Errorf("resp3 Result = %v, want nil", resp3.Result)
	}
}

func TestServerErrors(t *testing.T) {
	tests := []struct {
		name        string
		messages    []map[string]any
		wantErr     error
		wantErrCode int
	}{
		{
			name: "request before initialize returns ServerNotInitialized",
			messages: []map[string]any{
				{
					"jsonrpc": "2.0",
					"id":      1,
					"method":  "textDocument/hover",
				},
			},
			wantErrCode: CodeServerNotInitialized,
		},
		{
			name: "exit without shutdown returns ErrExitWithoutShutdown",
			messages: []map[string]any{
				{
					"jsonrpc": "2.0",
					"method":  "exit",
				},
			},
			wantErr: ErrExitWithoutShutdown,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var in, out bytes.Buffer
			for _, m := range tc.messages {
				writeFramedJSON(t, &in, m)
			}

			srv := NewServer(&in, &out, nil)
			err := srv.Run()

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Run() error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() unexpected error: %v", err)
			}

			if tc.wantErrCode != 0 {
				resp := readFramedResponse(t, bufio.NewReader(&out))
				if resp.Error == nil || resp.Error.Code != tc.wantErrCode {
					t.Errorf("got error %+v, want code %d", resp.Error, tc.wantErrCode)
				}
			}
		})
	}
}
