package assert

import (
	"reflect"
	"testing"
)

func True[T comparable](t *testing.T, value bool) {
	t.Helper()

	if !value {
		t.Errorf("Assertion failed")
	}
}

func Equal[T comparable](t *testing.T, expected T, actual T) {
	t.Helper()

	if actual != expected {
		t.Errorf("want: %v; got: %v", expected, actual)
	}
}

func Null(t *testing.T, value any) {
	t.Helper()

	isNil := value == nil

	if !isNil {
		switch reflect.TypeOf(value).Kind() {
		case reflect.Ptr, reflect.Map, reflect.Array, reflect.Chan, reflect.Slice, reflect.UnsafePointer:
			isNil = reflect.ValueOf(value).IsNil()
		}
	}

	if !isNil {
		t.Errorf("expect %v to be null", value)
	}
}

func NotNull(t *testing.T, value any) {
	t.Helper()

	isNil := value == nil

	if !isNil {
		switch reflect.TypeOf(value).Kind() {
		case reflect.Ptr, reflect.Map, reflect.Array, reflect.Chan, reflect.Slice, reflect.UnsafePointer:
			isNil = reflect.ValueOf(value).IsNil()
		}
	}

	if isNil {
		t.Errorf("expect %v to be not null", value)
	}
}
