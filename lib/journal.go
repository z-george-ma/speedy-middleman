package lib

import (
	"fmt"
	"unsafe"
)

type JournalBodyOffset struct {
	logOffset  int32
	keyOffset  int32
	keySize    int32
	valueSize  int32
	prevOffset int32
	nextOffset int32
}

type JournalBucketMeta struct {
	count   byte
	deleted byte // bitmap. sign bit true indicates overflow
}

type JournalMapHeader struct {
	version int32 // 4 bytes
	length  int32 // 4 bytes, length of hash map, offset 4
	head    int32 // offset to first item, 4 bytes, offset 8
	buckets int32 // 4 bytes, offset 12
}

type JournalLogDataHeader struct {
	head   int32
	length int32
	seq    int64
	cap    int32
}

type Journal struct {
	// load factor 0.75
	header            *JournalMapHeader   // 16 bytes header
	JournalBucketMeta []JournalBucketMeta // 2 bytes per bucket, offset 16
	buckets           []uint64            // hash value for items in the bucket, 32 bytes per item, offset 16+2*buckets
	bodyOffset        []JournalBodyOffset // 24 bytes, offset 16 + 2*buckets + 32*buckets
	log               *JournalLogDataHeader
	logData           []int32 // on AARCH64 this needs to be 16 bytes aligned
	data              []byte  // compact data format, offset 16+130*bucket
	outputCh          chan JournalKVP
}

type HashableString string

type Hashable interface {
	GetHashCode() uint64
}

func (s HashableString) GetHashCode() uint64 {
	// https://stackoverflow.com/questions/7666509/hash-function-for-string
	// http://www.cse.yorku.ca/~oz/hash.html
	var hash uint64 = 5381

	for _, c := range s {
		hash = (hash << 5) + hash + uint64(c)
	}

	return hash
}

const (
	FOUND = iota
	DELETED
	EMPTY
	FULL
	OVERFLOWED
)

func (j *Journal) findSlot(key string, hash uint64, findSlotToWrite bool) (retCode int, retOffset int32) {
	startBucket := int32(-1)
	bucket := int32(hash % uint64(j.header.buckets))

	firstDeleted := int32(-1)
	for startBucket != bucket {
		if startBucket == -1 {
			startBucket = bucket
		}

		meta := &j.JournalBucketMeta[bucket]
		bucket4 := bucket * 4
		length := int32(meta.count)
		slot := int32(0)

		for ; slot < length; slot++ {
			if (1<<slot)&meta.deleted > 0 {
				// skip deleted
				if firstDeleted < 0 {
					firstDeleted = bucket4 + slot
				}
				continue
			}
			if j.buckets[bucket4+slot] == hash {
				// check value and return slot
				offset := j.bodyOffset[bucket4+slot]
				dataLen := len(j.data)

				if int(offset.keyOffset+offset.keySize) > dataLen {
					firstPart := dataLen - int(offset.keyOffset)
					targetKey := unsafe.String(&j.data[offset.keyOffset], firstPart)

					if key[:firstPart] != targetKey {
						continue
					}

					targetKey = unsafe.String(&j.data[0], int(offset.keySize)-firstPart)

					if key[firstPart:] != targetKey {
						continue
					}
				} else {
					targetKey := unsafe.String(&j.data[offset.keyOffset], offset.keySize)

					if key != targetKey {
						continue
					}
				}

				retOffset = bucket4 + slot
				retCode = FOUND
				return
			}
		}

		if slot < 4 {
			if firstDeleted >= 0 {
				retOffset = firstDeleted
				retCode = DELETED
			} else {
				retOffset = bucket4 + slot
				retCode = EMPTY
			}

			return
		}

		if meta.deleted < 128 {
			// slot full, yet to overflow
			if findSlotToWrite {
				if firstDeleted > 0 {
					retOffset = firstDeleted
					retCode = DELETED
					return
				}
				// mark as overflowed
				meta.deleted = meta.deleted | 128
			} else {
				retCode = OVERFLOWED
				return
			}
		}

		// overflowed. probe next bucket until an empty one is found
		bucket++

		// revolve to start
		if bucket == j.header.buckets {
			bucket = 0
		}
	}

	// scanned all buckets
	if firstDeleted >= 0 && findSlotToWrite {
		retOffset = firstDeleted
		retCode = DELETED
		return
	}

	retCode = FULL
	return
}

func (j *Journal) Delete(key string) (ok bool) {
	hash := HashableString(key).GetHashCode()
	retCode, offset := j.findSlot(key, hash, false)

	ok = retCode == FOUND
	if ok {
		meta := &j.JournalBucketMeta[offset/4]
		meta.deleted = meta.deleted | (1 << (offset % 4))
		bodyOffset := j.bodyOffset[offset]
		prevOffset := bodyOffset.prevOffset
		nextOffset := bodyOffset.nextOffset

		j.bodyOffset[prevOffset].nextOffset = nextOffset
		j.bodyOffset[nextOffset].prevOffset = prevOffset

		if j.header.head == offset {
			j.header.head = nextOffset
		}

		j.header.length--

		if bodyOffset.logOffset >= 0 {
			j.logData[bodyOffset.logOffset] = -1
			bodyOffset.logOffset = -1
		}
	}

	return
}

func (j *Journal) Get(key string) (ok bool, value []byte) {
	hash := HashableString(key).GetHashCode()
	retCode, offset := j.findSlot(key, hash, false)

	ok = retCode == FOUND
	if ok {
		offset := j.bodyOffset[offset]

		dataLen := len(j.data)
		startPos := int(offset.keyOffset + offset.keySize)
		bytesToRead := int(offset.valueSize)

		if startPos+bytesToRead > dataLen {
			value = make([]byte, bytesToRead)

			bytesToRead -= copy(value, j.data[startPos:])
			copy(value[dataLen-startPos:], j.data[:bytesToRead])
		} else {
			value = j.data[startPos:(startPos + bytesToRead)]
		}
	}

	return
}

func (j *Journal) Set(key string, value []byte) error {
	hash := HashableString(key).GetHashCode()

	retCode, currOffset := j.findSlot(key, hash, true)

	if retCode == FULL {
		return fmt.Errorf("Hashmap full")
	}

	if retCode == DELETED {
		meta := &j.JournalBucketMeta[currOffset/4]
		meta.deleted = meta.deleted &^ (1 << (currOffset % 4))
	} else if retCode == EMPTY {
		meta := &j.JournalBucketMeta[currOffset/4]
		meta.count++
	}

	bodyOffset := &j.bodyOffset[currOffset]

	if retCode == FOUND {
		// fix linked list
		prevOffset := bodyOffset.prevOffset
		nextOffset := bodyOffset.nextOffset

		j.bodyOffset[prevOffset].nextOffset = nextOffset
		j.bodyOffset[nextOffset].prevOffset = prevOffset

		if j.header.head == currOffset {
			j.header.head = nextOffset
		}

		if bodyOffset.logOffset >= 0 {
			j.logData[bodyOffset.logOffset] = -1
		}
	} else {
		if j.header.length == 0 {
			j.header.head = currOffset
		}

		j.header.length = j.header.length + 1
	}

	head := &j.bodyOffset[j.header.head]
	lastOffset := &j.bodyOffset[head.prevOffset]
	lastOffset.nextOffset = currOffset

	startPos := int(lastOffset.keyOffset + lastOffset.keySize + lastOffset.valueSize)
	bytesKey := StringToBytes(key)
	dataLen := len(j.data)

	if startPos >= dataLen {
		startPos -= dataLen
	}

	bodyOffset.keyOffset = int32(startPos)
	bodyOffset.keySize = int32(len(bytesKey))
	bodyOffset.valueSize = int32(len(value))
	bodyOffset.prevOffset = head.prevOffset

	for j.header.head != currOffset {
		// if data overlays with head, delete head
		endPos := startPos + int(bodyOffset.keySize+bodyOffset.valueSize)

		if endPos <= int(head.keyOffset) {
			// ----currEnd----headStart----
			// no overlap
			break
		}

		// ----headStart----currEnd----
		if endPos > dataLen {
			// current end revolves
			// -----------headStart---
			// -currEnd----
			endPos -= dataLen

			if endPos <= int(head.keyOffset) && int(head.keyOffset+head.keySize+head.valueSize) <= startPos {
				// -----------headStart---headEnd---currStart--
				// -currEnd----
				break
			}
		} else {
			if startPos >= int(head.keyOffset+head.keySize+head.valueSize) {
				// ----headStart----headEnd-currStart----currEnd----
				break
			}
		}

		// delete head
		meta := &j.JournalBucketMeta[j.header.head/4]
		meta.deleted = meta.deleted | (1 << (j.header.head % 4))
		j.header.head = head.nextOffset
		if head.logOffset >= 0 {
			j.logData[head.logOffset] = -1
			head.logOffset = -1
		}

		head = &j.bodyOffset[j.header.head]
		j.header.length--
	}

	head.prevOffset = currOffset
	bodyOffset.nextOffset = j.header.head

	if startPos+int(bodyOffset.keySize) > dataLen {
		copy(j.data[startPos:], bytesKey[:(dataLen-startPos)])
		copy(j.data, bytesKey[(dataLen-startPos):])
	} else {
		copy(j.data[startPos:], bytesKey)
	}
	startPos += int(bodyOffset.keySize)
	if startPos >= dataLen {
		startPos -= dataLen
	}

	if startPos+int(bodyOffset.valueSize) > dataLen {
		copy(j.data[startPos:], value[:(dataLen-startPos)])
		copy(j.data, value[(dataLen-startPos):])
	} else {
		copy(j.data[startPos:], value)
	}
	j.buckets[currOffset] = hash

	// update journal
	tail := int32(0)

	if j.log.head == -1 {
		j.log.head = 0
	} else if j.log.length == j.log.cap {
		// head will be overwritten
		tail = j.log.head

		headOffset := j.logData[j.log.head]

		if headOffset >= 0 {
			j.bodyOffset[headOffset].logOffset = -1
		}

		j.log.head++
		if j.log.head >= int32(j.log.cap) {
			j.log.head = 0
		}
	} else {
		tail = j.log.head + j.log.length
	}

	if tail >= j.log.cap {
		tail -= j.log.cap
	}

	bodyOffset.logOffset = tail
	j.logData[tail] = currOffset

	if j.log.length < int32(j.log.cap) {
		j.log.length++
	}

	j.log.seq++

	if j.outputCh != nil {
		j.outputCh <- JournalKVP{
			Key:   key,
			Value: value,
			Seq:   j.log.seq,
		}
	}

	return nil
}

func (j *Journal) Len() int {
	return int(j.header.length)
}

func (j *Journal) JournalOffset() int {
	return int(j.log.seq)
}

type JournalLogIterator struct {
	*Journal
	from int64
}

func (j *Journal) LogIter(from int64) JournalLogIterator {
	return JournalLogIterator{
		Journal: j,
		from:    from,
	}
}

func (jli *JournalLogIterator) Next() (ok bool, ret JournalKVP) {
	if jli.from < jli.Journal.log.seq-int64(jli.log.length) {
		jli.from = jli.Journal.log.seq - int64(jli.log.length)
	}

	for {
		if jli.from >= jli.Journal.log.seq {
			ok = false
			return
		}

		offset := int64(jli.log.head+jli.log.length) - jli.Journal.log.seq + jli.from
		if offset >= int64(jli.log.cap) {
			offset -= int64(jli.log.cap)
		}

		bodyOffset := jli.Journal.logData[offset]
		if bodyOffset > 0 {
			bo := jli.bodyOffset[bodyOffset]

			dataLen := len(jli.Journal.data)
			startPos := int(bo.keyOffset)
			bytesToRead := int(bo.keySize)

			if startPos+bytesToRead > dataLen {
				keyBytes := make([]byte, bytesToRead)

				bytesToRead -= copy(keyBytes, jli.data[startPos:])
				copy(keyBytes[dataLen-startPos:], jli.data[:bytesToRead])
				ret.Key = BytesToString(keyBytes)
			} else {
				ret.Key = BytesToString(jli.data[startPos:(startPos + bytesToRead)])
			}

			startPos += int(bo.keySize)
			bytesToRead = int(bo.valueSize)

			if startPos+bytesToRead > dataLen {
				ret.Value = make([]byte, bytesToRead)

				bytesToRead -= copy(ret.Value, jli.data[startPos:])
				copy(ret.Value[dataLen-startPos:], jli.data[:bytesToRead])
			} else {
				ret.Value = jli.data[startPos:(startPos + bytesToRead)]
			}

			ret.Seq = jli.from
			ok = true

			jli.from++
			return
		}

		jli.from++
	}
}

type JournalIterator struct {
	*Journal
	offset int32
}

type JournalKVP struct {
	Key   string
	Value []byte
	Seq   int64
}

func (j *Journal) Iter() JournalIterator {
	return JournalIterator{
		Journal: j,
		offset:  -1,
	}
}

func (ji *JournalIterator) Next() (ok bool, key string, value []byte) {
	if ji.header.length == 0 {
		return
	}

	if ji.offset == -1 {
		// uninitialised
		ji.offset = ji.header.head
	} else if ji.offset == ji.header.head {
		return
	}

	offset := ji.bodyOffset[ji.offset]

	ok = true
	dataLen := len(ji.data)
	startPos := int(offset.keyOffset)
	bytesToRead := int(offset.keySize)

	if startPos+bytesToRead > dataLen {
		key += unsafe.String(&ji.data[startPos], dataLen-startPos)
		bytesToRead -= dataLen - startPos
		startPos = 0
		key += unsafe.String(&ji.data[startPos], bytesToRead)
	} else {
		key = unsafe.String(&ji.data[offset.keyOffset], offset.keySize)
	}

	startPos = int(offset.keyOffset + offset.keySize)
	if startPos >= dataLen {
		startPos -= dataLen
	}

	bytesToRead = int(offset.valueSize)

	if startPos+bytesToRead > dataLen {
		value = make([]byte, offset.valueSize)

		bytesToRead -= copy(value, ji.data[startPos:])
		copy(value[dataLen-startPos:], ji.data[:bytesToRead])
	} else {
		value = ji.data[startPos:(startPos + bytesToRead)]
	}

	ji.offset = offset.nextOffset

	return
}

const hashMapLoadFactor = 0.75

func (j *Journal) Init(headerBuf []byte, JournalSize int, cap int) (headerSize int) {
	buckets := int(float64(cap) / hashMapLoadFactor / 4)

	j.header = (*JournalMapHeader)(unsafe.Pointer(&headerBuf[0]))
	j.JournalBucketMeta = unsafe.Slice((*JournalBucketMeta)(unsafe.Pointer(&headerBuf[16])), buckets)
	j.buckets = unsafe.Slice((*uint64)(unsafe.Pointer(&headerBuf[16+2*buckets])), buckets*4)

	padding := (8 - 34*buckets%8) % 8
	j.bodyOffset = unsafe.Slice((*JournalBodyOffset)(unsafe.Pointer(&headerBuf[16+34*buckets+padding])), buckets*4)
	j.log = (*JournalLogDataHeader)(unsafe.Pointer(&headerBuf[16+130*buckets+padding]))
	padding += 4
	j.logData = unsafe.Slice((*int32)(unsafe.Pointer(&headerBuf[36+130*buckets+padding])), JournalSize)
	if j.header.version != 1 || j.header.buckets != int32(buckets) || j.log.cap != int32(JournalSize) {
		j.header.buckets = int32(buckets)
		j.log.cap = int32(JournalSize)

		// uninitialised
		j.Clear()
	}

	return 36 + 130*buckets + 4*JournalSize + padding
}

func (j *Journal) SetData(buf []byte) {
	j.data = buf
}

func (j *Journal) Output(ch chan JournalKVP) {
	j.outputCh = ch
}

func (j *Journal) Clear() {
	j.header.version = 1
	j.header.head = 0
	j.header.length = 0

	meta := JournalBucketMeta{}
	for i := range j.JournalBucketMeta {
		j.JournalBucketMeta[i] = meta
	}

	j.log.head = -1
	j.log.seq = 1
	j.log.length = 0
}
