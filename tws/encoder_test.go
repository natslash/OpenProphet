package tws

import (
	"reflect"
	"testing"
)

type mockFieldWriter struct {
	fields []string
}

func (m *mockFieldWriter) SendFields(fields ...string) error {
	m.fields = fields
	return nil
}

func TestEncoder_ReqCurrentTime(t *testing.T) {
	mockWriter := &mockFieldWriter{}
	encoder := NewEncoder(mockWriter)

	err := encoder.ReqCurrentTime()
	if err != nil {
		t.Fatalf("ReqCurrentTime failed: %v", err)
	}

	expected := []string{"49", "1"}
	if !reflect.DeepEqual(mockWriter.fields, expected) {
		t.Errorf("Encoder sent %q, want %q", mockWriter.fields, expected)
	}
}
