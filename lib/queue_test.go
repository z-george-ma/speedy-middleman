package lib

import (
	"context"
	"lib/assert"
	"testing"
)

func TestQueuePeekPop(t *testing.T) {
	var q Queue[int]
	q.Init()

	q.Push(1)
	ret, ok := q.Peek(context.Background())
	assert.Equal(t, 1, ret)
	assert.Equal(t, true, ok)
	q.Pop()

	q.Push(2)
	ret, ok = q.Peek(context.Background())

	assert.Equal(t, 2, ret)
	assert.Equal(t, true, ok)
}
