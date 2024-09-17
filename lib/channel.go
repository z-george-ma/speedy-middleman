package lib

import (
	"context"
	"errors"
	"reflect"
)

var END = errors.New("END")

type Channel[T any] chan T

// Send message to a new cancellable channel.
//
// Examples:
//
//	var ch lib.Channel[int] = make(chan int, 10)
//	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
//	defer cancel()
//	err := ch.Send(10, ctx)
func (c Channel[T]) Send(value T, ctx ...context.Context) (err error) {
	if len(ctx) == 0 {
		c <- value
		return
	}

	select {
	case c <- value:
		return
	case <-ctx[0].Done():
		return ctx[0].Err()
	}
}

// Get next item from a channel
//
// Examples:
//
//	var ch lib.Channel[int] = make(chan int, 10)
//	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
//	defer cancel()
//	ret, err := ch.Next(ctx)
func (c Channel[T]) Next(ctx ...context.Context) (ret T, err error) {
	var ok bool

	if len(ctx) == 0 {
		ret, ok = <-c
		if !ok {
			err = END
		}
		return
	}

	select {
	case ret, ok = <-c:
	case <-ctx[0].Done():
		err = ctx[0].Err()
	}

	return
}

type MultiChannel[T any, TContext any] struct {
	newChannel chan []ChannelWithContext[T, TContext]
	callback   func(bool, T, TContext)
}

type ChannelWithContext[T any, TContext any] struct {
	Channel <-chan T
	Context TContext
}

type ChannelValue[T any, TContext any] struct {
	Ok      bool
	Result  T
	Context TContext
}

func NewMultiChannel[T any, TContext any](callback func(bool, T, TContext), incomingSize int) *MultiChannel[T, TContext] {
	mc := &MultiChannel[T, TContext]{
		newChannel: make(chan []ChannelWithContext[T, TContext], incomingSize),
		callback:   callback,
	}

	go mc.recvLoop()
	return mc
}

func (mc *MultiChannel[T, TContext]) Add(ch ...ChannelWithContext[T, TContext]) {
	mc.newChannel <- ch
}

func (mc *MultiChannel[T, TContext]) AddSingle(ch <-chan T, context TContext) {
	mc.newChannel <- []ChannelWithContext[T, TContext]{
		{
			Channel: ch,
			Context: context,
		},
	}
}

func (mc *MultiChannel[T, TContext]) Close() {
	close(mc.newChannel)
}

func (mc *MultiChannel[T, TContext]) recvLoop() {
	cases := make([]reflect.SelectCase, 1, 4096)
	context := make([]TContext, 0, 4096)
	vacant := make([]int, 0, 4096)

	cases[0] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(mc.newChannel)}

	for {
		chosen, value, ok := reflect.Select(cases)

		if chosen == 0 {
			if !ok {
				return
			}

			// add incoming channels to select case
			chs := value.Interface().([]ChannelWithContext[T, TContext])
			added := 0
			l := len(vacant)

			for i := l - 1; i >= 0; i-- {
				cases[vacant[i]] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(chs[added].Channel)}
				context[vacant[i]-1] = chs[added].Context

				added++

				if added == len(chs) {
					break
				}
			}

			vacant = vacant[:l-added]

			chs = chs[added:]

			for _, c := range chs {
				cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(c.Channel)})
				context = append(context, c.Context)
			}
		} else {
			// one of the selected channels returns
			cv := ChannelValue[T, TContext]{
				Ok:      ok,
				Context: context[chosen-1],
			}

			if ok {
				cv.Result = value.Interface().(T)
			}

			cases[chosen].Chan = reflect.ValueOf(nil)

			vacant = append(vacant, chosen)
			mc.callback(ok, cv.Result, cv.Context)
		}
	}
}
