package client

import (
	"github.com/firecracker-microvm/firecracker-go-sdk/client/operations"
	"github.com/go-openapi/runtime"
	"github.com/stretchr/testify/assert"
	"net"
	"testing"
	"time"
)

func TestNewUnixSocketTransport(t *testing.T) {
	done := make(chan bool)

	expectedMessage := "PUT /logger HTTP/1.1\r\nHost: localhost\r\nUser-Agent: Go-http-client/1.1\r\nContent-Length: 0\r\nAccept: application/json\r\nContent-Type: application/json\r\nAccept-Encoding: gzip\r\n\r\n"
	socketPath := "testingUnixSocket.sock"
	addr, _ := net.ResolveUnixAddr("unix", socketPath)
	listener, _ := net.ListenUnix("unix", addr)
	defer listener.Close()

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
		Params:             operations.NewPutLoggerParams(),
	}
}
