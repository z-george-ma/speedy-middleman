package gcp_pubsub

/*
 Package gcp_pubsub provides functionality for connecting and consuming google pubsub messages
*/

import (
	"context"
	"errors"
	"lib"
	"time"

	"cloud.google.com/go/pubsub"
)

type DeliveryReport struct {
	*pubsub.Message
	Error  error
	Opaque any
}

type PubsubClient struct {
	*pubsub.Client
	ch     lib.MultiChannel[struct{}, publishContext]
	Report chan DeliveryReport
}

type publishContext struct {
	*pubsub.PublishResult
	pubsub.Message
	opaque any
}

func NewPubsubClient(ctx context.Context, projectId string) (ret *PubsubClient, err error) {
	client, err := pubsub.NewClient(ctx, projectId)

	if err == nil {
		ret = &PubsubClient{
			Client: client,
			Report: make(chan DeliveryReport),
		}
		ret.ch.Init(func(ok bool, value struct{}, pctx publishContext) {
			_, err := pctx.PublishResult.Get(ctx)

			ret.Report <- DeliveryReport{
				Message: &pctx.Message,
				Error:   err,
				Opaque:  pctx.opaque,
			}
		}, 1)
	}

	return
}

func (pc *PubsubClient) Close() error {
	pc.ch.Close()
	return pc.Client.Close()
}

func (pc *PubsubClient) TrackDelivery(msg pubsub.Message, result *pubsub.PublishResult, opaque any) {
	pc.ch.AddSingle(result.Ready(), publishContext{
		PublishResult: result,
		Message:       msg,
		opaque:        opaque,
	})
}

func (pc *PubsubClient) Publish(ctx context.Context, topic *pubsub.Topic, msg pubsub.Message, opaque any) {
	r := topic.Publish(ctx, &msg)
	pc.TrackDelivery(msg, r, opaque)
}

type PubsubMessage struct {
	Value      any
	Message    *pubsub.Message
	Opaque     any
	ReceivedAt time.Time
}

func Subscribe(ctx context.Context, sub *pubsub.Subscription, deser func(*pubsub.Message) (PubsubMessage, error), ch chan PubsubMessage) error {
	newCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	defer close(ch)

	err := sub.Receive(newCtx, func(c context.Context, msg *pubsub.Message) {
		data, err := deser(msg)
		data.ReceivedAt = time.Now()

		if err != nil {
			cancel(err)
			return
		}

		select {
		case ch <- data:
		case <-c.Done():
		}
	})

	if err == nil {
		err = context.Cause(newCtx)
	} else if errors.Is(err, context.Canceled) {
		err = errors.Unwrap(err)
	}

	return err
}

func GetOrCreateSubscription(ctx context.Context, client *pubsub.Client, topicName string, subscriptionId string, enableMessageOrdering bool) (*pubsub.Subscription, error) {
	subscription := client.Subscription(subscriptionId)

	// Return subscription if it exists
	exists, err := subscription.Exists(ctx)
	if err != nil {
		return nil, err
	}

	if exists {
		return subscription, nil
	}

	// Create and return the new subscription if it doesn't exist.
	topic := client.Topic(topicName)
	subscription, err = client.CreateSubscription(ctx, subscriptionId, pubsub.SubscriptionConfig{
		Topic:                 topic,
		EnableMessageOrdering: enableMessageOrdering,
	})
	if err != nil {
		return nil, err
	}

	return subscription, nil
}

func GetOrCreateTopic(ctx context.Context, client *pubsub.Client, topicName string) (*pubsub.Topic, error) {

	topic := client.Topic(topicName)
	exists, err := topic.Exists(ctx)
	if err != nil {
		return nil, err
	}

	if exists {
		return topic, nil
	}
	// Topic doesn't exist
	topic, err = client.CreateTopic(ctx, topicName)

	if err != nil {
		return nil, err
	}

	return topic, nil

}
