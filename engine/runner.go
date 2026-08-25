package main

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/andreacoppini/crossbreeder/engine/ap"
)

// Runner fans a host list out over a bounded pool of workers. This is the whole
// answer to "it only does one AP at a time": the per-AP work is pure network
// wait, so N in flight costs N sockets and a few kilobytes of stack, not N CPUs.
type Runner struct {
	Concurrency int
	Config      ap.Config
	// OnResult is called once per finished AP, from a worker goroutine, in
	// completion order. It must be safe to call concurrently.
	OnResult func(index int, r ap.Result)
}

// Run blocks until every host has a Result. Results come back in host order.
func (rn *Runner) Run(ctx context.Context, hosts []string) []ap.Result {
	n := rn.Concurrency
	if n < 1 {
		n = 1
	}
	if n > len(hosts) {
		n = len(hosts)
	}

	results := make([]ap.Result, len(hosts))
	var next atomic.Int64
	var wg sync.WaitGroup

	for w := 0; w < n; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1)) - 1
				if i >= len(hosts) {
					return
				}
				if ctx.Err() != nil {
					results[i] = ap.Result{IP: hosts[i], Status: "Cancelled"}
					continue
				}
				// Each worker writes only to its own slot, so no lock is needed
				// on results itself.
				results[i] = ap.Run(ctx, hosts[i], rn.Config)
				if rn.OnResult != nil {
					rn.OnResult(i, results[i])
				}
			}
		}()
	}
	wg.Wait()
	return results
}
