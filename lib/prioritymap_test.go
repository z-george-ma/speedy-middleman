package lib

import (
	"lib/assert"
	"testing"
)

type OrderedInt struct {
	value int
	index int
	key   string
}

func (o *OrderedInt) Index() int {
	return o.index
}
func (o *OrderedInt) SetIndex(i int) {
	o.index = i
}

func (o *OrderedInt) Key() string {
	return o.key
}

func (o *OrderedInt) Less(other Orderable) bool {
	return o.value < other.(*OrderedInt).value
}

func TestPriorityMap(t *testing.T) {
	var dict PriorityMap[string, OrderedInt, *OrderedInt]
	dict.Init(5)

	assert.Equal(t, nil, dict.Set(&OrderedInt{value: 1, key: "abc1"}))
	assert.Equal(t, nil, dict.Set(&OrderedInt{value: 2, key: "abc2"}))
	assert.Equal(t, nil, dict.Set(&OrderedInt{value: 0, key: "abc3"}))
	assert.Equal(t, nil, dict.Set(&OrderedInt{value: 3, key: "abc4"}))
	assert.Equal(t, nil, dict.Set(&OrderedInt{value: 1, key: "abc5"}))
	assert.Equal(t, 0, dict.Set(&OrderedInt{value: 1, key: "abc6"}).value)
	assert.Equal(t, nil, dict.Delete("abc3"))
}
