// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

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
