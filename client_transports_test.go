package firecracker

import (
	"context"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/firecracker-microvm/firecracker-go-sdk/client/operations"
	"github.com/go-openapi/runtime"
	"github.com/stretchr/testify/assert"
)

const expectedEndpointPath = "/test-operation"

func TestNewUnixSocketTransport(t *testing.T) {
	done := make(chan bool)

	socketPath := "testingUnixSocket.sock"
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		panic(err)
	}

	server := http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			assert.Equal(t, expectedEndpointPath, req.URL.String())
			w.WriteHeader(http.StatusOK)
			close(done)
		}),
	}

	go server.Serve(listener)
	defer func() {
		server.Shutdown(context.Background())
		listener.Close()
		os.Remove(socketPath)
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
		PathPattern:        expectedEndpointPath,
		ProducesMediaTypes: []string{"application/json"},
		Schemes:            []string{"http"},
		Params:             operations.NewPutLoggerParams(),
		Reader:             &operations.PutLoggerReader{},
	}
}
