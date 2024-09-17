package lib

import "unsafe"

type Memory struct {
	layout *MemoryLayout
	base   *Block
}

const MinBlockSize = 64 // including header / footer
const NoFreeBlock = ^uint32(0)

type Block struct {
	base     unsafe.Pointer
	boundary int32
	offset   uint32
	layout   *BlockLayout
}

type BlockLayout struct {
	size int32  // block size, negative means free
	prev uint32 // prev block index
	next uint32 // next block index
}

type MemoryLayout struct {
	free      uint32
	dataStart byte
}

func (m *Memory) Load(mem []byte, init bool) {
	m.layout = (*MemoryLayout)(unsafe.Pointer(&mem[0]))

	base := unsafe.Pointer(&m.layout.dataStart)
	m.base = &Block{
		base:     base,
		boundary: int32(len(mem) - 4),
		offset:   0,
		layout:   (*BlockLayout)(base),
	}
	if init {
		m.layout.free = 0
		m.base.layout.size = -m.base.boundary
		m.base.layout.prev = 0
		m.base.layout.next = 0
	}
}

func (b *Block) Get(offset uint32) *Block {
	return &Block{
		base:     b.base,
		boundary: b.boundary,
		offset:   offset,
		layout:   (*BlockLayout)(unsafe.Pointer(uintptr(b.base) + uintptr(offset))),
	}
}

func (b *Block) Resize(size int32) {
	b.layout.size = size

	end := b.offset - 4
	if size > 0 {
		end += uint32(size)
	} else {
		end += uint32(-size)
	}

	*(*int32)(unsafe.Pointer((uintptr(b.base) + uintptr(end)))) = size
}

func (b *Block) Next() *Block {
	next := int32(b.offset)
	if b.layout.size > 0 {
		next += b.layout.size
	} else {
		next += -b.layout.size
	}
	if next+MinBlockSize > b.boundary {
		return nil
	}

	return b.Get(uint32(next))
}

func (b *Block) Prev() *Block {
	if b.offset < MinBlockSize {
		// prev block should be at least MinBlockSize long
		return nil
	}

	prevSize := *(*int32)(unsafe.Pointer((uintptr(b.base) + uintptr(b.offset-4))))
	prev := int32(b.offset)

	if prevSize > 0 {
		prev -= prevSize
	} else {
		prev += prevSize
	}

	return b.Get(uint32(prev))
}

func (m *Memory) Alloc(size int) unsafe.Pointer {
	if size <= 0 || m.layout.free == NoFreeBlock {
		return nil
	}

	block := m.base.Get(m.layout.free)
	s := int32((size+7)/8*8) + 8 // 4 bytes size * 2, aligned to n*8
	if s < MinBlockSize {
		s = MinBlockSize
	}

	for {
		// first fit
		if s <= -block.layout.size {
			if s+MinBlockSize <= -block.layout.size {
				// split
				newBlock := block.Get(block.offset + uint32(s))

				if block.offset != block.layout.prev {
					prev := block.Get(block.layout.prev)
					prev.layout.next = newBlock.offset
					newBlock.layout.prev = prev.offset
				} else {
					newBlock.layout.prev = newBlock.offset
				}

				if block.offset != block.layout.next {
					next := block.Get(block.layout.next)
					next.layout.prev = newBlock.offset
					newBlock.layout.next = next.offset
				} else {
					newBlock.layout.next = newBlock.offset
				}

				newBlock.Resize(block.layout.size + s)

				if block.offset == m.layout.free {
					m.layout.free = newBlock.offset
				}

				block.Resize(s)
				return unsafe.Pointer(&block.layout.prev)
			}

			if block.offset == block.layout.next {
				// no more free block
				m.layout.free = NoFreeBlock
			} else {
				next := block.Get(block.layout.next)
				prev := block
				if block.offset != block.layout.prev {
					prev = block.Get(block.layout.prev)
					next.layout.prev = prev.offset
				} else {
					next.layout.prev = next.offset
				}

				prev.layout.next = block.layout.next
			}

			block.Resize(-block.layout.size)

			return unsafe.Pointer(&block.layout.prev)
		}

		if block.offset == block.layout.next {
			return nil
		}

		block = block.Get(block.layout.next)
	}
}

func (m *Memory) Free(item unsafe.Pointer) {
	if item == nil {
		return
	}

	block := m.base.Get(uint32(uintptr(item) - 4 - uintptr(m.base.base)))
	block.Resize(-block.layout.size)

	// coalesce next
	for {
		next := block.Next()
		if next == nil || next.layout.size > 0 {
			break
		}

		if next.layout.prev == next.offset {
			m.layout.free = NoFreeBlock
		} else {
			prev := next.Get(next.layout.prev)
			if next.layout.next != next.offset {
				prev.layout.next = next.layout.next
			} else {
				prev.layout.next = prev.offset
			}
		}

		block.Resize(block.layout.size + next.layout.size)
	}

	// coalesce prev
	for {
		prev := block.Prev()
		if prev == nil || prev.layout.size > 0 {
			break
		}

		prev.Resize(block.layout.size + prev.layout.size)
		block = prev
	}

	block.layout.prev = block.offset

	if m.layout.free == NoFreeBlock {
		block.layout.next = block.offset
	} else {
		free := m.base.Get(m.layout.free)
		free.layout.prev = block.offset
		block.layout.next = free.offset
	}

	m.layout.free = block.offset
}
