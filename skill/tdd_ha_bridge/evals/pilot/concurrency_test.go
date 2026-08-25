package pilot

import (
	"sync"
	"sync/atomic"
	"testing"
)

type reserveSystem struct {
	reserve func() bool
	stock   func() int64
}

type reserveFactory func(initialStock int64, afterRead func()) reserveSystem

// newCorrectReserveSystem uses compare-and-swap so the stock check and decrement form one atomic transition.
func newCorrectReserveSystem(initialStock int64, afterRead func()) reserveSystem {
	var stock atomic.Int64
	stock.Store(initialStock)
	return reserveSystem{
		reserve: func() bool {
			for {
				observed := stock.Load()
				if observed < 1 {
					return false
				}
				if afterRead != nil {
					afterRead()
				}
				if stock.CompareAndSwap(observed, observed-1) {
					return true
				}
			}
		},
		stock: stock.Load,
	}
}

// newLostUpdateReserveSystem separates read and write, reproducing the oversell fault under a forced interleaving.
func newLostUpdateReserveSystem(initialStock int64, afterRead func()) reserveSystem {
	var stock atomic.Int64
	stock.Store(initialStock)
	return reserveSystem{
		reserve: func() bool {
			observed := stock.Load()
			if observed < 1 {
				return false
			}
			if afterRead != nil {
				afterRead()
			}
			stock.Store(observed - 1)
			return true
		},
		stock: stock.Load,
	}
}

// firstRoundBarrier deterministically pauses the first two reads and releases them into the competing write together.
type firstRoundBarrier struct {
	mu       sync.Mutex
	arrivals int
	release  chan struct{}
}

// newFirstRoundBarrier creates a single-use synchronization seam; later retries pass through the closed channel.
func newFirstRoundBarrier() *firstRoundBarrier {
	return &firstRoundBarrier{release: make(chan struct{})}
}

// wait blocks until both reserve calls have observed the same pre-decrement state.
func (barrier *firstRoundBarrier) wait() {
	barrier.mu.Lock()
	barrier.arrivals++
	if barrier.arrivals == 2 {
		close(barrier.release)
	}
	release := barrier.release
	barrier.mu.Unlock()
	<-release
}

// reserveContractPasses observes only the safety, liveness, and final-state contract of two reservations.
func reserveContractPasses(factory reserveFactory, concurrent bool) bool {
	var hook func()
	if concurrent {
		hook = newFirstRoundBarrier().wait
	}
	system := factory(1, hook)
	results := runTwoReservations(system.reserve, concurrent)
	return countSuccesses(results) == 1 && system.stock() == 0
}

// runTwoReservations switches only the scheduling strategy while preserving the same public operations.
func runTwoReservations(reserve func() bool, concurrent bool) [2]bool {
	if !concurrent {
		return [2]bool{reserve(), reserve()}
	}
	var results [2]bool
	var workers sync.WaitGroup
	workers.Add(2)
	for index := range results {
		go func() {
			defer workers.Done()
			results[index] = reserve()
		}()
	}
	workers.Wait()
	return results
}

// countSuccesses turns the two outcomes into the externally meaningful number of accepted reservations.
func countSuccesses(results [2]bool) int {
	count := 0
	for _, succeeded := range results {
		if succeeded {
			count++
		}
	}
	return count
}

// TestP3ConcurrencyMutationKill proves that a deterministic interleaving kills a fault missed by sequential execution.
func TestP3ConcurrencyMutationKill(t *testing.T) {
	factories := []reserveFactory{newLostUpdateReserveSystem}
	assertCorrectCandidate(t, reserveContractPasses(newCorrectReserveSystem, true), "P3")
	baselineKills := countReserveKills(factories, false)
	skillKills := countReserveKills(factories, true)
	if baselineKills != 0 || skillKills != len(factories) {
		t.Fatalf("P3 unexpected kill matrix: baseline=%d skill=%d mutants=%d", baselineKills, skillKills, len(factories))
	}
	t.Logf("P3 kill matrix: baseline=%d/%d skill=%d/%d", baselineKills, len(factories), skillKills, len(factories))
}

// countReserveKills counts candidates rejected by either sequential or controlled-concurrency evidence.
func countReserveKills(factories []reserveFactory, concurrent bool) int {
	kills := 0
	for _, factory := range factories {
		if !reserveContractPasses(factory, concurrent) {
			kills++
		}
	}
	return kills
}
