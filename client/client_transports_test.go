package client

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/firecracker-microvm/firecracker-go-sdk/client/operations"
	"github.com/go-openapi/runtime"
	"github.com/stretchr/testify/assert"
)

func TestNewUnixSocketTransport(t *testing.T) {
	done := make(chan bool)

	expectedMessage := "PUT /logger HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"User-Agent: Go-http-client/1.1\r\n" +
		"Content-Length: 0\r\n" +
		"Accept: application/json\r\n" +
		"Content-Type: application/json\r\n" +
		"Accept-Encoding: gzip\r\n" +
		"\r\n"
	socketPath := "testingUnixSocket.sock"
	addr, err := net.ResolveUnixAddr("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.ListenUnix("unix", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		listener.Close()
		os.Remove(socketPath)
	}()

	go func() {
		conn, err := listener.AcceptUnix()
		if err != nil {
			t.Error(err)
		}

		buf := make([]byte, 512)
		nr, err := conn.Read(buf)
		if err != nil {
			return
		}

		data := string(buf[0:nr])
		assert.Equal(t, expectedMessage, data, "expectedMessage received on socket is different than what was sent")
		done <- true
	}()

	unixTransport := NewUnixSocketTransport(socketPath, nil, false)

	unixTransport.Submit(testOperation())

	select {
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Timed out from the listener")
	case <-done:
	}
}

func testOperation() *runtime.ClientOperation {
	return &runtime.ClientOperation{
		ID:                 "putLogger",
		Method:             "PUT",
		PathPattern:        "/logger",
		ProducesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http"},
		Params:             operations.NewPutLoggerParamsWithTimeout(time.Millisecond),
	}
}
