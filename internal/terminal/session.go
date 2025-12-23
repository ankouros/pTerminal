package terminal

// Session is a binary-safe terminal stream.
// Implementations include SSH-backed sessions and local PTY process sessions (e.g. telecom).
type Session interface {
	Write(p []byte) error
	Resize(cols, rows int) error
	Close() error

	Output() <-chan []byte
	Done() <-chan struct{}
}
