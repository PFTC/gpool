package gpool

import (
	"errors"
	"sync"
	"time"
)

var (
	// need have a more suitable error name
	ErrMaxWorkerNum = errors.New("worker number excess max")
	ErrStopped      = errors.New("WorkerPool has been stopped")
)

// WorkerPool define a worker pool
type WorkerPool struct {
	sync.Mutex                    //for queue thread-safe
	maxWorkerNumber int           //the max worker number in the pool
	workerNumber    int           //the worker number now in the pool
	workers         []*Worker     //the available worker queue
	maxIdleTime     time.Duration //the recycle time. That means goroutine will be destroyed when it has not been used for maxIdleTime.
	stop            chan struct{} //stop
	wg              sync.WaitGroup
	objectPool      *sync.Pool //gc-friendly
}

//Worker run as a goroutine
type Worker struct {
	fn           chan func()
	lastUsedTime int64
}

// NewLimt creates a worker pool
//
// maxWorkerNum define the max worker number in the pool. When the worker
// number exceeds maxWorkerNum, we will ignore the job.
// recycleTime(minute) define the time to recycle goroutine. When a goroutine has
// not been used for recycleTime, it will be recycled.
func NewLimit(maxWorkerNum, recycleTime int) (*WorkerPool, error) {
	wp := &WorkerPool{
		maxWorkerNumber: maxWorkerNum,
		maxIdleTime:     time.Duration(recycleTime) * time.Minute,
		objectPool: &sync.Pool{
			New: func() interface{} {
				return &Worker{
					fn: make(chan func()),
				}
			},
		},
	}
	go wp.init()
	return wp, nil
}

// NewUnlimit creates a unlimited-number worker pool.
func NewUnlimit(recycleTime int) (*WorkerPool, error) {
	wp := &WorkerPool{
		maxWorkerNumber: -1,
		maxIdleTime:     time.Duration(recycleTime) * time.Minute,
	}

	go wp.init()
	return wp, nil
}

// init initializes the workerpool.
//
// init func will be in charge of cleaning up goroutines and receiving the stop signal.
func (wp *WorkerPool) init() {
	tick := time.Tick(wp.maxIdleTime)
L:
	for {
		select {
		case <-tick:
			wp.cleanup()
		case <-wp.stop:
			wp.wg.Wait()
			break L
		}
	}
}

// cleanup cleans up the available worker queue.
func (wp *WorkerPool) cleanup() {
	i := 0
	now := time.Now().Unix()
	for ; i < len(wp.workers); i++ {
		if time.Duration(now-wp.workers[i].lastUsedTime)*time.Second < wp.maxIdleTime {
			break
		} else {
			close(wp.workers[i].fn)
		}
	}
	wp.Lock()
	wp.workers = wp.workers[i:]
	wp.Unlock()
}

// stopPool stops the worker pool.
func (wp *WorkerPool) StopPool() {
	wp.Lock()
	defer wp.Unlock()
	if _, ok := <-wp.stop; !ok {
		return
	}
	close(wp.stop)
}

// Queue assigns a worker for job (fn func(), with closure we can define every job in this form)
//
// If the worker pool is limited-number and the worker number has reached the limit, we prefer to discard the job.
func (wp *WorkerPool) Queue(fn func()) (err error) {
	select {
	case <-wp.stop:
		return ErrStopped
	default:
	}
	if worker, err := wp.getWorker(); err == nil {
		worker.fn <- fn
	}
	return
}

// GetWorker select a worker.
//
// If the available worker queue is empty, we will new a worker.
// else we will select the last worker, in this case, the worker queue
// is like a FILO queue, and the select algorithm is kind of like LRU.
func (wp *WorkerPool) getWorker() (worker *Worker, err error) {
	if len(wp.workers) == 0 {
		wp.workerNumber++
		if wp.maxWorkerNumber != -1 && wp.workerNumber > wp.maxWorkerNumber {
			err = ErrMaxWorkerNum
			return
		}
		worker = &Worker{
			fn: make(chan func()),
		}
		wp.wg.Add(1)
		go wp.StartWorker(worker)
		return worker
	}

	wp.Lock()
	worker = wp.workers[len(wp.workers)-1]
	wp.workers = wp.workers[:len(wp.workers)-1]
	wp.Unlock()
	return worker
}

// StartWorker starts a new goroutine.
func (wp *WorkerPool) startWorker(worker *Worker) {
	defer wp.wg.Done()
	for {
		select {
		case f, ok := <-worker.fn:
			if !ok {
				return
			}
			if f == nil {
				continue
			}
			f()
			worker.lastUsedTime = time.Now().Unix()
			wp.Lock()
			wp.workers = append(wp.workers, worker)
			wp.Unlock()
		case <-wp.stop:
			return
		}
	}
}
