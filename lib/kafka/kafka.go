package kafka

import (
	"context"
	"errors"
	"fmt"
	"lib"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

type KafkaConfig struct {
	BootstrapServers string

	// Optional
	SecurityProtocol string // default to plaintext. if saslUsername exists, default to sasl_ssl
	SaslMechanism    string
	SaslUsername     string
	SaslPassword     string
	MaxMessageBytes  int
	MaxPollInterval  int
}

type KafkaConsumer struct {
	*kafka.Consumer
}

type KafkaProducer struct {
	*kafka.Producer
}

func ensureProtocol(config *KafkaConfig) {
	if config.SecurityProtocol == "" {
		if config.SaslUsername != "" {
			config.SecurityProtocol = "sasl_ssl"
		} else {
			config.SecurityProtocol = "plaintext"
		}
	}

	if config.SecurityProtocol == "sasl_ssl" && config.SaslMechanism == "" {
		config.SaslMechanism = "PLAIN"
	}
}

func NewKafkaProducer(config *KafkaConfig, additionalConfig lib.M) (ret *KafkaProducer, err error) {
	ensureProtocol(config)

	configMap := kafka.ConfigMap{
		"bootstrap.servers": config.BootstrapServers, // alloc
		"security.protocol": config.SecurityProtocol, // alloc
	}

	if config.MaxMessageBytes != 0 {
		configMap["message.max.bytes"] = config.MaxMessageBytes // alloc
	}

	if config.SaslUsername != "" {
		configMap["sasl.mechanisms"] = config.SaslMechanism // alloc
		configMap["sasl.username"] = config.SaslUsername    // alloc
		configMap["sasl.password"] = config.SaslPassword    // alloc
	}

	if additionalConfig != nil {
		for k, v := range additionalConfig {
			configMap[k] = v
		}
	}
	p, err := kafka.NewProducer(&configMap)

	if err != nil {
		return
	}

	ret = &KafkaProducer{
		Producer: p,
	}
	return
}

func (kc *KafkaConsumer) Connect(config *KafkaConfig, additionalConfig lib.M, groupId string) (err error) {
	ensureProtocol(config)

	configMap := kafka.ConfigMap{
		"bootstrap.servers":       config.BootstrapServers, // alloc
		"security.protocol":       config.SecurityProtocol, // alloc
		"group.id":                groupId,                 // alloc
		"enable.auto.commit":      false,                   // alloc
		"socket.keepalive.enable": true,                    // alloc
	}

	if config.MaxMessageBytes != 0 {
		configMap["message.max.bytes"] = config.MaxMessageBytes // alloc
	}

	if config.SaslUsername != "" {
		configMap["sasl.mechanisms"] = config.SaslMechanism // alloc
		configMap["sasl.username"] = config.SaslUsername    // alloc
		configMap["sasl.password"] = config.SaslPassword    // alloc
	}

	if config.MaxPollInterval != 0 {
		configMap["max.poll.interval.ms"] = config.MaxPollInterval
	}

	if additionalConfig != nil {
		for k, v := range additionalConfig {
			configMap[k] = v
		}
	}

	c, err := kafka.NewConsumer(&configMap)

	if err != nil {
		return
	}

	kc.Consumer = c
	return
}

type ErrorCallback func(error)

func (kc *KafkaConsumer) Assign(ctx context.Context, topic string, partitions []int32, ch chan *kafka.Message, errCallback ...ErrorCallback) error {
	topicPartitions := make([]kafka.TopicPartition, len(partitions)) // alloc
	for i, partition := range partitions {
		topicPartitions[i] = kafka.TopicPartition{Topic: &topic, Partition: partition, Offset: kafka.OffsetStored}
	}
	err := kc.Consumer.Assign(topicPartitions)

	if err != nil {
		return err
	}

	go func() { // alloc
		defer close(ch)
		for {
			msg, err := kc.Consumer.ReadMessage(time.Second)
			if err == nil {
				select {
				case ch <- msg:
					continue
				case <-ctx.Done():
					return
				}
			} else if !err.(kafka.Error).IsTimeout() {
				if len(errCallback) > 0 {
					errCallback[0](err)
				}
				return
			}
		}
	}()

	return nil
}

type KafkaMessage struct {
	Value      any
	Message    *kafka.Message
	Opaque     any
	ReceivedAt time.Time
}

func (kc *KafkaConsumer) subscribe(ctx context.Context, deser func(*kafka.Message) (KafkaMessage, error), ch chan KafkaMessage, errCh chan error) {
	defer func() {
		if e := recover(); e != nil {
			if err, ok := e.(error); ok {
				errCh <- err
			} else {
				errCh <- fmt.Errorf("%v", e)
			}
		}
	}()

	for !lib.IsDone(ctx) {
		m, err := kc.Consumer.ReadMessage(time.Second)

		if err == nil {
			ch <- lib.Must(deser(m))
			continue
		}

		if e, ok := err.(kafka.Error); ok && e.IsTimeout() {
			continue
		}

		errCh <- err
		return
	}

	errCh <- nil
}

func (kc *KafkaConsumer) Subscribe(ctx context.Context, topic string, deser func(*kafka.Message) (KafkaMessage, error), ch chan KafkaMessage) error {
	defer close(ch)
	errCh := make(chan error, 2)

	err := kc.Consumer.Subscribe(topic, func(c *kafka.Consumer, e kafka.Event) error {
		switch e.(type) {
		case kafka.RevokedPartitions:
			lib.Log.Warn().Str("type", fmt.Sprintf("%T", e)).Msg("Kafka partitions revoked")
			errCh <- errors.New("kafka partitions revoked")
		}
		return nil
	})

	if err != nil {
		return err
	}

	go kc.subscribe(ctx, deser, ch, errCh)

	return <-errCh
}
