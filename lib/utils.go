package lib

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"
	"unsafe"
)

type M = map[string]any

// Wraps allows dynamically created wrapper around a given function
// Note: this uses reflect.MakeFunc under the hood and is an expensive operation
//
// Examples:
//
//	add := func(a int, b int) int { return a + b }
//
//	lib.Wraps(add, func(inner reflect.Value, params []reflect.Value) []reflect.Value {
//		println("Invocation start")
//		ret := inner.Call([]reflect.Value{reflect.ValueOf(4), reflect.ValueOf(5)})
//		return ret
//	})
func Wraps[T any](f T, hook func(reflect.Value, []reflect.Value) []reflect.Value) T {

	fn := reflect.ValueOf(f)

	inner := func(params []reflect.Value) []reflect.Value {
		return hook(fn, params)
	}

	ret := reflect.MakeFunc(fn.Type(), inner)

	v, _ := ret.Interface().(T)

	return v
}

// Transforms replaces given function to a different signature
// Note: this uses reflect.MakeFunc under the hood and is an expensive operation
//
// Examples:
//
//	add := func(a int, b int) int { return a + b }
//	lib.Transforms[func(int, int) int, func(int, int) float64](add, func(inner reflect.Value, params []reflect.Value) []reflect.Value {
//		println("Invocation start")
//		ret := inner.Call([]reflect.Value{reflect.ValueOf(4), reflect.ValueOf(5)})
//		ret[0] = reflect.ValueOf(4.2)
//		return ret
//	})
func Transforms[T any, TRet any](f T, hook func(reflect.Value, []reflect.Value) []reflect.Value) TRet {
	fn := reflect.ValueOf(f)
	var targetFn TRet

	inner := func(params []reflect.Value) []reflect.Value {
		return hook(fn, params)
	}

	ret := reflect.MakeFunc(reflect.ValueOf(targetFn).Type(), inner)

	v, _ := ret.Interface().(TRet)

	return v
}

// Timeout creates a context that times out after given seconds.
// Internally it creates a goroutine to cancel after timeout, so no need to `defer cancel()`
// By default it uses AppContext from AppScope.
//
// Note: don't use it for high concurrency scenario.
//
// Examples:
//
//	ctx := lib.Timeout(10, context.Background())
func Timeout(seconds int, ctx ...context.Context) context.Context {
	parent := AppScope.Context
	if len(ctx) > 0 {
		parent = ctx[0]
	}
	ret, cancel := context.WithTimeout(parent, time.Duration(seconds)*time.Second)

	// Warning: this will not fit for high concurrency scenario.
	// You either will need to write `defer cancel()` in the code,
	// or have a background GC routine to cancel.
	go func() { // alloc
		<-ret.Done()
		cancel()
	}()

	return ret
}

// Assert err is nil
func Assert(err error) {
	if err != nil {
		if AppScope.Log != nil {
			AppScope.Log.Err().Caller(1).Msg(err.Error())
		}
		panic(err)
	}
}

func Must[T any](ret T, err error) T {
	if err != nil {
		if AppScope.Log != nil {
			AppScope.Log.Err().Caller(1).Msg(err.Error())
		}
		panic(err)
	}

	return ret
}

// IsDone returns true if ctx is complete
func IsDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func BytesToString(bs []byte) string {
	return *(*string)(unsafe.Pointer(&bs))
}

func CopyInto(buf *[]byte, b []byte) []byte {
	lenb := len(b)

	var rawBuf []byte
	if *buf == nil {
		rawBuf = make([]byte, lenb)
		*buf = rawBuf
	} else {
		rawBuf = *buf
	}

	n := copy(rawBuf, b)
	if n < lenb {
		rawBuf = append(rawBuf, b[n:lenb]...)
		*buf = rawBuf
	}
	return rawBuf[:lenb]
}

func StringToBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

func Contains[T comparable](arr []T, item T) bool {
	for _, i := range arr {
		if item == i {
			return true
		}
	}
	return false
}

func BuildString(builder *strings.Builder, args ...string) {
	for _, arg := range args {
		builder.WriteString(arg)
	}
}

func ReadAll(r io.Reader, buf *[]byte) error {
	b := (*buf)[:0]
	for {
		n, err := r.Read(b[len(b):cap(b)])
		b = b[:len(b)+n]
		if err != nil {
			*buf = b
			if err == io.EOF {
				err = nil
			}
			return err
		}

		if len(b) == cap(b) {
			// Add more capacity (let append pick how much).
			b = append(b, 0)[:len(b)]
			*buf = b
		}
	}
}
func WriteAll(w io.Writer, args ...[]byte) error {
	for _, arg := range args {
		startPos := 0

		for startPos < len(arg) {
			n, err := w.Write(arg[startPos:])
			if err != nil {
				return err
			}

			startPos += n
		}
	}
	return nil
}

func Retry(n int, f func() error) (err error) {
	for i := 0; i < n; i++ {
		err = f()
		if err == nil {
			return
		}
	}
	return
}

func LogUnhandledException(log Logger, skip int) {
	if e := recover(); e != nil {
		if err, ok := e.(error); ok {
			log.Err().Error(err, skip)
		} else {
			log.Fatal().Msg(fmt.Sprint(e))
		}
	}
}

func CloseWithTimeout(close func() error, timeout time.Duration) error {
	hasClosed := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	go func() {
		defer recover()
		hasClosed <- close()
	}()

	select {
	case <-ctx.Done():
		return context.Canceled
	case e := <-hasClosed:
		return e
	}
}
