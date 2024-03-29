package immcheck

import (
	"reflect"
	"sync"
)

type cache map[reflect.Type]interface{}

type cacheStripe struct {
	cache cache
}

// PCache is safe for concurrent use cache that tries to keep data local for the goroutine
// and reduce synchronization overhead.
//
// Due to its implementation specifics,
// in some edge cases, PCache can potentially restore previously-stored items after eviction,
// so please take into account that it is possible and valid to observe "old" values of the specific key.
// While this behavior is unconventional, it is totally usable for immutable key-value pairs,
// keys that will always resolve into the same value,
// and just in cases when it is easy for you to identify that the value is old and drop it or set to the new one.
//
// All operations run in amortized constant time.
// PCache does its best to cache items inside and do as little synchronization as possible
// but since it is cache, there is no guarantee that PCache won't evict your item after store.
//
// PCache evicts random items if I goroutine local cache achieves maxSizePerGoroutine size.
// PCache cleans itself entirely from time to time.
//
// The zero PCache is invalid. Use NewPCache method to create PCache.
type pCache struct {
	maxSize int
	pool    *sync.Pool
}

// newPCache creates PCache with maxSizePerGoroutine.
func newPCache(maxSizePerGoroutine uint) *pCache {
	return &pCache{
		maxSize: int(maxSizePerGoroutine),
		pool: &sync.Pool{
			New: func() interface{} {
				return &cacheStripe{
					cache: make(cache),
				}
			},
		},
	}
}

// load fetches (value, true) from cache associated with key or (nil, false) if it is not present.
func (p *pCache) load(key reflect.Type) (interface{}, bool) {
	stripe := p.pool.Get().(*cacheStripe)
	defer p.pool.Put(stripe)
	value, ok := stripe.cache[key]
	return value, ok
}

// store stores value for a key in cache.
func (p *pCache) store(key reflect.Type, value interface{}) {
	stripe := p.pool.Get().(*cacheStripe)
	defer p.pool.Put(stripe)

	stripe.cache[key] = value
	if len(stripe.cache) <= p.maxSize {
		return
	}
	for k := range stripe.cache {
		delete(stripe.cache, k)
		break
	}
}
