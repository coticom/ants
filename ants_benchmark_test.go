// MIT License

// Copyright (c) 2018 Andy Pan

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package ants

import (
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	RunTimes           = 1e6
	PoolCap            = 5e4
	BenchParam         = 10
	DefaultExpiredTime = 10 * time.Second
)

func demoFunc() {
	time.Sleep(time.Duration(BenchParam) * time.Millisecond)
}

func demoPoolFunc(args interface{}) {
	n := args.(int)
	time.Sleep(time.Duration(n) * time.Millisecond)
}

func demoPanicFunc() {
	rand.Seed(time.Now().UnixNano())
	randomNumber := rand.Intn(10000)
	//simulate a large number of goroutines causing panics.
	if randomNumber%2 == 0 {
		panic("panic")
	}
}

var stopLongRunningFunc int32

func longRunningFunc() {
	for atomic.LoadInt32(&stopLongRunningFunc) == 0 {
		runtime.Gosched()
	}
}

var stopLongRunningPoolFunc int32

func longRunningPoolFunc(arg interface{}) {
	if ch, ok := arg.(chan struct{}); ok {
		<-ch
		return
	}
	for atomic.LoadInt32(&stopLongRunningPoolFunc) == 0 {
		runtime.Gosched()
	}
}

func BenchmarkGoroutines(b *testing.B) {
	var wg sync.WaitGroup
	for i := 0; i < b.N; i++ {
		wg.Add(RunTimes)
		for j := 0; j < RunTimes; j++ {
			go func() {
				demoFunc()
				wg.Done()
			}()
		}
		wg.Wait()
	}
}

func BenchmarkChannel(b *testing.B) {
	var wg sync.WaitGroup
	sema := make(chan struct{}, PoolCap)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wg.Add(RunTimes)
		for j := 0; j < RunTimes; j++ {
			sema <- struct{}{}
			go func() {
				demoFunc()
				<-sema
				wg.Done()
			}()
		}
		wg.Wait()
	}
}

func BenchmarkErrGroup(b *testing.B) {
	var wg sync.WaitGroup
	var pool errgroup.Group
	pool.SetLimit(PoolCap)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wg.Add(RunTimes)
		for j := 0; j < RunTimes; j++ {
			pool.Go(func() error {
				demoFunc()
				wg.Done()
				return nil
			})
		}
		wg.Wait()
	}
}

func BenchmarkAntsPool(b *testing.B) {
	var wg sync.WaitGroup
	p, _ := NewPool(PoolCap, WithExpiryDuration(DefaultExpiredTime))
	defer p.Release()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wg.Add(RunTimes)
		for j := 0; j < RunTimes; j++ {
			_ = p.Submit(func() {
				demoFunc()
				wg.Done()
			})
		}
		wg.Wait()
	}
}

func BenchmarkAntsPoolWithPanic(b *testing.B) {
	var wg sync.WaitGroup
	p, _ := NewPool(PoolCap, WithExpiryDuration(DefaultExpiredTime))
	defer p.Release()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wg.Add(RunTimes)
		for j := 0; j < RunTimes; j++ {
			_ = p.Submit(func() {
				defer func() {
					wg.Done()
				}()
				demoPanicFunc()
			})
		}
		wg.Wait()
	}
}

func BenchmarkAntsMultiPool(b *testing.B) {
	var wg sync.WaitGroup
	p, _ := NewMultiPool(10, PoolCap/10, RoundRobin, WithExpiryDuration(DefaultExpiredTime))
	defer p.ReleaseTimeout(DefaultExpiredTime) //nolint:errcheck

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wg.Add(RunTimes)
		for j := 0; j < RunTimes; j++ {
			_ = p.Submit(func() {
				demoFunc()
				wg.Done()
			})
		}
		wg.Wait()
	}
}

func BenchmarkGoroutinesThroughput(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for j := 0; j < RunTimes; j++ {
			go demoFunc()
		}
	}
}

func BenchmarkSemaphoreThroughput(b *testing.B) {
	sema := make(chan struct{}, PoolCap)
	for i := 0; i < b.N; i++ {
		for j := 0; j < RunTimes; j++ {
			sema <- struct{}{}
			go func() {
				demoFunc()
				<-sema
			}()
		}
	}
}

func BenchmarkAntsPoolThroughput(b *testing.B) {
	p, _ := NewPool(PoolCap, WithExpiryDuration(DefaultExpiredTime))
	defer p.Release()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < RunTimes; j++ {
			_ = p.Submit(demoFunc)
		}
	}
}

func BenchmarkAntsMultiPoolThroughput(b *testing.B) {
	p, _ := NewMultiPool(10, PoolCap/10, RoundRobin, WithExpiryDuration(DefaultExpiredTime))
	defer p.ReleaseTimeout(DefaultExpiredTime) //nolint:errcheck

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < RunTimes; j++ {
			_ = p.Submit(demoFunc)
		}
	}
}

func BenchmarkParallelAntsPoolThroughput(b *testing.B) {
	p, _ := NewPool(PoolCap, WithExpiryDuration(DefaultExpiredTime))
	defer p.Release()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = p.Submit(demoFunc)
		}
	})
}

func BenchmarkParallelAntsMultiPoolThroughput(b *testing.B) {
	p, _ := NewMultiPool(10, PoolCap/10, RoundRobin, WithExpiryDuration(DefaultExpiredTime))
	defer p.ReleaseTimeout(DefaultExpiredTime) //nolint:errcheck

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = p.Submit(demoFunc)
		}
	})
}
