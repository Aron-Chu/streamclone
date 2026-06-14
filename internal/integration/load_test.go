package integration_test

import (
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"streamclone/internal/chat/batch"
	"streamclone/internal/chat/tokenize"
)

func TestHighVelocityChatBoundedMemory(t *testing.T) {
	trie := tokenize.NewTrie()
	emoteNames := []string{
		"KEKW", "PogChamp", "Clap", "LULW", "catJAM",
		"monkaS", "Sadge", "EZ", "OMEGALUL", "peepoHappy",
		"FeelsBadMan", "pepeD", "LUL", "Copium", "Aware",
		"NODDERS", "GIGACHAD", "Wokege", "madge", "Clueless",
	}
	for i, name := range emoteNames {
		trie.Insert(name, tokenize.Emote{
			ID:  fmt.Sprintf("emote-%d", i),
			URL: fmt.Sprintf("/emotes/emote-%d/1x.webp", i),
			Zw:  i%5 == 0,
		})
	}

	dict := &tokenize.ChannelDict{}
	dict.Swap(trie)

	messages := []string{
		"hello world KEKW this is a test",
		"PogChamp that was amazing Clap Clap",
		"LULW you actually did it catJAM",
		"monkaS what is happening Sadge",
		"EZ game EZ life OMEGALUL",
		"peepoHappy nice play FeelsBadMan but also pepeD",
		"LUL classic Copium I knew it all along Aware",
		"NODDERS yes GIGACHAD energy right there",
		"Wokege did you see that madge so annoying Clueless",
		"@viewer123 KEKW LULW PogChamp triple emote combo",
	}

	const (
		totalMessages = 50000
		numWorkers    = 10
		maxMemoryMB   = 256
	)

	var memBefore runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memBefore)

	var wg sync.WaitGroup
	var tokenizeCount atomic.Int64
	var totalLatencyNs atomic.Int64
	var maxLatencyNs atomic.Int64

	perWorker := totalMessages / numWorkers

	started := time.Now()

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				msg := messages[(workerID*perWorker+i)%len(messages)]

				start := time.Now()
				frags := dict.Tokenize(msg)
				elapsed := time.Since(start).Nanoseconds()

				tokenizeCount.Add(1)
				totalLatencyNs.Add(elapsed)

				for {
					current := maxLatencyNs.Load()
					if elapsed <= current {
						break
					}
					if maxLatencyNs.CompareAndSwap(current, elapsed) {
						break
					}
				}

				if len(frags) == 0 {
					t.Errorf("expected fragments, got empty for msg: %s", msg)
				}
			}
		}(w)
	}

	wg.Wait()
	wallTime := time.Since(started)

	var memAfter runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memAfter)

	heapGrowthMB := float64(int64(memAfter.HeapAlloc)-int64(memBefore.HeapAlloc)) / (1024 * 1024)
	totalSysMB := float64(memAfter.Sys) / (1024 * 1024)

	count := tokenizeCount.Load()
	avgLatencyUs := float64(totalLatencyNs.Load()) / float64(count) / 1000
	maxLatencyUs := float64(maxLatencyNs.Load()) / 1000
	throughput := float64(count) / wallTime.Seconds()

	t.Logf("Tokenizer load test results:")
	t.Logf("  Messages processed: %d", count)
	t.Logf("  Wall time: %v", wallTime)
	t.Logf("  Throughput: %.0f msg/s", throughput)
	t.Logf("  Avg latency: %.2f µs", avgLatencyUs)
	t.Logf("  Max latency: %.2f µs", maxLatencyUs)
	t.Logf("  Heap growth: %.2f MB", heapGrowthMB)
	t.Logf("  Total sys: %.2f MB", totalSysMB)

	if heapGrowthMB > float64(maxMemoryMB) {
		t.Errorf("heap growth %.2f MB exceeds cap %d MB", heapGrowthMB, maxMemoryMB)
	}

	if throughput < 10000 {
		t.Errorf("throughput %.0f msg/s below 10,000 msg/s target", throughput)
	}
}

func TestHighVelocityChatBatchLatency(t *testing.T) {
	const (
		batchWindowMS     = 75
		totalMessages     = 10000
		numChannels       = 5
		maxBatchLatencyMS = 150
	)

	var mu sync.Mutex
	var batchLatencies []time.Duration

	latencyFlusher := func(channel string, data []byte) {
		var frame batch.Frame
		if err := json.Unmarshal(data, &frame); err != nil {
			return
		}
		if len(frame.Messages) < 2 {
			return
		}
		minReceived := frame.Messages[0].ServerReceivedTS
		for _, msg := range frame.Messages[1:] {
			if msg.ServerReceivedTS < minReceived {
				minReceived = msg.ServerReceivedTS
			}
		}
		latency := time.Duration(frame.ServerSentTS-minReceived) * time.Millisecond
		mu.Lock()
		batchLatencies = append(batchLatencies, latency)
		mu.Unlock()
	}

	b := batch.New(batchWindowMS, latencyFlusher)

	trie := tokenize.NewTrie()
	for i, name := range []string{"KEKW", "PogChamp", "Clap", "LULW", "catJAM"} {
		trie.Insert(name, tokenize.Emote{
			ID:  fmt.Sprintf("emote-%d", i),
			URL: fmt.Sprintf("/emotes/emote-%d/1x.webp", i),
		})
	}
	dict := &tokenize.ChannelDict{}
	dict.Swap(trie)

	messages := []string{
		"hello KEKW world",
		"PogChamp nice one Clap",
		"LULW catJAM dance",
		"just chatting KEKW KEKW",
		"@user PogChamp LULW",
	}

	var wg sync.WaitGroup
	perChannel := totalMessages / numChannels

	started := time.Now()

	for ch := 0; ch < numChannels; ch++ {
		wg.Add(1)
		go func(chIdx int) {
			defer wg.Done()
			channel := fmt.Sprintf("channel_%d", chIdx)
			for i := 0; i < perChannel; i++ {
				msg := messages[i%len(messages)]
				frags := dict.Tokenize(msg)

				batchFrags := make([]batch.Fragment, len(frags))
				copy(batchFrags, frags)

				b.Add(channel, batch.BatchMessage{
					ID:        fmt.Sprintf("msg-%d-%d", chIdx, i),
					User:      fmt.Sprintf("user%d", i%100),
					Color:     "#1E90FF",
					TS:        time.Now().UnixMilli(),
					Fragments: batchFrags,
				})

				if i%100 == 0 {
					time.Sleep(time.Microsecond)
				}
			}
		}(ch)
	}

	wg.Wait()
	wallTime := time.Since(started)

	time.Sleep(time.Duration(batchWindowMS*2) * time.Millisecond)

	mu.Lock()
	latencyCount := len(batchLatencies)
	mu.Unlock()

	throughput := float64(totalMessages) / wallTime.Seconds()

	t.Logf("Batch load test results:")
	t.Logf("  Messages sent: %d across %d channels", totalMessages, numChannels)
	t.Logf("  Wall time: %v", wallTime)
	t.Logf("  Throughput: %.0f msg/s", throughput)
	t.Logf("  Batch flushes observed: %d", latencyCount)
	t.Logf("  Batch window: %d ms (max allowed: %d ms)", batchWindowMS, maxBatchLatencyMS)

	if latencyCount == 0 {
		t.Error("expected at least one multi-message batch flush")
	}

	var maxLatency time.Duration
	for _, lat := range batchLatencies {
		if lat > maxLatency {
			maxLatency = lat
		}
	}

	if maxLatency > time.Duration(maxBatchLatencyMS)*time.Millisecond {
		t.Errorf("max batch flush latency %v exceeds cap %d ms", maxLatency, maxBatchLatencyMS)
	}

	var memStats runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memStats)
	heapMB := float64(memStats.HeapAlloc) / (1024 * 1024)
	t.Logf("  Heap after test: %.2f MB", heapMB)

	if heapMB > 128 {
		t.Errorf("heap %.2f MB exceeds 128 MB bound after processing %d messages", heapMB, totalMessages)
	}
}

func TestTokenizerConcurrentSwapUnderLoad(t *testing.T) {
	dict := &tokenize.ChannelDict{}

	buildTrie := func(emoteCount int) *tokenize.Trie {
		trie := tokenize.NewTrie()
		for i := 0; i < emoteCount; i++ {
			trie.Insert(fmt.Sprintf("Emote%d", i), tokenize.Emote{
				ID:  fmt.Sprintf("id-%d", i),
				URL: fmt.Sprintf("/emotes/id-%d/1x.webp", i),
				Zw:  i%3 == 0,
			})
		}
		return trie
	}

	dict.Swap(buildTrie(50))

	const (
		duration    = 2 * time.Second
		numReaders  = 8
		swapEveryMS = 50
	)

	ctx := make(chan struct{})
	var wg sync.WaitGroup
	var ops atomic.Int64
	var panicked atomic.Int32

	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panicked.Add(1)
				}
			}()
			msgs := []string{
				"hello Emote0 world Emote1",
				"Emote5 test Emote10 Emote20",
				"no emotes here at all",
				"Emote49 last one Emote0",
			}
			for {
				select {
				case <-ctx:
					return
				default:
				}
				for _, msg := range msgs {
					frags := dict.Tokenize(msg)
					if len(frags) == 0 {
						panic("empty frags")
					}
					ops.Add(1)
				}
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Duration(swapEveryMS) * time.Millisecond)
		defer ticker.Stop()
		sizes := []int{10, 25, 50, 100, 50, 25}
		idx := 0
		for {
			select {
			case <-ctx:
				return
			case <-ticker.C:
				dict.Swap(buildTrie(sizes[idx%len(sizes)]))
				idx++
			}
		}
	}()

	time.Sleep(duration)
	close(ctx)
	wg.Wait()

	if panicked.Load() > 0 {
		t.Errorf("concurrent swap caused %d panics", panicked.Load())
	}

	t.Logf("Concurrent swap test: %d operations in %v (%.0f ops/s)", ops.Load(), duration, float64(ops.Load())/duration.Seconds())
}
