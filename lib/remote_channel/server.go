package remotechannel

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

type SendMessage struct {
	Id     uint64
	Data   []byte
	NextId uint64
}

type Server struct {
	memFile *Memfile
}

func (s *Server) Init(indexFile string, dataFile string, offsetFile string) error {
	s.memFile = &Memfile{}

	if err := s.memFile.Init(indexFile, dataFile, offsetFile); err != nil {
		return err
	}

	return nil
}

func ReadAndAck(reader io.Reader, cursor *Cursor, errCh chan error) {
	defer recover()

	var buf [8]byte
	for {
		l, err := io.ReadFull(reader, buf[:])

		if err != nil || l != 8 {
			errCh <- err
			return
		}

		ack := binary.NativeEndian.Uint64(buf[:])
		cursor.Ack(ack)
	}
}

func Sendfile(ctx context.Context, cursor *Cursor, writer io.Writer, errCh chan error) {
	defer recover()
	defer cursor.Close()

	for {
		fd, length, err := cursor.Next(ctx)

		if errors.Is(err, context.Canceled) {
			return
		}

		if err != nil {
			errCh <- err
			return
		}

		if fd == nil {
			continue
		}

		if length > 0 {
			// optimise: use syscall.Sendfile directly
			// https://golangwebforum.com/posts/optimizing-large-file-transfers-linux-go-tcp-syscall-24.html
			f := io.LimitReader(fd, int64(length))
			_, err = io.Copy(writer, f)
		} else {
			_, err = io.Copy(writer, fd)
			fd.Close()
		}

		if err != nil {
			errCh <- err
			return
		}
	}
}

type BufPlusReader struct {
	buf    []byte
	reader io.Reader
	n      int
}

func (bpr *BufPlusReader) Read(p []byte) (n int, err error) {
	if bpr.buf == nil {
		return bpr.reader.Read(p)
	}

	if len(p) == 0 {
		return
	}

	n = copy(p, bpr.buf[bpr.n:])
	if n == 0 {
		bpr.buf = nil
		return bpr.reader.Read(p)
	}
	bpr.n += n
	return
}

func (sv *Server) onConnect(ctx context.Context, conn *net.TCPConn) error {
	defer recover()

	br := bufio.NewReaderSize(conn, 260) // max subscription name 255 bytes

	var s string
	if buf, err := br.ReadSlice('\n'); err != nil {
		conn.Close()
		return err
	} else {
		s = string(buf)
	}

	if ss := strings.Split(s, " "); len(ss) != 2 || ss[0] != "SUB" {
		conn.Close()
		return fmt.Errorf("invalid command %s", s)
	} else {
		subscription := strings.TrimRight(ss[1], "\n")

		buf := make([]byte, br.Buffered())
		if _, err := br.Read(buf); err != nil {
			conn.Close()
			return err
		}
		cursor := sv.memFile.Register(subscription)
		if cursor == nil {
			conn.Close()
			return errors.New("max subscriptions reached")
		}

		end := make(chan error, 2)

		var bpr io.Reader
		if len(buf) == 0 {
			bpr = conn
		} else {
			bpr = &BufPlusReader{
				buf: buf, reader: conn,
			}
		}

		go ReadAndAck(bpr, cursor, end)
		go Sendfile(ctx, cursor, conn, end)

		var err error
		select {
		case <-ctx.Done():
		case err = <-end:
		}

		conn.SetDeadline(time.Now())
		conn.Close()

		return err
	}
}

func loop(ctx context.Context, l *net.TCPListener, onConnect func(*net.TCPConn), ret chan error) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		conn, err := l.AcceptTCP()
		if err != nil {
			ret <- err
			return
		}

		// By default Nagle is turned off
		// conn.SetNoDelay(true)
		onConnect(conn)
	}
}

func (sv *Server) Start(ctx context.Context, tcpAddr string) error {
	defer recover()

	lc := &net.ListenConfig{}
	lc.KeepAlive = 5 * time.Second

	listener, err := lc.Listen(ctx, "tcp4", tcpAddr)
	if err != nil {
		return err
	}

	l := listener.(*net.TCPListener)
	end := make(chan error, 1)

	onConn := func(c *net.TCPConn) {
		go func() {
			if err := sv.onConnect(ctx, c); err != nil {
				println(err.Error())
			}
		}()
	}
	go loop(ctx, l, onConn, end)
	select {
	case <-ctx.Done():
		l.SetDeadline(time.Now())
		l.Close()
		return nil
	case err = <-end:
		l.Close()
		return err
	}
}

func (sv *Server) Send(ctx context.Context, item *DeliveryItem, callback DeliverCallback) error {
	return sv.memFile.Add(item, callback)
}

func (sv *Server) Head() uint64 {
	return *sv.memFile.state.Head
}

func (sv *Server) Close() error {
	return sv.memFile.Close()
}
