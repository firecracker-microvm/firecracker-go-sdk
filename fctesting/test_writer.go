package fctesting

// TestWriter is used to mock out writing and/or do other things such as
// syncing when to do assertions in the event that a writer is used in a
// goroutine
type TestWriter struct {
	WriteFn func([]byte) (int, error)
}

func (w *TestWriter) Write(b []byte) (int, error) {
	return w.WriteFn(b)
}
