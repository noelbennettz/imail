package async

import (
	"runtime/debug"
	"sync"

	"github.com/noelbennettz/imail/utils/context"
	log "github.com/sirupsen/logrus"
)

type Callback func(params any)

type Async struct {
	wg  *sync.WaitGroup
	ctx *context.Context
}

func New(ctx *context.Context) *Async {
	return &Async{
		ctx: ctx,
	}
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
				as.HandleErrRecover(err)
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

// HandleErrRecover logs a panic that escaped a callback.
func (as *Async) HandleErrRecover(err interface{}) {
	log.WithContext(as.ctx).Errorf("goroutine panic:%s  \n %s", err, string(debug.Stack()))
}
