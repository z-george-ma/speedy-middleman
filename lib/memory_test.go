package lib

import (
	"lib/assert"
	"testing"
	"unsafe"
)

func TestMemory(t *testing.T) {
	var mem Memory
	mem.Load(make([]byte, 4096), true)
	m := mem.Alloc(10)
	assert.NotNull(t, m)
	m1 := mem.Alloc(999)
	assert.NotNull(t, m1)

	// align to memory boundary
	assert.Equal(t, 1008, *(*int32)(unsafe.Pointer(uintptr(m1) - 4)))

	// block >= min block size
	assert.Equal(t, MinBlockSize, *(*int32)(unsafe.Pointer(uintptr(m1) - 8)))

	m2 := mem.Alloc(10)
	assert.NotNull(t, m2)

	// footer is populated
	assert.Equal(t, 1008, *(*int32)(unsafe.Pointer(uintptr(m2) - 8)))

	mem.Free(m1)

	// header and footer is set to correct value after free
	assert.Equal(t, -1008, *(*int32)(unsafe.Pointer(uintptr(m1) - 4)))
	assert.Equal(t, -1008, *(*int32)(unsafe.Pointer(uintptr(m2) - 8)))

	// freed block is added to free list
	assert.Equal(t, uint32(uintptr(m1)-uintptr(mem.base.base)-4), mem.layout.free)

	b := mem.base.Get(mem.layout.free)
	lastBlock := mem.base.Get(b.layout.next)
	assert.Equal(t, 4092-1008-MinBlockSize-MinBlockSize, -int(lastBlock.layout.size))
	assert.Equal(t, mem.layout.free, lastBlock.layout.prev)
	assert.Equal(t, lastBlock.offset, lastBlock.layout.next)

	// split works
	m3 := mem.Alloc(300)
	assert.NotNull(t, m3)
	b = mem.base.Get(mem.layout.free)
	assert.Equal(t, 312-1008, int32(b.layout.size))

	// merge works
	mem.Free(m)
	mem.Free(m3)
	mem.Free(m2)

	assert.Equal(t, 0, int(mem.base.offset))
	assert.Equal(t, -int(mem.base.boundary), int(mem.base.layout.size))
	b = mem.base.Get(mem.layout.free)
	assert.Equal(t, mem.base.offset, b.offset)
	assert.Equal(t, mem.base.layout.size, b.layout.size)
	assert.Equal(t, b.offset, b.layout.next)
	assert.Equal(t, b.offset, b.layout.prev)

}
