package lib

import (
	"container/heap"
)

type PriorityMap[TKey comparable, T any, PT interface {
	PtrOrderable[T]
	WithKey[TKey]
}] struct {
	heap HeapSlice[T, PT]
	dict map[TKey]PT
	size int
}

// Initialise PriorityMap
func (p *PriorityMap[TKey, T, PT]) Init(size int) {
	p.dict = make(map[TKey]PT)
	p.size = size
}

// Add an item to PriorityMap and return evicted item or nil
func (p *PriorityMap[TKey, T, PT]) Set(value PT) (ret PT) {
	key := value.Key()
	item, ok := p.dict[key]

	if ok {
		i := item.Index()
		*item = *value
		item.SetIndex(i)
		heap.Fix(&p.heap, i)

		return
	}

	if len(p.heap) >= p.size {
		if value.Less(p.heap[0]) {
			return value
		}

		ret = heap.Pop(&p.heap).(*T)
		delete(p.dict, ret.Key())
	}

	heap.Push(&p.heap, value)
	p.dict[key] = value

	return
}

// Get item by key. If not present, return nil
func (p *PriorityMap[TKey, T, PT]) Get(key TKey) (ret PT) {
	ret, ok := p.dict[key]
	if !ok {
		ret = nil
	}
	return
}

func (p *PriorityMap[TKey, T, PT]) DeleteItem(item PT) {
	if item.Index() < 0 {
		return
	}
	heap.Remove(&p.heap, item.Index())
	delete(p.dict, item.Key())
	item.SetIndex(-1)
}

// Delete item by key and return the item or nil
func (p *PriorityMap[TKey, T, PT]) Delete(key TKey) (ret PT) {
	ret, ok := p.dict[key]
	if ok {
		p.DeleteItem(ret)
	}

	return
}

func (p *PriorityMap[TKey, T, PT]) Items() []PT {
	return p.heap
}
