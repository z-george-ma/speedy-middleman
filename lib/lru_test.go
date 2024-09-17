package lib

import (
	"lib/assert"
	"testing"
)

func TestLRU(t *testing.T) {
	rt := NewLRU[string, string](3, 1)

	rt.Set("abc", "def")
	rt.Set("abc1", "def")
	rt.Set("abc2", "def")
	rt.Set("abc3", "def")

	_, ok := rt.Get("abc")
	assert.Equal(t, false, ok)

	rt.Set("abc1", "def2")
	rt.Set("abc4", "def")
	_, ok = rt.Get("abc2")
	assert.Equal(t, false, ok)

	v, _ := rt.Get("abc1")
	assert.Equal(t, "def2", v)

	rt.Set("abc5", "def")
	rt.Set("abc6", "def")
	_, ok = rt.Get("abc4")
	assert.Equal(t, false, ok)
}
