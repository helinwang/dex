package consensus

import (
	"sync"

	lru "github.com/hashicorp/golang-lru"
)

// collector collects items and releases them once the count threshold
// is reached. It is used to collect the signature shares.
type collector struct {
	threshold int
	merged    *lru.Cache

	mu         sync.Mutex
	mergeItems map[Hash][]Hash
	items      map[Hash]interface{}
}

func newCollector(threshold int) *collector {
	c, err := lru.New(1024)
	if err != nil {
		panic(err)
	}

	return &collector{
		threshold:  threshold,
		merged:     c,
		mergeItems: make(map[Hash][]Hash),
		items:      make(map[Hash]interface{}),
	}
}

func (c *collector) Remove(target Hash) {
	c.mu.Lock()
	current := c.mergeItems[target]
	for i := range current {
		delete(c.items, current[i])
	}
	delete(c.mergeItems, target)
	c.mu.Unlock()
}

func (c *collector) Add(target Hash, itemHash Hash, item interface{}) ([]interface{}, bool) {
	if c.merged.Contains(target) {
		// already merged before
		return nil, false
	}

	c.mu.Lock()
	if _, ok := c.items[itemHash]; ok {
		// already added
		c.mu.Unlock()
		return nil, false
	}

	current := c.mergeItems[target]
	if len(current)+1 >= c.threshold {
		items := make([]interface{}, c.threshold)
		items[0] = item
		for i := range current {
			h := current[i]
			items[i+1] = c.items[h]
		}
		c.merged.Add(target, struct{}{})
		c.mu.Unlock()
		return items, false
	}

	c.mergeItems[target] = append(current, itemHash)
	c.items[itemHash] = item
	c.mu.Unlock()
	return nil, true
}

func (c *collector) Get(itemHash Hash) interface{} {
	c.mu.Lock()
	r := c.items[itemHash]
	c.mu.Unlock()
	return r
}
