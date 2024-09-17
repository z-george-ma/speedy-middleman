package repl

import (
	"container/list"
	"context"
	"encoding/binary"
	"lib"
	"sync"
	"time"

	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"
)

type LSNTracker struct {
	lock         sync.Mutex
	keepAliveLSN pglogrepl.LSN
	lastAckedLSN pglogrepl.LSN
	lsnList      *list.List
	lsnMap       map[uint64]*list.Element
}

func (lt *LSNTracker) SetKeepAliveLSN(lsn pglogrepl.LSN) {
	lt.lock.Lock()
	lt.keepAliveLSN = lsn
	lt.lock.Unlock()
}

func (lt *LSNTracker) AddLSN(lsn pglogrepl.LSN) {
	lt.lock.Lock()

	elem := lt.lsnList.PushBack(lsn)
	lt.lsnMap[uint64(lsn)] = elem

	lt.lock.Unlock()
}

func (lt *LSNTracker) CompleteLSN(lsn pglogrepl.LSN) {
	lt.lock.Lock()

	l := uint64(lsn)
	if elem, ok := lt.lsnMap[l]; ok {
		delete(lt.lsnMap, l)
		f := lt.lsnList.Front()
		if f.Value == elem.Value {
			lt.lastAckedLSN = lsn
		}
		lt.lsnList.Remove(elem)
	}
	lt.lock.Unlock()
}

func (lt *LSNTracker) GetLSN() (ret pglogrepl.LSN) {
	lt.lock.Lock()
	if lt.lsnList.Len() == 0 {
		ret = lt.keepAliveLSN
	} else {
		ret = lt.lastAckedLSN
	}
	lt.lock.Unlock()
	return ret
}

type PgRepl struct {
	PluginArguments           []string
	OnRelation                func(*pglogrepl.RelationMessage)
	OnSendStandbyStatusUpdate func(pglogrepl.LSN)
	Messages                  chan *PgReplMessage

	*pgconn.PgConn
	IdentifySystemResult pglogrepl.IdentifySystemResult

	relations map[uint32]*pglogrepl.RelationMessage
	tracker   *LSNTracker
}

type PgReplMessage struct {
	WALStart     uint64
	ServerWALEnd uint64
	ServerTime   time.Time

	Type       pglogrepl.MessageType
	RelationId uint32
	Data       any
	RefCount   int
}

func (pg *PgRepl) Connect(ctx context.Context, source string, slotName string) error {
	conn, err := pgconn.Connect(ctx, source)

	if err != nil {
		return err
	}
	pg.PgConn = conn

	if err != nil {
		return err
	}

	_, err = pglogrepl.CreateReplicationSlot(ctx, conn, slotName, "pgoutput", pglogrepl.CreateReplicationSlotOptions{})

	if err != nil {
		e, ok := err.(*pgconn.PgError)

		if !ok {
			return err
		}

		if e.Code != "42710" {
			return err
		}
		// slot already exists
	}

	si, err := pglogrepl.IdentifySystem(ctx, conn)

	if err != nil {
		return err
	}
	pg.IdentifySystemResult = si

	return nil
}

type PgReplErrorResponse struct {
	*pgproto3.ErrorResponse
}

type PgReplUnknownType struct {
}

func (pg PgReplErrorResponse) Error() string {
	return pg.Message
}

func (pg PgReplUnknownType) Error() string {
	return "Unknown response"
}

func (pg *PgRepl) Complete(m ...*PgReplMessage) {
	for _, msg := range m {
		if msg.RefCount == 0 {
			pg.tracker.CompleteLSN(pglogrepl.LSN(msg.WALStart))
		} else {
			msg.RefCount--
		}
	}
}

func (pg *PgRepl) InflightCount() int {
	return pg.tracker.lsnList.Len()
}

func (pg *PgRepl) Start(ctx context.Context, slotName string, startLSN uint64, standbyUpdateDeadline time.Duration, deser func(*pglogrepl.RelationMessage, pglogrepl.MessageType, pglogrepl.XLogData) *PgReplMessage) error {
	defer close(pg.Messages)
	pg.relations = make(map[uint32]*pglogrepl.RelationMessage)
	pg.tracker = &LSNTracker{
		lsnList: list.New(),
		lsnMap:  map[uint64]*list.Element{},
	}

	ch := lib.Channel[*PgReplMessage](pg.Messages)

	err := pglogrepl.StartReplication(ctx, pg.PgConn, slotName, pglogrepl.LSN(startLSN), pglogrepl.StartReplicationOptions{
		PluginArgs: pg.PluginArguments,
	})

	if err != nil {
		return err
	}

	standbyDeadline := time.Now().Add(standbyUpdateDeadline)

	for !lib.IsDone(ctx) {
		if time.Now().After(standbyDeadline) {
			lsn := pg.tracker.GetLSN()
			err := pglogrepl.SendStandbyStatusUpdate(ctx, pg.PgConn, pglogrepl.StandbyStatusUpdate{
				WALWritePosition: lsn,
			})

			if pg.OnSendStandbyStatusUpdate != nil {
				pg.OnSendStandbyStatusUpdate(lsn)
			}

			if err != nil {
				return err
			}

			standbyDeadline = time.Now().Add(standbyUpdateDeadline)
		}

		recvCtx, cancel := context.WithDeadline(ctx, standbyDeadline)
		rawMsg, err := pg.PgConn.ReceiveMessage(recvCtx)
		cancel()
		if err != nil {
			if pgconn.Timeout(err) {
				continue
			}
			return err
		}

		if r, ok := rawMsg.(*pgproto3.ErrorResponse); ok {
			return PgReplErrorResponse{r}
		}

		msg, ok := rawMsg.(*pgproto3.CopyData)
		if !ok {
			// Received unexpected message. skipping.
			continue
		}

		switch msg.Data[0] {
		case pglogrepl.PrimaryKeepaliveMessageByteID:
			pkm, err := pglogrepl.ParsePrimaryKeepaliveMessage(msg.Data[1:])
			if err != nil {
				return err
			}

			if pkm.ReplyRequested {
				standbyDeadline = time.Time{}
			}

			pg.tracker.SetKeepAliveLSN(pkm.ServerWALEnd)
			continue
		case pglogrepl.XLogDataByteID:
			xld, err := pglogrepl.ParseXLogData(msg.Data[1:])
			if err != nil {
				return err
			}
			msgType := pglogrepl.MessageType(xld.WALData[0])

			if msgType == pglogrepl.MessageTypeRelation {
				m, err := pglogrepl.Parse(xld.WALData)
				if err != nil {
					return err
				}
				rel := m.(*pglogrepl.RelationMessage)
				pg.relations[rel.RelationID] = rel
				if pg.OnRelation != nil {
					pg.OnRelation(rel)
				}
				continue
			}

			switch msgType {
			case pglogrepl.MessageTypeBegin, pglogrepl.MessageTypeCommit:
				continue
			case pglogrepl.MessageTypeInsert, pglogrepl.MessageTypeUpdate, pglogrepl.MessageTypeDelete, pglogrepl.MessageTypeTruncate:
				relationId := binary.BigEndian.Uint32(xld.WALData[1:5])
				pg.tracker.AddLSN(xld.WALStart)
				ch.Send(deser(pg.relations[relationId], msgType, xld), ctx)
			}
		default:
			// unknown type
			return PgReplUnknownType{}
		}
	}

	return nil
}
