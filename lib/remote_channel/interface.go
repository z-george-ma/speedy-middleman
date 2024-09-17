/*
Package remotechannel is a library to achieve pubsub between application
*/
package remotechannel

import "context"

type DeliveryItem struct {
	Id     uint64
	Opaque any
	Data   []byte
	Ready  chan error
}

type DeliverCallback = func(*DeliveryItem)

type ISender interface {
	Send(ctx context.Context, item *DeliveryItem, callback DeliverCallback) error
	Head() uint64
}
