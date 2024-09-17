package lib

import "container/list"

type lruValue[TKey comparable, T any] struct {
	key   TKey
	value T
}

type LRU[TKey comparable, T any] struct {
	dict   map[TKey]*list.Element
	list   *list.List
	length int
}

func NewLRU[TKey comparable, T any](len int, cap int) *LRU[TKey, T] {
	return &LRU[TKey, T]{
		dict:   make(map[TKey]*list.Element, cap),
		list:   list.New(),
		length: len,
	}
}

func (l *LRU[TKey, T]) Get(key TKey) (ret T, ok bool) {
	node, ok := l.dict[key]

	if ok {
		value := node.Value.(*lruValue[TKey, T])
		ret = value.value
		l.list.MoveToFront(node)
	}

	return
}

func (l *LRU[TKey, T]) Set(key TKey, value T) {
	node, ok := l.dict[key]
	if ok {
		nodeValue := node.Value.(*lruValue[TKey, T])
		nodeValue.value = value
		l.list.MoveToFront(node)
		return
	}

	node = l.list.PushFront(&lruValue[TKey, T]{key, value})
	l.dict[key] = node

	if l.list.Len() > l.length {
		nodeValue := l.list.Remove(l.list.Back()).(*lruValue[TKey, T])
		delete(l.dict, nodeValue.key)
	}
}
