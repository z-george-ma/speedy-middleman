package lib

import (
	"context"
	"fmt"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
)

type AppScopeData struct {
	Log       Logger
	Context   context.Context
	Cancel    context.CancelFunc
	waitGroup sync.WaitGroup
}

var AppScope AppScopeData

// Init will initialise AppScope to capture and start graceful exit on SIGINT and SIGTERM
func (d *AppScopeData) Init(log Logger) {
	appContext, appCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM) // alloc

	d.Log = log
	d.Context = appContext
	d.Cancel = appCancel
}

// Go starts a goroutine for application to wait until complete
func (d *AppScopeData) Go(routine func()) {
	d.waitGroup.Add(1)
	go func() { // alloc
		defer func() { // alloc
			if e := recover(); e != nil {
				if err, ok := e.(error); ok {
					d.Log.Err().Caller(4).Msg(err.Error())
				} else {
					d.Log.Fatal().Msg(fmt.Sprint(e))
				}
			}
			d.Cancel()
			d.waitGroup.Done()
		}()
		routine()
	}()
}

// GoWithClose starts a goroutine and monitors AppScope context.
// Upon cancellation of AppScope context, it runs onClose. If onClose returns true, it forces completion of wait group
func (d *AppScopeData) GoWithClose(routine func(), onClose func() bool) {
	d.waitGroup.Add(1)
	var ended atomic.Bool

	go func() { // alloc
		defer func() { // alloc
			if e := recover(); e != nil {
				if err, ok := e.(error); ok {
					d.Log.Err().Caller(4).Msg(err.Error())
				} else {
					d.Log.Fatal().Msg(fmt.Sprint(e))
				}
			}
			d.Cancel()

			if ended.CompareAndSwap(false, true) {
				d.waitGroup.Done()
			}
		}()
		routine()
	}()

	context.AfterFunc(d.Context, func() {
		if onClose() {
			if ended.CompareAndSwap(false, true) {
				d.waitGroup.Done()
			}
		}
	})
}

// Done waits until all go routines from appscope exit.
//
// cancel: call AppScope.Cancel() when main function exits.
func (d *AppScopeData) Done(cancel bool) {
	if cancel {
		d.Cancel()
	}
	d.waitGroup.Wait()
}
