package events

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/segmentio/kafka-go"
)

type AuctionEvent struct {
	Timestamp     time.Time `json:"ts"`
	UserID        string    `json:"user_id,omitempty"`
	Geo           string    `json:"geo"`
	Format        string    `json:"format"`
	AdvertiserID  string    `json:"advertiser_id"`
	ClearingPrice float64   `json:"clearing_price"`
}

type Publisher interface {
	Publish(ctx context.Context, e AuctionEvent) error
	Close() error
}

type NoopPublisher struct{}

func (NoopPublisher) Publish(_ context.Context, _ AuctionEvent) error { return nil }
func (NoopPublisher) Close() error                                     { return nil }

// maxKafkaConcurrency bounds the number of in-flight Kafka writes.
// Exceeding this drops the event and returns an error counted by the caller.
const maxKafkaConcurrency = 256

type KafkaPublisher struct {
	writer *kafka.Writer
	sem    chan struct{}
}

func NewKafkaPublisher(brokers []string, topic string) *KafkaPublisher {
	return &KafkaPublisher{
		writer: &kafka.Writer{
			Addr:                   kafka.TCP(brokers...),
			Topic:                  topic,
			Balancer:               &kafka.LeastBytes{},
			AllowAutoTopicCreation: true,
			MaxAttempts:            5,
			WriteTimeout:           10 * time.Second,
			ReadTimeout:            10 * time.Second,
		},
		sem: make(chan struct{}, maxKafkaConcurrency),
	}
}

func (p *KafkaPublisher) Publish(ctx context.Context, e AuctionEvent) error {
	select {
	case p.sem <- struct{}{}:
	default:
		return errors.New("kafka publish queue full")
	}
	defer func() { <-p.sem }()

	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return p.writer.WriteMessages(ctx, kafka.Message{Value: b})
}

func (p *KafkaPublisher) Close() error {
	return p.writer.Close()
}
