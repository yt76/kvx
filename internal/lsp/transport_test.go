package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestReadMessage(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{
			name: "valid message with Content-Length",
			raw:  "Content-Length: 15\r\n\r\n{\"jsonrpc\":\"2\"}",
			want: `{"jsonrpc":"2"}`,
		},
		{
			name: "valid message with extra Content-Type header",
			raw:  "Content-Type: application/vscode-jsonrpc; charset=utf-8\r\nContent-Length: 17\r\n\r\n{\"method\":\"exit\"}",
			want: `{"method":"exit"}`,
		},
		{
			name:    "missing Content-Length header",
			raw:     "Content-Type: application/json\r\n\r\n{}",
			wantErr: true,
		},
		{
			name:    "invalid Content-Length number",
			raw:     "Content-Length: abc\r\n\r\n{}",
			wantErr: true,
		},
		{
			name:    "malformed header line",
			raw:     "Content-Length 10\r\n\r\n{}",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := bufio.NewReader(strings.NewReader(tc.raw))
			got, err := ReadMessage(r)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (payload=%q)", string(got))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("ReadMessage() = %q, want %q", string(got), tc.want)
			}
		})
	}
}

func TestWriteAndReadRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	resp := ResponseMessage{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Result:  nil,
	}

	if err := WriteMessage(&buf, resp); err != nil {
		t.Fatalf("WriteMessage failed: %v", err)
	}

	gotBytes, err := ReadMessage(bufio.NewReader(&buf))
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}

	// Verify that "result": null is explicitly present for nil result (e.g. shutdown).
	if !strings.Contains(string(gotBytes), `"result":null`) {
		t.Errorf("expected explicit \"result\":null in JSON response, got %s", string(gotBytes))
	}
}
