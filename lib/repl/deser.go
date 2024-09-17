package repl

import (
	"encoding/binary"
	"lib"
	"reflect"
	"time"

	"github.com/jackc/pglogrepl"
)

const SQLTIMEFORMAT = "2006-01-02 15:04:05"
const SQLTIME2FORMAT = "2006-01-02 15:04:05.999999-07"

func WALDataScan(data []byte, a ...any) {
	n := 0

	for _, d := range a {
		tupleDataType := data[n]
		n++

		if tupleDataType != pglogrepl.TupleDataTypeText && tupleDataType != pglogrepl.TupleDataTypeBinary {
			// tuple null or toast
			if d == nil {
				continue
			}

			switch p := d.(type) {
			case *int:
				*p = 0
			case *uint:
				*p = 0
			case *float64:
				*p = 0
			case *bool:
				*p = false
			case *time.Time:
				*p = time.Time{}
			case *string:
				*p = ""
			case *[]byte:
				*p = []byte{}
			case **int:
				*p = nil
			case **uint:
				*p = nil
			case **float64:
				*p = nil
			case **bool:
				*p = nil
			case **time.Time:
				*p = nil
			case **string:
				*p = nil
			case **[]byte:
				*p = nil
			}
			continue
		}

		tupleDataLength := int(binary.BigEndian.Uint32(data[n:]))
		n += 4

		s := data[n : n+tupleDataLength]
		n += tupleDataLength

		if d == nil {
			continue
		}

		switch p := d.(type) {
		case *int:
			*p = lib.Cast[int](s)
		case *uint:
			*p = lib.Cast[uint](s)
		case *float64:
			*p = lib.Cast[float64](s)
		case *bool:
			*p = lib.Cast[bool](s)
		case *time.Time:
			t, err := time.Parse(SQLTIME2FORMAT, lib.BytesToString(s))
			if err != nil {
				t, _ = time.Parse(SQLTIMEFORMAT, lib.BytesToString(s))
			}
			*p = t
		case *string:
			*p = string(s)
		case *[]byte:
			t := make([]byte, len(s))
			copy(t, s)
			*p = t
		case **int:
			*p = lib.Cast[*int](s)
		case **uint:
			*p = lib.Cast[*uint](s)
		case **float64:
			*p = lib.Cast[*float64](s)
		case **bool:
			*p = lib.Cast[*bool](s)
		case **time.Time:
			t, err := time.Parse(SQLTIME2FORMAT, lib.BytesToString(s))
			if err != nil {
				t, _ = time.Parse(SQLTIMEFORMAT, lib.BytesToString(s))
			}
			*p = &t
		case **string:
			// clone the source slice
			t := string(s)
			*p = &t
		case **[]byte:
			// clone the source slice
			t := make([]byte, len(s))
			copy(t, s)
			*p = &t
		}
	}
}

type model interface {
	Fields() map[string]any
}

type Model[T any] interface {
	model
	*T
}

type Deser map[string]reflect.Type

func Register[T any, TModel Model[T]](deser Deser, relation string) {
	var zero [0]T
	deser[relation] = reflect.TypeOf(zero).Elem()
}

func (ds Deser) Deserialise(rel *pglogrepl.RelationMessage, msgType pglogrepl.MessageType, xld pglogrepl.XLogData) *PgReplMessage {
	if msgType != pglogrepl.MessageTypeInsert && msgType != pglogrepl.MessageTypeDelete && msgType != pglogrepl.MessageTypeUpdate {
		// truncate
		return &PgReplMessage{
			WALStart:     uint64(xld.WALStart),
			ServerWALEnd: uint64(xld.ServerWALEnd),
			ServerTime:   xld.ServerTime,
			Type:         msgType,
			RelationId:   0,
			Data:         nil,
		}
	}

	typ, ok := ds[rel.RelationName]

	if !ok {
		return &PgReplMessage{
			WALStart:     uint64(xld.WALStart),
			ServerWALEnd: uint64(xld.ServerWALEnd),
			ServerTime:   xld.ServerTime,
			Type:         msgType,
			RelationId:   rel.RelationID,
			Data:         nil,
		}
	}

	t := reflect.New(typ).Elem().Addr()

	fields := t.Interface().(model).Fields()

	ret := make([]any, rel.ColumnNum)
	for i, c := range rel.Columns {
		v, ok := fields[c.Name]

		if ok {
			ret[i] = v
		} else {
			ret[i] = nil
		}
	}

	WALDataScan(xld.WALData[8:], ret...)

	return &PgReplMessage{
		WALStart:     uint64(xld.WALStart),
		ServerWALEnd: uint64(xld.ServerWALEnd),
		ServerTime:   xld.ServerTime,
		Type:         msgType,
		RelationId:   rel.RelationID,
		Data:         t.Interface(),
	}
}
