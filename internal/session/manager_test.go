package session

import (
	"bytes"
	"testing"

	"github.com/ankouros/pterminal/internal/model"
)

func TestBufferOutputDropsAndMarks(t *testing.T) {
	mgr := NewManager(model.AppConfig{})
	hostID := 1

	chunk := bytes.Repeat([]byte("x"), 9*1024)
	for i := 0; i < 2001; i++ {
		mgr.BufferOutput(hostID, chunk)
	}

	chunks := mgr.DrainBuffered(hostID)
	if len(chunks) == 0 {
		t.Fatal("expected buffered chunks")
	}
	if !bytes.Equal(chunks[0], truncatedOutputMsg) {
		t.Fatalf("expected truncated marker first, got %q", string(chunks[0]))
	}
}
