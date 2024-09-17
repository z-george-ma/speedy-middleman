package lib

import (
	"lib/assert"
	"strconv"
	"strings"
	"testing"
)

func TestJournal_Perf(t *testing.T) {
	data := make([]byte, 4096*1000)
	var m Journal
	headerSize := m.Init(data, 60000, 30000)
	m.SetData(data[headerSize:])
	m.Clear()

	for j := 0; j < 10000; j++ {
		for i := 0; i < 10000; i++ {
			str := strconv.Itoa(i)
			m.Set(str, []byte(str))
			m.Set("3333", []byte(str))
		}
	}

	for i := 0; i < 10000; i++ {
		str := strconv.Itoa(i)
		m.Delete(str)
	}

	assert.Equal(t, 0, m.Len())
}

func TestJournal_Normal_Operation(t *testing.T) {
	data := make([]byte, 4096*1000)
	var m Journal
	headerSize := m.Init(data, 60000, 30000)
	m.SetData(data[headerSize:])

	m.Set("abcd", []byte("abcd"))
	m.Set("abcd", []byte("def"))
	ok, _ := m.Get("abc")
	assert.Equal(t, false, ok)

	ok, value := m.Get("abcd")
	assert.Equal(t, true, ok)
	assert.Equal(t, "def", string(value))

	assert.Equal(t, 1, m.Len())

	m.Set("abcd1", []byte("def1"))

	ok, value = m.Get("abcd")
	assert.Equal(t, true, ok)
	assert.Equal(t, "def", string(value))

	assert.Equal(t, 2, m.Len())

	m.Set("abcd1", []byte("def2"))
	assert.Equal(t, 2, m.Len())

	m.Delete("abcdef")
	assert.Equal(t, 2, m.Len())

	ok, value = m.Get("abcd1")
	assert.Equal(t, true, ok)
	assert.Equal(t, "def2", string(value))

	m.Delete("abcd")
	assert.Equal(t, 1, m.Len())
	ok, _ = m.Get("abcd")
	assert.Equal(t, false, ok)
}

func TestJournal_Iter(t *testing.T) {
	data := make([]byte, 4096*1000)
	var m Journal
	headerSize := m.Init(data, 60000, 30000)
	m.SetData(data[headerSize:])

	values := []string{"abc", "def", "abc", "efg"}

	for _, v := range values {
		m.Set(v, []byte(v))
	}
	assert.Equal(t, 3, m.Len())

	i := m.Iter()

	ok, k, v := i.Next()
	assert.Equal(t, true, ok)
	assert.Equal(t, "def", k)
	assert.Equal(t, "def", string(v))

	ok, k, v = i.Next()
	assert.Equal(t, true, ok)
	assert.Equal(t, "abc", k)
	assert.Equal(t, "abc", string(v))

	ok, k, v = i.Next()
	assert.Equal(t, true, ok)
	assert.Equal(t, "efg", k)
	assert.Equal(t, "efg", string(v))

	ok, k, v = i.Next()
	assert.Equal(t, false, ok)
}

func TestJournal_Rotate(t *testing.T) {
	data := make([]byte, 190)
	var m Journal
	headerSize := m.Init(data, 1, 4)
	m.SetData(data[headerSize:])

	m.Set("12", []byte("123"))
	m.Set("23", []byte("234"))
	m.Set("34", []byte("345"))
	assert.Equal(t, 2, m.Len())

	ok, value := m.Get("12")
	assert.Equal(t, false, ok)

	ok, value = m.Get("23")
	assert.Equal(t, true, ok)
	assert.Equal(t, "234", string(value))

	ok, value = m.Get("34")
	assert.Equal(t, true, ok)
	assert.Equal(t, "345", string(value))

	m.Set("abc", []byte("abc"))
	assert.Equal(t, 1, m.Len())

	m.Set("de", []byte("de"))
	assert.Equal(t, 2, m.Len())

	ok, value = m.Get("abc")
	assert.Equal(t, true, ok)
	assert.Equal(t, "abc", string(value))

	ok, value = m.Get("de")
	assert.Equal(t, true, ok)
	assert.Equal(t, "de", string(value))
}

func TestJournal_Logs(t *testing.T) {
	data := make([]byte, 226)
	var m Journal
	headerSize := m.Init(data, 10, 4)
	m.SetData(data[headerSize:])
	m.Output(make(chan JournalKVP, 10))

	m.Set("1", []byte("1"))
	m.Set("2", []byte("2"))
	m.Set("3", []byte("3"))

	m.Delete("2")

	m.Set("4", []byte("4"))
	m.Set("5", []byte("56")) // should overwrite 1
	m.Set("4", []byte("5"))

	assert.Equal(t, 7, m.JournalOffset())

	var values []string
	for v := range m.outputCh {
		values = append(values, v.Key)

		if len(values) == 6 {
			break
		}
	}

	assert.Equal(t, "123454", strings.Join(values, ""))

	i := m.LogIter(1)
	ok, ret := i.Next()
	assert.Equal(t, true, ok)
	assert.Equal(t, "3", ret.Key)

	ok, ret = i.Next()
	assert.Equal(t, true, ok)
	assert.Equal(t, "5", ret.Key)

	ok, ret = i.Next()
	assert.Equal(t, true, ok)
	assert.Equal(t, "4", ret.Key)

	ok, ret = i.Next()
	assert.Equal(t, false, ok)
}

func TestJournal_LogRotate(t *testing.T) {
	data := make([]byte, 226)
	var m Journal
	headerSize := m.Init(data, 3, 4)
	m.SetData(data[headerSize:])
	m.Output(make(chan JournalKVP, 10))

	m.Set("1", []byte("1"))
	m.Set("2", []byte("2"))
	m.Set("3", []byte("3"))
	m.Set("4", []byte("4"))
	m.Delete("3")
	m.Set("5", []byte("5"))

	i := m.LogIter(1)
	ok, ret := i.Next()
	assert.Equal(t, true, ok)
	assert.Equal(t, "4", ret.Key)

	ok, ret = i.Next()
	assert.Equal(t, true, ok)
	assert.Equal(t, "5", ret.Key)

	ok, ret = i.Next()
	assert.Equal(t, false, ok)
}
