package remotechannel

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

type AckMessage struct {
	id    uint64
	ready chan error
}

type Subscriber struct {
	conn  *net.TCPConn
	ackCh chan AckMessage
}

func (sub *Subscriber) Init(addr string, subName string) error {
	dialer := &net.Dialer{}
	conn, err := dialer.Dial("tcp", addr)

	if err != nil {
		return err
	}

	sub.ackCh = make(chan AckMessage, 100)
	sub.conn = conn.(*net.TCPConn)
	// default nagle off
	// sub.conn.SetNoDelay(true)
	sub.conn.SetKeepAlive(true)
	sub.conn.SetKeepAlivePeriod(5 * time.Second)

	subCommand := fmt.Sprintf("SUB %s\n", subName)
	_, err = sub.conn.Write([]byte(subCommand))

	return err
}

func ReadMessage(reader io.Reader, callback func(id uint64, data []byte) error, errCh chan error) {
	defer recover()

	var buf [12]byte
	for {
		l, err := io.ReadFull(reader, buf[:])

		if err != nil || l != len(buf) {
			errCh <- err
			return
		}

		id := binary.NativeEndian.Uint64(buf[:8])
		length := int(binary.NativeEndian.Uint32(buf[8:]))

		data := make([]byte, length)
		l, err = io.ReadFull(reader, data)

		if err != nil || l != length {
			errCh <- err
			return
		}

		if err = callback(id, data); err != nil {
			errCh <- err
			return
		}
	}
}

func SendAck(writer io.Writer, ch chan AckMessage, errCh chan error) {
	defer recover()

	for m := range ch {
		err := binary.Write(writer, binary.NativeEndian, m.id)
		m.ready <- err

		if err != nil {
			errCh <- err
			return
		}
	}
}

type Message struct {
	Data any
	id   uint64
	sub  *Subscriber
}

func (sub *Subscriber) Subscribe(ctx context.Context, deser func([]byte) (any, error), ch chan Message) (err error) {
	end := make(chan error, 2)

	go ReadMessage(sub.conn, func(id uint64, data []byte) error {
		if d, err := deser(data); err != nil {
			return err
		} else {
			ch <- Message{
				Data: d,
				id:   id,
				sub:  sub,
			}
			return nil
		}
	}, end)

	go SendAck(sub.conn, sub.ackCh, end)

	select {
	case <-ctx.Done():
	case err = <-end:
	}

	sub.conn.SetDeadline(time.Now())
	sub.conn.Close()
	close(sub.ackCh)

	return err
}

func (sub Message) Ack() chan error {
	return sub.sub.ack(sub.id)
}

func (sub *Subscriber) ack(id uint64) chan error {
	ch := make(chan error, 1)
	sub.ackCh <- AckMessage{
		id, ch,
	}
	return ch
}
