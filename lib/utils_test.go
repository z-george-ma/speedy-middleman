package lib

import (
	"lib/assert"
	"testing"
)

func TestBytesToString(t *testing.T) {
	s := "ABC€"
	bs := []byte(s)
	assert.Equal(t, s, BytesToString(bs))

	assert.Equal(t, s, BytesToString(StringToBytes(s)))
}
