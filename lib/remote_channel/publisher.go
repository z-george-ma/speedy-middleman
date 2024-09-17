package remotechannel

import (
	"context"
)

type Publisher struct {
	buf      chan *DeliveryItem
	ser      func(item any) ([]byte, error)
	sender   ISender
	callback DeliverCallback
}

func (p *Publisher) Init(sender ISender, ser func(item any) ([]byte, error), sendBufferSize int, deliveryReport chan *DeliveryItem) {
	p.sender = sender
	p.ser = ser
	p.buf = make(chan *DeliveryItem, sendBufferSize)

	if deliveryReport != nil {
		p.callback = func(item *DeliveryItem) {
			deliveryReport <- item
		}
	}
}

func (p *Publisher) Send(item any, opaque any) *DeliveryItem {
	ret := &DeliveryItem{
		Opaque: opaque,
		Ready:  make(chan error, 1),
	}
	if buf, err := p.ser(item); err != nil {
		ret.Ready <- err
		if p.callback != nil {
			p.callback(ret)
		}
	} else {
		ret.Data = buf
		// could panic if buf is closed
		p.buf <- ret
	}

	return ret
}

func (p *Publisher) Start(ctx context.Context) error {
	id := p.sender.Head() + 1
	for {
		var ok bool
		var item *DeliveryItem
		select {
		case <-ctx.Done():
			return p.Close()
		case item, ok = <-p.buf:
		}

		if !ok {
			return nil
		}

		item.Id = id
		if err := p.sender.Send(ctx, item, p.callback); err != nil {
			// unrecoverable error
			break
		}
		id++
	}

	return p.Close()
}

func (p *Publisher) Close() error {
	close(p.buf)
	return nil
}
