package firecracker

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	log "github.com/sirupsen/logrus"
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
		logger: log.NewEntry(log.New()),
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
