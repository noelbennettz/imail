package async

import (
	"errors"
	"runtime/debug"
	"sync"

	"github.com/noelbennettz/imail/utils/context"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"
)

type Callback func(params any)

type Async struct {
	wg        *sync.WaitGroup
	lastError error
	ctx       *context.Context
}

func New(ctx *context.Context) *Async {
	return &Async{
		ctx: ctx,
	}
}

func (as *Async) LastError() error {
	return as.lastError
}

func (as *Async) WaitProcess(callback Callback, params any) {
	if as.wg == nil {
		as.wg = &sync.WaitGroup{}
	}
	as.wg.Add(1)
	as.Process(func(params any) {
		defer as.wg.Done()
		callback(params)
	}, params)
}

func (as *Async) Process(callback Callback, params any) {
	go func() {
		defer func() {
			if err := recover(); err != nil {
				as.lastError = as.HandleErrRecover(err)
			}
		}()
		callback(params)
	}()
}

func (as *Async) Wait() {
	if as.wg == nil {
		return
	}
	as.wg.Wait()
}

// HandleErrRecover recovers from a panic
func (as *Async) HandleErrRecover(err interface{}) (returnErr error) {
	switch err.(type) {
	case error:
		returnErr = err.(error)
	default:
		returnErr = errors.New(cast.ToString(err))
	}

	log.WithContext(as.ctx).Errorf("goroutine panic:%s  \n %s", err, string(debug.Stack()))

	return
}
