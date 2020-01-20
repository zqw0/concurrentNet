package threading

import (
	"sync"
	"time"
)

type FutureImpl struct {
	wait *chan interface{}
}

func (f *FutureImpl) get() interface{} {
	return <-*f.wait
}

func (f *FutureImpl) isDone() bool {
	return len(*f.wait) == 1
}

func newFuture(c *chan interface{}) Future {
	return &FutureImpl{wait: c}
}

func NewThreadPool(core, ext int, span time.Duration, w uint64, strategy func(interface{})) ThreadPool {
	tp := threadPoolImpl{}
	tp.Init(core, ext, span, w, strategy)
	tp.Boot()
	return &tp
}

type ThreadPool interface {
	Launcher
	PoolExecutor
	Status() PoolState
	Init(core, ext int, d time.Duration, w uint64, strategy func(interface{}))
	WaitForStop()
}

type threadPoolImpl struct {
	status         PoolState
	s              func(interface{})
	core           int
	workQueue      chan *Task
	controlChannel chan ControlType
	g              sync.WaitGroup
}

func (t *threadPoolImpl) Init(core, ext int, span time.Duration, w uint64, strategy func(interface{})) {
	t.core = core
	t.workQueue = make(chan *Task, 1000)
	t.controlChannel = make(chan ControlType, t.core)
	t.s = strategy
	t.g.Add(t.core)
}
func (t *threadPoolImpl) Boot() {
	t.status = RUNNING
	for i := 0; i < t.core; i++ {
		go t.LaunchWork()
	}

}

func (t *threadPoolImpl) LaunchWork() {
	for {
		select {
		case task := <-t.workQueue:
			task.rev <- task.t(task.param...)
		case op := <-t.controlChannel:
			switch op {
			case STOPALL:
				t.g.Done()
				t.controlChannel <- op
				return
			case STOPANY:
				t.g.Done()
			case SHUTDOWN:
				t.controlChannel <- op
				if len(t.workQueue) == 0 {
					t.g.Done()
					return
				} else {
					// todo
				}
			}
		}
	}
}

func (t *threadPoolImpl) LaunchWorkExt() {
	t.g.Add(1)
	t.LaunchWork()
}

func (t *threadPoolImpl) Status() PoolState {
	return t.status
}

func (t *threadPoolImpl) WaitForStop() {
	t.g.Wait()
}

func (t *threadPoolImpl) Shutdown() {
	t.status = STOPPING
	t.controlChannel <- SHUTDOWN
}

func (t *threadPoolImpl) ShutdownNow() {
	t.status = STOPPING
	t.controlChannel <- STOPALL
	go func() {
		t.g.Wait()
		t.status = STOPPED
	}()
}

func (t *threadPoolImpl) addQueue(task *Task) {
	switch t.status {
	case RUNNING:
		t.workQueue <- task
	case STOPPING:
		panic("pool has been close")
	}
}

func (t *threadPoolImpl) Exec(f func()) {
	t.addQueue(&Task{param: nil, t: func(i ...interface{}) interface{} {
		f()
		return nil
	}})
}

func (t *threadPoolImpl) Execwr(f func() interface{}) Future {
	tsk := Task{param: nil, t: func(i ...interface{}) interface{} {
		return f()
	}}
	tsk.init()
	t.addQueue(&tsk)
	return newFuture(&tsk.rev)
}

func (t *threadPoolImpl) Execwp(f func(...interface{}), p ...interface{}) {
	t.addQueue(&Task{param: p, t: func(i ...interface{}) interface{} {
		f(i...)
		return nil
	}})
}

func (t *threadPoolImpl) Execwpr(f func(...interface{}) interface{}, p ...interface{}) Future {
	tsk := Task{param: p, t: f}
	tsk.init()
	t.addQueue(&tsk)
	return newFuture(&tsk.rev)
}
