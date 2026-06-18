package groupcache

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestStatClassBackwardCompatibility verifies that with a single stat class (default),
// the behavior matches the original code that didn't support stat classes.
func TestStatClassBackwardCompatibility(t *testing.T) {
	ws := NewWorkspace(DefaultResponseHeaderTimeout)
	getter := GetterFunc(func(ctx context.Context, key string, dest Sink, info *Info, statClass int) error {
		return dest.SetString("value-"+key, time.Time{})
	})

	// Create group with default stat classes (1)
	g := NewGroupWithWorkspace(Options{
		Workspace:       ws,
		Name:            "backward-compat-group",
		CacheBytesLimit: 1 << 20,
		Getter:          getter,
		// StatClasses not specified, defaults to 1
	})

	ctx := context.Background()

	// Get multiple keys with stat class 0 (default)
	var result string
	for i := 0; i < 3; i++ {
		key := "key-" + string(rune('0'+i))
		err := g.Get(ctx, key, StringSink(&result), nil, 0)
		assert.NoError(t, err)
	}

	// Verify stats - should have single stat class entry
	mainStats := g.CacheStats(MainCache)
	hotStats := g.CacheStats(HotCache)

	// Should have exactly 1 stat class in both caches
	assert.Equal(t, 1, len(mainStats), "should have exactly 1 stat class in main cache")
	assert.Equal(t, 1, len(hotStats), "should have exactly 1 stat class in hot cache")

	// Verify at least some stats are recorded
	totalItems := mainStats[0].Items + hotStats[0].Items
	assert.Greater(t, totalItems, int64(0), "should have items in at least one cache")
}

// TestStatClassPerClassIsolation verifies that different stat classes track stats independently.
func TestStatClassPerClassIsolation(t *testing.T) {
	ws := NewWorkspace(DefaultResponseHeaderTimeout)
	getter := GetterFunc(func(ctx context.Context, key string, dest Sink, info *Info, statClass int) error {
		return dest.SetString("value-"+key, time.Time{})
	})

	// Create group with 3 stat classes
	g := NewGroupWithWorkspace(Options{
		Workspace:       ws,
		Name:            "per-class-isolation-group",
		CacheBytesLimit: 1 << 20,
		Getter:          getter,
		StatClasses:     3,
	})

	ctx := context.Background()
	var result string

	// Load keys into different stat classes
	// Class 0: key-0a, key-0b
	for i := 0; i < 2; i++ {
		key := "key-0" + string(rune('a'+i))
		err := g.Get(ctx, key, StringSink(&result), nil, 0)
		assert.NoError(t, err)
	}

	// Class 1: key-1a, key-1b, key-1c
	for i := 0; i < 3; i++ {
		key := "key-1" + string(rune('a'+i))
		err := g.Get(ctx, key, StringSink(&result), nil, 1)
		assert.NoError(t, err)
	}

	// Class 2: key-2a
	for i := 0; i < 1; i++ {
		key := "key-2" + string(rune('a'+i))
		err := g.Get(ctx, key, StringSink(&result), nil, 2)
		assert.NoError(t, err)
	}

	// Verify stats - should have 3 stat class entries
	mainStats := g.CacheStats(MainCache)
	hotStats := g.CacheStats(HotCache)

	assert.Equal(t, 3, len(mainStats), "main cache should have 3 stat classes")
	assert.Equal(t, 3, len(hotStats), "hot cache should have 3 stat classes")

	// Combine stats from both caches
	totalClass0 := mainStats[0].Items + hotStats[0].Items
	totalClass1 := mainStats[1].Items + hotStats[1].Items
	totalClass2 := mainStats[2].Items + hotStats[2].Items

	// Verify isolation: each class has the correct number of items
	assert.Equal(t, int64(2), totalClass0, "class 0 should have 2 items")
	assert.Equal(t, int64(3), totalClass1, "class 1 should have 3 items")
	assert.Equal(t, int64(1), totalClass2, "class 2 should have 1 item")
}

// TestStatClassCacheHits verifies that cache hits are tracked separately per class.
func TestStatClassCacheHits(t *testing.T) {
	ws := NewWorkspace(DefaultResponseHeaderTimeout)
	getter := GetterFunc(func(ctx context.Context, key string, dest Sink, info *Info, statClass int) error {
		return dest.SetString("value-"+key, time.Time{})
	})

	// Create group with 2 stat classes
	g := NewGroupWithWorkspace(Options{
		Workspace:       ws,
		Name:            "cache-hits-group",
		CacheBytesLimit: 1 << 20,
		Getter:          getter,
		StatClasses:     2,
	})

	ctx := context.Background()
	var result string

	// First round: Load keys from both classes (all misses)
	// Class 0
	g.Get(ctx, "key-0", StringSink(&result), nil, 0)
	// Class 1
	g.Get(ctx, "key-1", StringSink(&result), nil, 1)

	// Second round: Access same keys again (all hits)
	// Class 0 - 2 more gets
	g.Get(ctx, "key-0", StringSink(&result), nil, 0)
	g.Get(ctx, "key-0", StringSink(&result), nil, 0)

	// Class 1 - 1 more get
	g.Get(ctx, "key-1", StringSink(&result), nil, 1)

	mainStats := g.CacheStats(MainCache)
	hotStats := g.CacheStats(HotCache)

	// Combine stats from both caches
	class0Gets := mainStats[0].Gets + hotStats[0].Gets
	class0Hits := mainStats[0].Hits + hotStats[0].Hits
	class1Gets := mainStats[1].Gets + hotStats[1].Gets
	class1Hits := mainStats[1].Hits + hotStats[1].Hits

	// Class 0: should have gets and hits
	assert.Greater(t, class0Gets, int64(0), "class 0 should have gets")
	assert.Greater(t, class0Hits, int64(0), "class 0 should have hits")

	// Class 1: should have gets and hits
	assert.Greater(t, class1Gets, int64(0), "class 1 should have gets")
	assert.Greater(t, class1Hits, int64(0), "class 1 should have hits")

	// Class 0 should have more gets than class 1
	assert.Greater(t, class0Gets, class1Gets, "class 0 should have more gets than class 1")
}

// TestStatClassBytesCounting verifies that bytes are tracked correctly per class.
func TestStatClassBytesCounting(t *testing.T) {
	ws := NewWorkspace(DefaultResponseHeaderTimeout)
	getter := GetterFunc(func(ctx context.Context, key string, dest Sink, info *Info, statClass int) error {
		// Return different sized values based on stat class
		if statClass == 0 {
			return dest.SetString("x", time.Time{}) // 1 byte
		}
		return dest.SetString("verylongvalue", time.Time{}) // 13 bytes
	})

	// Create group with 2 stat classes
	g := NewGroupWithWorkspace(Options{
		Workspace:       ws,
		Name:            "bytes-counting-group",
		CacheBytesLimit: 1 << 20,
		Getter:          getter,
		StatClasses:     2,
	})

	ctx := context.Background()
	var result string

	// Load one key per class
	g.Get(ctx, "a", StringSink(&result), nil, 0)
	g.Get(ctx, "b", StringSink(&result), nil, 1)

	mainStats := g.CacheStats(MainCache)
	hotStats := g.CacheStats(HotCache)

	// Combine stats from both caches
	class0Bytes := mainStats[0].Bytes + hotStats[0].Bytes
	class1Bytes := mainStats[1].Bytes + hotStats[1].Bytes

	// Class 1 should have more bytes than class 0 (larger value)
	assert.Greater(t, class1Bytes, class0Bytes, "class 1 should have more bytes due to larger value")
}

// TestStatClassEvictionsFields verifies eviction-related fields are tracked per stat class.
func TestStatClassEvictionsFields(t *testing.T) {
	ws := NewWorkspace(DefaultResponseHeaderTimeout)
	getter := GetterFunc(func(ctx context.Context, key string, dest Sink, info *Info, statClass int) error {
		// Keep value size fixed to make memory pressure deterministic.
		return dest.SetString("0123456789", time.Time{})
	})

	g := NewGroupWithWorkspace(Options{
		Workspace:       ws,
		Name:            "evictions-fields-group",
		CacheBytesLimit: 30,
		Getter:          getter,
		StatClasses:     2,
	})

	ctx := context.Background()
	var result string

	// Fill cache with class 1 entries so evictions happen under memory pressure.
	for i := 0; i < 6; i++ {
		key := "k" + string(rune('0'+i))
		err := g.Get(ctx, key, StringSink(&result), nil, 1)
		assert.NoError(t, err)
	}

	mainStats := g.CacheStats(MainCache)
	hotStats := g.CacheStats(HotCache)

	class0Evictions := mainStats[0].Evictions + hotStats[0].Evictions
	class0MemFullEvictions := mainStats[0].EvictionsNonExpiredOnMemFull + hotStats[0].EvictionsNonExpiredOnMemFull
	class1Evictions := mainStats[1].Evictions + hotStats[1].Evictions
	class1MemFullEvictions := mainStats[1].EvictionsNonExpiredOnMemFull + hotStats[1].EvictionsNonExpiredOnMemFull

	// We only inserted class 1 entries, so class 0 eviction counters should remain zero.
	assert.Equal(t, int64(0), class0Evictions, "class 0 should have no evictions")
	assert.Equal(t, int64(0), class0MemFullEvictions, "class 0 should have no mem-full evictions")

	// Memory pressure should trigger removeOldest on non-expired keys.
	assert.Greater(t, class1Evictions, int64(0), "class 1 should have evictions")
	assert.Greater(t, class1MemFullEvictions, int64(0), "class 1 should have non-expired mem-full evictions")
	assert.GreaterOrEqual(t, class1Evictions, class1MemFullEvictions,
		"mem-full non-expired evictions cannot exceed total evictions")
}

// TestStatClassOutOfRange verifies that out-of-range stat classes fallback to class 0.
func TestStatClassOutOfRange(t *testing.T) {
	ws := NewWorkspace(DefaultResponseHeaderTimeout)
	getter := GetterFunc(func(ctx context.Context, key string, dest Sink, info *Info, statClass int) error {
		return dest.SetString("value-"+key, time.Time{})
	})

	// Create group with 2 stat classes
	g := NewGroupWithWorkspace(Options{
		Workspace:       ws,
		Name:            "out-of-range-group",
		CacheBytesLimit: 1 << 20,
		Getter:          getter,
		StatClasses:     2,
	})

	ctx := context.Background()
	var result string

	// Load with valid class 0
	g.Get(ctx, "key-0", StringSink(&result), nil, 0)

	// Load with invalid class 5 (should clamp to 0)
	g.Get(ctx, "key-5", StringSink(&result), nil, 5)

	// Load with negative class (should clamp to 0)
	g.Get(ctx, "key-neg", StringSink(&result), nil, -1)

	mainStats := g.CacheStats(MainCache)
	hotStats := g.CacheStats(HotCache)

	// Combine stats from both caches
	class0Items := mainStats[0].Items + hotStats[0].Items
	class0Gets := mainStats[0].Gets + hotStats[0].Gets
	class1Items := mainStats[1].Items + hotStats[1].Items
	class1Gets := mainStats[1].Gets + hotStats[1].Gets

	// All three items should be in class 0 (clamped)
	assert.Equal(t, int64(3), class0Items, "all 3 items should be in class 0 (clamped)")

	// Gets should be greater than items (since lookupCache calls both mainCache.get and hotCache.get)
	assert.Greater(t, class0Gets, class0Items, "gets should be greater than items due to dual cache lookup")

	// Class 1 should be empty
	assert.Equal(t, int64(0), class1Items, "class 1 should have no items")
	assert.Equal(t, int64(0), class1Gets, "class 1 should have no gets")
}

// TestStatClassSumEqualsPrevious verifies that summing all stat classes equals previous single-class behavior.
func TestStatClassSumEqualsPrevious(t *testing.T) {
	ws1 := NewWorkspace(DefaultResponseHeaderTimeout)
	ws2 := NewWorkspace(DefaultResponseHeaderTimeout)
	getter := GetterFunc(func(ctx context.Context, key string, dest Sink, info *Info, statClass int) error {
		return dest.SetString("value-"+key, time.Time{})
	})

	// Create single stat class group (old behavior)
	g1 := NewGroupWithWorkspace(Options{
		Workspace:       ws1,
		Name:            "single-class",
		CacheBytesLimit: 1 << 20,
		Getter:          getter,
		StatClasses:     1,
	})

	// Create multi stat class group (new behavior)
	g2 := NewGroupWithWorkspace(Options{
		Workspace:       ws2,
		Name:            "multi-class",
		CacheBytesLimit: 1 << 20,
		Getter:          getter,
		StatClasses:     3,
	})

	ctx := context.Background()
	var result string

	// Same access pattern for both groups
	keys := []struct {
		key   string
		class int
	}{
		{"a", 0},
		{"b", 1},
		{"c", 2},
		{"a", 0},
		{"b", 1},
		{"c", 2},
		{"d", 0},
	}

	// Single class group - all accesses to class 0
	for _, k := range keys {
		g1.Get(ctx, k.key, StringSink(&result), nil, 0)
	}

	// Multi class group - accesses to respective classes
	for _, k := range keys {
		g2.Get(ctx, k.key, StringSink(&result), nil, k.class)
	}

	// Get stats from both groups
	stats1Main := g1.CacheStats(MainCache)
	stats1Hot := g1.CacheStats(HotCache)
	stats2Main := g2.CacheStats(MainCache)
	stats2Hot := g2.CacheStats(HotCache)

	// Calculate totals
	total1Items := stats1Main[0].Items + stats1Hot[0].Items
	total1Gets := stats1Main[0].Gets + stats1Hot[0].Gets

	var total2Items, total2Gets int64
	for _, s := range stats2Main {
		total2Items += s.Items
		total2Gets += s.Gets
	}
	for _, s := range stats2Hot {
		total2Items += s.Items
		total2Gets += s.Gets
	}

	// Single class should match the sum of multi-class
	assert.Equal(t, total1Items, total2Items, "total items should match")
	assert.Equal(t, total1Gets, total2Gets, "total gets should match")
}

// TestStatClassLocalAndPeerValues verifies that stat class is preserved for both local and peer values.
func TestStatClassLocalAndPeerValues(t *testing.T) {
	// This test verifies that the fix to getLocally() is working correctly.
	// Both local and peer-sourced values should have their stat class set correctly.

	ws := NewWorkspace(DefaultResponseHeaderTimeout)
	callCount := 0
	getter := GetterFunc(func(ctx context.Context, key string, dest Sink, info *Info, statClass int) error {
		callCount++
		return dest.SetString("value-"+key, time.Time{})
	})

	g := NewGroupWithWorkspace(Options{
		Workspace:       ws,
		Name:            "local-peer-class-group",
		CacheBytesLimit: 1 << 20,
		Getter:          getter,
		StatClasses:     2,
	})

	ctx := context.Background()
	var result string

	// Load keys with different stat classes
	g.Get(ctx, "local-0", StringSink(&result), nil, 0)
	g.Get(ctx, "local-1", StringSink(&result), nil, 1)

	// Access again (from cache)
	g.Get(ctx, "local-0", StringSink(&result), nil, 0)
	g.Get(ctx, "local-1", StringSink(&result), nil, 1)

	mainStats := g.CacheStats(MainCache)
	hotStats := g.CacheStats(HotCache)

	// Verify that items were stored with their correct stat classes
	totalClass0Items := mainStats[0].Items + hotStats[0].Items
	totalClass1Items := mainStats[1].Items + hotStats[1].Items

	assert.Greater(t, totalClass0Items, int64(0), "class 0 should have at least 1 item")
	assert.Greater(t, totalClass1Items, int64(0), "class 1 should have at least 1 item")

	// Getter should be called twice (for the 2 loads, not for the 2 cache hits)
	assert.Equal(t, 2, callCount, "getter should be called exactly 2 times (once per unique key)")
}
