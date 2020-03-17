package threading

import (
	"sync"

	"gunplan.top/concurrentNet/util"
)

type StealPool interface {
	Launcher
	PoolExecutor
	Status() PoolState
	Init(core int, w int, strategy func(interface{}))
	WaitForStop()
}

type stealPoolImpl struct {
	BasePool
	status         PoolStatusTransfer
	s              func(interface{})
	core           int
	index          util.Sequence
	workQueues     []chan *Task
	controlChannel chan ControlType
	g              sync.WaitGroup
	w              sync.Mutex
}

func (t *stealPoolImpl) Init(core int, w int, strategy func(interface{})) {
	t.BasePool.Init()
	t.core = core
	t.workQueues = make([]chan *Task, t.core)
	for i := range t.workQueues {
		t.workQueues[i] = make(chan *Task, w)
	}
	t.controlChannel = make(chan ControlType, t.core)
	t.s = strategy
	t.index = util.Sequence{Max: core}
	t.status = PoolStatusTransfer{}
	t.BasePool.addQueue = t.addQueue
	t.g.Add(t.core)
}

func (t *stealPoolImpl) Boot() {
	t.status.whenThreadBooting()
	for i := 0; i < t.core; i++ {
		go t.LaunchWork(i)
	}
	t.status.whenThreadBooted()
}

func (t *stealPoolImpl) LaunchWork(i int) {
	for {
		select {
		case task, ok := <-t.workQueues[i]:
			// core execute unit
			if ok {
				task.rev <- task.t(task.param...)
			}
			// control unit
		case op := <-t.controlChannel:
			switch op {

			case SHUTDOWN:
				t.controlChannel <- op
				if len(t.workQueues[i]) != 0 {
					t.consumeRemain(i)
				}
				fallthrough

			case SHUTDOWNNOW:
				t.controlChannel <- op
				fallthrough

			case STOPANY:
				t.g.Done()
				return
			}
		default:
			task, ok := <-t.workQueues[t.index.Next()]
			if ok {
				task.rev <- task.t(task.param...)
			}
		}
	}
}

func (t *stealPoolImpl) consumeRemain(i int) {
	for len(t.workQueues[i]) != 0 {
		// The necessity og locking here is that
		// we have to make sure operator get length
		// and operator consume the channel is an
		// atomic operation.
		t.w.Lock()
		if len(t.workQueues[i]) != 0 {
			task := <-t.workQueues[i]
			t.w.Unlock()
			task.rev <- task.t(task.param...)
		} else {
			t.w.Unlock()
		}
	}
	close(t.workQueues[i])
}

func (t *stealPoolImpl) ShutdownNow() {
	for i := range t.workQueues {
		close(t.workQueues[i])
	}
	t.waitStop(SHUTDOWNNOW)
}

func (t *stealPoolImpl) ShutdownAny() {
	t.controlChannel <- STOPANY
}

func (t *stealPoolImpl) Shutdown() {
	t.waitStop(SHUTDOWN)
}

func (t *stealPoolImpl) WaitForStop() {

}
func (t *stealPoolImpl) waitStop(c ControlType) {
	t.status.whenThreadStopping()
	t.controlChannel <- c
	go func() {
		t.g.Wait()
		close(t.controlChannel)
		t.status.whenThreadStopped()
	}()
}

func (t *stealPoolImpl) addQueue(task *Task) {
	switch t.status.get() {
	case RUNNING:
		t.addQueue0(task)
	case STOPPED:
		fallthrough
	case STOPPING:
		panic("pool has been close")
	}
}

func (t *stealPoolImpl) addQueue0(task *Task) {
	t.workQueues[t.index.Next()] <- task
}
