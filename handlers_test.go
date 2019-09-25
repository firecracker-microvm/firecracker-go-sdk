package firecracker

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	ops "github.com/firecracker-microvm/firecracker-go-sdk/client/operations"
	"github.com/firecracker-microvm/firecracker-go-sdk/fctesting"
)

func TestHandlerListAppend(t *testing.T) {
	h := HandlerList{}
	h.Append(Handler{Name: "foo"})

	if size := h.Len(); size != 0 {
		t.Errorf("expected length to be '0', but received '%d'", size)
	}

	expectedNames := []string{
		"foo",
		"bar",
		"baz",
	}

	for _, name := range expectedNames {
		h = h.Append(Handler{Name: name})
	}

	for i, name := range expectedNames {
		if e, a := name, h.list[i].Name; e != a {
			t.Errorf("expected %q, but received %q", e, a)
		}
	}
}

func TestHandlerListPrepend(t *testing.T) {
	h := HandlerList{}
	h.Prepend(Handler{Name: "foo"})

	if size := h.Len(); size != 0 {
		t.Errorf("expected length to be '0', but received '%d'", size)
	}

	expectedNames := []string{
		"foo",
		"bar",
		"baz",
	}

	for _, name := range expectedNames {
		h = h.Prepend(Handler{Name: name})
	}

	for i, name := range expectedNames {
		if e, a := name, h.list[len(h.list)-i-1].Name; e != a {
			t.Errorf("expected %q, but received %q", e, a)
		}
	}
}

func TestHandlerListRemove(t *testing.T) {
	h := HandlerList{}.Append(
		Handler{
			Name: "foo",
		},
		Handler{
			Name: "bar",
		},
		Handler{
			Name: "baz",
		},
		Handler{
			Name: "foo",
		},
		Handler{
			Name: "baz",
		},
	)

	h.Remove("foo")

	if e, a := 5, h.Len(); e != a {
		t.Errorf("expected '%d', but received '%d'", e, a)
	}

	h = h.Remove("foo")
	if e, a := 3, h.Len(); e != a {
		t.Errorf("expected '%d', but received '%d'", e, a)
	}

	if e, a := "bar", h.list[0].Name; e != a {
		t.Errorf("expected %s, but received %s", e, a)
	}

	h = h.Remove("invalid-name")
	if e, a := 3, h.Len(); e != a {
		t.Errorf("expected '%d', but received '%d'", e, a)
	}

	h = h.Remove("baz")
	if e, a := 1, h.Len(); e != a {
		t.Errorf("expected '%d', but received '%d'", e, a)
	}

	h = h.Remove("bar")
	if e, a := 0, h.Len(); e != a {
		t.Errorf("expected '%d', but received '%d'", e, a)
	}
}

func TestHandlerListClear(t *testing.T) {
	h := HandlerList{}
	h = h.Append(
		Handler{Name: "foo"},
		Handler{Name: "foo"},
		Handler{Name: "foo"},
		Handler{Name: "foo"},
		Handler{Name: "foo"},
		Handler{Name: "foo"},
		Handler{Name: "foo"},
	)

	h.Clear()
	if e, a := 7, h.Len(); e != a {
		t.Errorf("expected '%d', but received '%d'", e, a)
	}

	h = h.Clear()
	if e, a := 0, h.Len(); e != a {
		t.Errorf("expected '%d', but received '%d'", e, a)
	}
}

func TestHandlerListRun(t *testing.T) {
	count := 0
	bazErr := fmt.Errorf("baz error")

	h := HandlerList{}
	h = h.Append(
		Handler{
			Name: "foo",
			Fn: func(ctx context.Context, m *Machine) error {
				count++
				return nil
			},
		},
		Handler{
			Name: "bar",
			Fn: func(ctx context.Context, m *Machine) error {
				count += 10
				return nil
			},
		},
		Handler{
			Name: "baz",
			Fn: func(ctx context.Context, m *Machine) error {
				return bazErr
			},
		},
		Handler{
			Name: "qux",
			Fn: func(ctx context.Context, m *Machine) error {
				count *= 100
				return nil
			},
		},
	)

	ctx := context.Background()
	m := &Machine{
		logger: fctesting.NewLogEntry(t),
	}
	if err := h.Run(ctx, m); err != bazErr {
		t.Errorf("expected an error, but received %v", err)
	}

	if e, a := 11, count; e != a {
		t.Errorf("expected '%d', but received '%d'", e, a)
	}

	h = h.Remove("baz")
	if err := h.Run(ctx, m); err != nil {
		t.Errorf("expected no error, but received %v", err)
	}

	if e, a := 2200, count; e != a {
		t.Errorf("expected '%d', but received '%d'", e, a)
	}
}

func TestHandlerListHas(t *testing.T) {
	cases := []struct {
		name     string
		elemName string
		list     HandlerList
		expected bool
	}{
		{
			name:     "contains",
			elemName: "foo",
			list: HandlerList{}.Append(
				Handler{
					Name: "foo",
				},
			),
			expected: true,
		},
		{
			name:     "does not contain",
			elemName: "foo",
			list:     HandlerList{},
		},
		{
			name:     "similar names",
			elemName: "foo",
			list: HandlerList{}.Append(
				Handler{
					Name: "foo1",
				},
			),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if e, a := c.expected, c.list.Has(c.elemName); e != a {
				t.Errorf("expected %t, but received %t", e, a)
			}
		})
	}
}

func TestHandlerListSwappend(t *testing.T) {
	fn := func(ctx context.Context, m *Machine) error {
		return nil
	}

	cases := []struct {
		name         string
		list         HandlerList
		elem         Handler
		expectedList HandlerList
	}{
		{
			name: "append one",
			list: HandlerList{}.Append(
				Handler{
					Name: "bar",
				},
			),
			elem: Handler{
				Name: "foo",
			},
			expectedList: HandlerList{}.Append(
				Handler{
					Name: "bar",
				},
				Handler{
					Name: "foo",
				},
			),
		},
		{
			name: "swap single",
			list: HandlerList{}.Append(
				Handler{
					Name: "bar",
				},
				Handler{
					Name: "foo",
				},
			),
			elem: Handler{
				Name: "foo",
				Fn:   fn,
			},
			expectedList: HandlerList{}.Append(
				Handler{
					Name: "bar",
				},
				Handler{
					Name: "foo",
					Fn:   fn,
				},
			),
		},
		{
			name: "swap multiple",
			list: HandlerList{}.Append(
				Handler{
					Name: "foo",
				},
				Handler{
					Name: "bar",
				},
				Handler{
					Name: "foo",
				},
			),
			elem: Handler{
				Name: "foo",
				Fn:   fn,
			},
			expectedList: HandlerList{}.Append(
				Handler{
					Name: "foo",
					Fn:   fn,
				},
				Handler{
					Name: "bar",
				},
				Handler{
					Name: "foo",
					Fn:   fn,
				},
			),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.list = c.list.Swappend(c.elem)

			if e, a := c.expectedList, c.list; !compareHandlerLists(e, a) {
				t.Errorf("expected %v, but received %v", e, a)
			}
		})
	}
}

func TestHandlerListReplace(t *testing.T) {
	fn := func(ctx context.Context, m *Machine) error {
		return nil
	}

	cases := []struct {
		name         string
		list         HandlerList
		elem         Handler
		expectedList HandlerList
	}{
		{
			name: "swap none",
			list: HandlerList{}.Append(
				Handler{
					Name: "bar",
				},
			),
			elem: Handler{
				Name: "foo",
			},
			expectedList: HandlerList{}.Append(
				Handler{
					Name: "bar",
				},
			),
		},
		{
			name: "swap single",
			list: HandlerList{}.Append(
				Handler{
					Name: "bar",
				},
				Handler{
					Name: "foo",
				},
			),
			elem: Handler{
				Name: "foo",
				Fn:   fn,
			},
			expectedList: HandlerList{}.Append(
				Handler{
					Name: "bar",
				},
				Handler{
					Name: "foo",
					Fn:   fn,
				},
			),
		},
		{
			name: "swap multiple",
			list: HandlerList{}.Append(
				Handler{
					Name: "foo",
				},
				Handler{
					Name: "bar",
				},
				Handler{
					Name: "foo",
				},
			),
			elem: Handler{
				Name: "foo",
				Fn:   fn,
			},
			expectedList: HandlerList{}.Append(
				Handler{
					Name: "foo",
					Fn:   fn,
				},
				Handler{
					Name: "bar",
				},
				Handler{
					Name: "foo",
					Fn:   fn,
				},
			),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.list = c.list.Swap(c.elem)

			if e, a := c.expectedList, c.list; !compareHandlerLists(e, a) {
				t.Errorf("expected %v, but received %v", e, a)
			}
		})
	}
}

func TestHandlerListAppendAfter(t *testing.T) {
	cases := []struct {
		name         string
		list         HandlerList
		afterName    string
		elem         Handler
		expectedList HandlerList
	}{
		{
			name: "no append",
			list: HandlerList{}.Append(
				Handler{
					Name: "foo",
				},
				Handler{
					Name: "bar",
				},
				Handler{
					Name: "baz",
				},
			),
			afterName: "not exist",
			elem: Handler{
				Name: "qux",
			},
			expectedList: HandlerList{}.Append(
				Handler{
					Name: "foo",
				},
				Handler{
					Name: "bar",
				},
				Handler{
					Name: "baz",
				},
			),
		},
		{
			name: "append after",
			list: HandlerList{}.Append(
				Handler{
					Name: "foo",
				},
				Handler{
					Name: "bar",
				},
				Handler{
					Name: "baz",
				},
			),
			afterName: "foo",
			elem: Handler{
				Name: "qux",
			},
			expectedList: HandlerList{}.Append(
				Handler{
					Name: "foo",
				},
				Handler{
					Name: "qux",
				},
				Handler{
					Name: "bar",
				},
				Handler{
					Name: "baz",
				},
			),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.list = c.list.AppendAfter(c.afterName, c.elem)
			if e, a := c.expectedList, c.list; !compareHandlerLists(e, a) {
				t.Errorf("expected %v, but received %v", e, a)
			}
		})
	}
}

func TestHandlers(t *testing.T) {
	called := ""
	metadata := map[string]string{
		"foo": "bar",
		"baz": "qux",
	}

	cases := []struct {
		Handler Handler
		Client  fctesting.MockClient
		Config  Config
	}{
		{
			Handler: BootstrapLoggingHandler,
			Client: fctesting.MockClient{
				PutLoggerFn: func(params *ops.PutLoggerParams) (*ops.PutLoggerNoContent, error) {
					called = BootstrapLoggingHandler.Name
					return nil, nil
				},
			},
			Config: Config{
				LogLevel:    "Debug",
				LogFifo:     filepath.Join(testDataPath, "firecracker.log"),
				MetricsFifo: filepath.Join(testDataPath, "firecracker-metrics"),
			},
		},
		{
			Handler: CreateMachineHandler,
			Client: fctesting.MockClient{
				PutMachineConfigurationFn: func(params *ops.PutMachineConfigurationParams) (*ops.PutMachineConfigurationNoContent, error) {
					called = CreateMachineHandler.Name
					return &ops.PutMachineConfigurationNoContent{}, nil
				},
				GetMachineConfigurationFn: func(params *ops.GetMachineConfigurationParams) (*ops.GetMachineConfigurationOK, error) {
					return &ops.GetMachineConfigurationOK{
						Payload: &models.MachineConfiguration{},
					}, nil
				},
			},
			Config: Config{},
		},
		{
			Handler: CreateBootSourceHandler,
			Client: fctesting.MockClient{
				PutGuestBootSourceFn: func(params *ops.PutGuestBootSourceParams) (*ops.PutGuestBootSourceNoContent, error) {
					called = CreateBootSourceHandler.Name
					return &ops.PutGuestBootSourceNoContent{}, nil
				},
			},
			Config: Config{},
		},
		{
			Handler: AttachDrivesHandler,
			Client: fctesting.MockClient{
				PutGuestDriveByIDFn: func(params *ops.PutGuestDriveByIDParams) (*ops.PutGuestDriveByIDNoContent, error) {
					called = AttachDrivesHandler.Name
					return &ops.PutGuestDriveByIDNoContent{}, nil
				},
			},
			Config: Config{
				Drives: NewDrivesBuilder("/foo/bar").Build(),
			},
		},
		{
			Handler: CreateNetworkInterfacesHandler,
			Client: fctesting.MockClient{
				PutGuestNetworkInterfaceByIDFn: func(params *ops.PutGuestNetworkInterfaceByIDParams) (*ops.PutGuestNetworkInterfaceByIDNoContent, error) {
					called = CreateNetworkInterfacesHandler.Name
					return &ops.PutGuestNetworkInterfaceByIDNoContent{}, nil
				},
			},
			Config: Config{
				NetworkInterfaces: []NetworkInterface{{
					StaticConfiguration: &StaticNetworkConfiguration{
						MacAddress:  "macaddress",
						HostDevName: "host",
					},
				}},
			},
		},
		{
			Handler: AddVsocksHandler,
			Client: fctesting.MockClient{
				PutGuestVsockByIDFn: func(params *ops.PutGuestVsockByIDParams) (*ops.PutGuestVsockByIDNoContent, error) {
					called = AddVsocksHandler.Name
					return &ops.PutGuestVsockByIDNoContent{}, nil
				},
			},
			Config: Config{
				VsockDevices: []VsockDevice{
					{
						Path: "path",
						CID:  123,
					},
				},
			},
		},
		{
			Handler: NewSetMetadataHandler(metadata),
			Client: fctesting.MockClient{
				PutMmdsFn: func(params *ops.PutMmdsParams) (*ops.PutMmdsNoContent, error) {
					called = SetMetadataHandlerName
					if !reflect.DeepEqual(metadata, params.Body) {
						return nil, fmt.Errorf("incorrect metadata value: %v", params.Body)
					}
					return &ops.PutMmdsNoContent{}, nil
				},
			},
			Config: Config{},
		},
	}

	ctx := context.Background()
	socketpath := filepath.Join(testDataPath, "socket")
	cfg := Config{}

	defer func() {
		os.Remove(cfg.SocketPath)
		os.Remove(cfg.LogFifo)
		os.Remove(cfg.MetricsFifo)
	}()

	for _, c := range cases {
		t.Run(c.Handler.Name, func(t *testing.T) {
			// cache in case test exited early and can be cleaned up later
			cfg = c.Config
			// resetting called for the next test
			called = ""

			client := NewClient(socketpath, fctesting.NewLogEntry(t), true, WithOpsClient(&c.Client))
			m, err := NewMachine(ctx, c.Config, WithClient(client), WithLogger(fctesting.NewLogEntry(t)))
			if err != nil {
				t.Fatalf("failed to create machine: %v", err)
			}

			if err := c.Handler.Fn(ctx, m); err != nil {
				t.Errorf("failed to call handler function: %v", err)
			}

			if e, a := c.Handler.Name, called; e != a {
				t.Errorf("expected %v, but received %v", e, a)
			}

			// clean up any created resources
			os.Remove(c.Config.SocketPath)
			os.Remove(c.Config.LogFifo)
			os.Remove(c.Config.MetricsFifo)
		})
	}
}

func TestCreateLogFilesHandler(t *testing.T) {
	logWriterBuf := new(bytes.Buffer)
	config := Config{
		LogFifo:       filepath.Join(testDataPath, "firecracker-log.fifo"),
		MetricsFifo:   filepath.Join(testDataPath, "firecracker-metrics.fifo"),
		FifoLogWriter: logWriterBuf,
	}

	defer func() {
		os.Remove(config.LogFifo)
		os.Remove(config.MetricsFifo)
	}()

	ctx := context.Background()
	m, err := NewMachine(ctx, config, WithLogger(fctesting.NewLogEntry(t)))
	if err != nil {
		t.Fatalf("failed to create machine: %v", err)
	}

	// spin off goroutine to write to Log fifo so we don't block
	go func() {
		timer := time.NewTimer(1 * time.Second)
		for {
			select {
			case <-timer.C:
				t.Error("timed out waiting for log fifo")
			default:
				fifoPipe, err := os.OpenFile(config.LogFifo, os.O_WRONLY, os.ModePerm)
				if err != nil {
					time.Sleep(10 * time.Millisecond)
					continue
				}

				timer.Stop()
				_, err = fifoPipe.WriteString("data")
				if err != nil {
					t.Errorf("Failed to write to fifo %v", err)
				}
				return
			}
		}
	}()

	err = CreateLogFilesHandler.Fn(ctx, m)
	if err != nil {
		t.Errorf("failed to call CreateLogFilesHandler function: %v", err)
	}
}

func compareHandlerLists(l1, l2 HandlerList) bool {
	if l1.Len() != l2.Len() {
		return false
	}

	for i := 0; i < len(l1.list); i++ {
		e1, e2 := l1.list[i], l2.list[i]

		if e1.Name != e2.Name {
			return false
		}

		v1 := reflect.ValueOf(e1.Fn)
		v2 := reflect.ValueOf(e2.Fn)
		if v1.Pointer() != v2.Pointer() {
			return false
		}
	}

	return true
}
