package events

import (
	"context"
	"encoding/json"
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

type KafkaPublisher struct {
	writer *kafka.Writer
}

func NewKafkaPublisher(brokers []string, topic string) *KafkaPublisher {
	return &KafkaPublisher{
		writer: &kafka.Writer{
			Addr:                   kafka.TCP(brokers...),
			Topic:                  topic,
			Balancer:               &kafka.LeastBytes{},
			AllowAutoTopicCreation: true,
		},
	}
}

func (p *KafkaPublisher) Publish(ctx context.Context, e AuctionEvent) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return p.writer.WriteMessages(ctx, kafka.Message{Value: b})
}

func (p *KafkaPublisher) Close() error {
	return p.writer.Close()
}
