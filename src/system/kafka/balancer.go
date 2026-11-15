package kafka

import (
	"cmp"
	"slices"
	"sync"
)

// leastBytes picks the partition of a message with the rule of bill's balancer (task 045 D10b:
// kafka-go v0.4.31 LeastBytes.Balance, read on 2026-11-15). librdkafka has no such partitioner, so
// the producer sets the partition of each message itself.
//   - One counter list for the producer (all topics share it, as one bill writer).
//   - When the number of partitions differs from the number of counters, the list is made again with
//     all counts 0, sorted by partition number.
//   - The message goes to the counter with the fewest bytes; on a tie, the first one (the lowest
//     partition number). Then that count grows by the bytes of the key and the value.
//
// The counts are in memory and start at 0 (as bill).
type leastBytes struct {
	mu       sync.Mutex
	counters []leastBytesCounter
}

type leastBytesCounter struct {
	partition int32
	bytes     uint64
}

// balance returns the partition of a message of size bytes (len(key) + len(value)). partitions must
// not be empty.
func (lb *leastBytes) balance(size int, partitions []int32) int32 {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	if len(partitions) != len(lb.counters) {
		lb.counters = make([]leastBytesCounter, len(partitions))
		for i, p := range partitions {
			lb.counters[i].partition = p
		}
		slices.SortFunc(lb.counters, func(a, b leastBytesCounter) int { return cmp.Compare(a.partition, b.partition) })
	}
	best := 0
	for i := 1; i < len(lb.counters); i++ {
		if lb.counters[i].bytes < lb.counters[best].bytes {
			best = i
		}
	}
	c := &lb.counters[best]
	c.bytes += uint64(size)
	return c.partition
}
