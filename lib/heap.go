package lib

type WithKey[TKey comparable] interface {
	Key() TKey
}

type Orderable interface {
	Less(other Orderable) bool
	Index() int
	SetIndex(i int)
}

type PtrOrderable[T any] interface {
	Orderable
	*T
}

type HeapSlice[T any, PT PtrOrderable[T]] []PT

func (h *HeapSlice[T, PT]) Len() int {
	return len(*h)
}

func (h *HeapSlice[T, PT]) Less(i, j int) bool {
	slice := ([]PT)(*h)
	return slice[i].Less(slice[j])
}

func (h *HeapSlice[T, PT]) Swap(i, j int) {
	slice := ([]PT)(*h)
	slice[i], slice[j] = slice[j], slice[i]
	slice[i].SetIndex(i)
	slice[j].SetIndex(j)
}

func (h *HeapSlice[T, PT]) Push(x any) {
	l := len(*h)
	item := x.(PT)
	item.SetIndex(l)
	*h = append(*h, item)
}

func (h *HeapSlice[T, PT]) Pop() (ret any) {
	newLen := len(*h) - 1
	ret = (*h)[newLen]
	*h = (*h)[:newLen]
	return
}
