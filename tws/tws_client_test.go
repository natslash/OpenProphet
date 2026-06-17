package tws

import (
	"bytes"
	"encoding/binary"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestSplitFields(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		want    []string
	}{
		{
			name:    "empty payload",
			payload: []byte(""),
			want:    nil,
		},
		{
			name:    "single null",
			payload: []byte("\x00"),
			want:    []string{""},
		},
		{
			name:    "normal fields",
			payload: []byte("1\x002\x003\x00"),
			want:    []string{"1", "2", "3"},
		},
		{
			name:    "trailing empty fields",
			payload: []byte("71\x002\x001\x00\x00\x00"),
			want:    []string{"71", "2", "1", "", ""},
		},
		{
			name:    "no trailing null",
			payload: []byte("1\x002"), // Not standard, but testing behavior
			want:    []string{"1", "2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitFields(tt.payload)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitFields(%q) = %q, want %q", tt.payload, got, tt.want)
			}
		})
	}
}

func TestWriteFrame(t *testing.T) {
	// Create a pipe to intercept the connection output
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	c := NewClient("127.0.0.1", 4002, 1)
	c.conn = client // mock the connection

	payload := []byte("test_payload")
	expectedLen := uint32(len(payload))

	errCh := make(chan error, 1)
	go func() {
		errCh <- c.writeFrame(payload)
	}()

	// Read header
	var hdr [4]byte
	n, err := server.Read(hdr[:])
	if err != nil || n != 4 {
		t.Fatalf("Failed to read header: %v, bytes read: %d", err, n)
	}

	ln := binary.BigEndian.Uint32(hdr[:])
	if ln != expectedLen {
		t.Errorf("Expected length prefix %d, got %d", expectedLen, ln)
	}

	// Read payload
	buf := make([]byte, ln)
	n, err = server.Read(buf)
	if err != nil || uint32(n) != ln {
		t.Fatalf("Failed to read payload: %v, bytes read: %d", err, n)
	}

	if !bytes.Equal(buf, payload) {
		t.Errorf("Expected payload %q, got %q", payload, buf)
	}

	// Ensure writeFrame finished without error
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("writeFrame returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("writeFrame timed out")
	}
}

func TestHandleMessage_AsyncError(t *testing.T) {
	c := NewClient("localhost", 4002, 1)
	
	// Test 1: Not connected -> goes to errCh
	f := []string{"4", "2", "123", "504", "Not connected"}
	c.handleMessage(f)
	
	select {
	case err := <-c.errCh:
		if err == nil || err.Error() != "TWS error (id 123, code 504): Not connected" {
			t.Errorf("Expected specific error in errCh, got %v", err)
		}
	default:
		t.Errorf("Expected error to be routed to errCh")
	}

	// Test 2: Connected -> goes to AsyncErrorCallback
	c.connected = true
	var cbReqID, cbCode int
	var cbMsg string
	cbCalled := make(chan bool, 1)
	
	c.AsyncErrorCallback = func(reqID, code int, msg string) {
		cbReqID = reqID
		cbCode = code
		cbMsg = msg
		cbCalled <- true
	}

	f2 := []string{"4", "2", "124", "10162", "Order rejected"}
	c.handleMessage(f2)

	select {
	case <-cbCalled:
		if cbReqID != 124 || cbCode != 10162 || cbMsg != "Order rejected" {
			t.Errorf("Callback args incorrect: %d, %d, %s", cbReqID, cbCode, cbMsg)
		}
	case <-time.After(time.Second):
		t.Errorf("Expected AsyncErrorCallback to be called")
	}
}
