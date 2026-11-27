package v1

import (
	"errors"
	"sync"
	"time"
	"yamaclient/lib/api"
	"yamaclient/lib/channel"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

var errNilChannelEntry = errors.New("channel in cache was nil")

type channelCache struct {
	mu       sync.Mutex // serializes lookup + assign (compound ops aren't atomic)
	channels []*channel.Channel
	lru      *expirable.LRU[api.Song, int]

	freeMu sync.Mutex // guards free; release() can run on a background goroutine
	free   []int
}

func newChannelCache(chs []*channel.Channel, ttl time.Duration) *channelCache {
	c := &channelCache{channels: chs, free: make([]int, 0, len(chs))}
	for i := len(chs) - 1; i >= 0; i-- {
		c.free = append(c.free, i)
	}
	c.lru = expirable.NewLRU(len(chs), c.Release, ttl)
	return c
}

func (c *channelCache) ForEach(f func(*channel.Channel)) {
	for _, c := range c.channels {
		f(c)
	}
}

// fires on eviction, TTL expiry and lru Remove()
func (c *channelCache) Release(_ api.Song, idx int) {
	c.freeMu.Lock()
	c.free = append(c.free, idx)
	c.freeMu.Unlock()
}

func (c *channelCache) Pop() (int, bool) {
	c.freeMu.Lock()
	defer c.freeMu.Unlock()
	n := len(c.free)
	if n == 0 {
		return 0, false
	}
	idx := c.free[n-1]
	c.free = c.free[:n-1]
	return idx, true
}

// Stops cursong if channel held it (.Play() should ensure directly after)
func (c *channelCache) Load(song api.Song) (*channel.Channel, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if idx, ok := c.lru.Get(song); ok {
		return c.channels[idx], nil
	}
	c.lru.Remove(song) // drop a stale/expired entry @ this key so its slot isn't leaked

	idx, ok := c.Pop()
	if !ok {
		// evict, retry
		c.lru.RemoveOldest()
		if idx, ok = c.Pop(); !ok {
			return nil, errors.New("no free channel")
		}
	}

	ch := c.channels[idx]
	if ch == nil {
		panic(errNilChannelEntry)
	}
	ch.Stop() // ex, was playing back-to-back
	content := channel.Content{Name: song.Name, Data: song.Name, Source: api.ToSourceType(song.Type)}
	if err := ch.Load(content, false); err != nil {
		c.Release(song, idx)
		return nil, err
	}
	c.lru.Add(song, idx) // shouldnt evict (since we checked ahead)
	return ch, nil
}
