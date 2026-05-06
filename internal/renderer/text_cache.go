//go:build !headless

package renderer

import (
	"container/list"

	"github.com/veandco/go-sdl2/sdl"
)

const maxTextCacheEntries = 256

// sizeKey is the cache key for text measurement results (SizeUTF8 CGo calls).
// bold is included because the primary font's bold style changes measurements.
type sizeKey struct {
	text   string
	fontID uint8
	small  bool
	bold   bool
}

// wrapKey is the cache key for WrapText results.
type wrapKey struct {
	text     string
	maxWidth int32
}

type textRunKey struct {
	text   string
	fontID uint8
	small  bool
	bold   bool
	r, g, b uint8
}

type textRunVal struct {
	tex  *sdl.Texture
	w, h int32
}

type textRunEntry struct {
	key textRunKey
	val textRunVal
}

type textCache struct {
	lru   *list.List
	items map[textRunKey]*list.Element
	max   int
}

func newTextCache(max int) *textCache {
	return &textCache{
		lru:   list.New(),
		items: make(map[textRunKey]*list.Element),
		max:   max,
	}
}

func (c *textCache) get(key textRunKey) (textRunVal, bool) {
	el, ok := c.items[key]
	if !ok {
		return textRunVal{}, false
	}
	c.lru.MoveToFront(el)
	return el.Value.(*textRunEntry).val, true
}

func (c *textCache) put(key textRunKey, val textRunVal) {
	if el, ok := c.items[key]; ok {
		c.lru.MoveToFront(el)
		el.Value.(*textRunEntry).val = val
		return
	}
	entry := &textRunEntry{key: key, val: val}
	el := c.lru.PushFront(entry)
	c.items[key] = el
	for c.lru.Len() > c.max {
		back := c.lru.Back()
		if back == nil {
			break
		}
		evicted := back.Value.(*textRunEntry)
		evicted.val.tex.Destroy()
		delete(c.items, evicted.key)
		c.lru.Remove(back)
	}
}

func (c *textCache) clear() {
	for _, el := range c.items {
		el.Value.(*textRunEntry).val.tex.Destroy()
	}
	c.lru.Init()
	c.items = make(map[textRunKey]*list.Element)
}
