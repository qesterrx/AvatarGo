package broker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/qesterrx/AvatarGo/internal/config"
	"github.com/qesterrx/AvatarGo/internal/logger"
	"github.com/qesterrx/AvatarGo/internal/models"

	amqp "github.com/rabbitmq/amqp091-go" // AMQP-клиент
)

type RabbitMQ struct {
	conn *amqp.Connection

	chUnload *amqp.Channel
	chDelete *amqp.Channel

	exchange    string
	queueUnload string
	rkeyUnload  string
	queueDelete string
	rkeyDelete  string
}

func NewRabbitMq(cfg *config.RabbitConfig) (*RabbitMQ, error) {
	// Подключаемся к RabbitMQ
	rabbitConnStr := fmt.Sprintf("amqp://%s:%s@%s:%d/",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
	)

	exchange := fmt.Sprintf("%s.direct", cfg.Queue)
	queueUnload := fmt.Sprintf("%s.unload", cfg.Queue)
	queueDelete := fmt.Sprintf("%s.delete", cfg.Queue)
	rkeyUnload := queueUnload
	rkeyDelete := queueDelete

	conn, err := amqp.Dial(rabbitConnStr)
	if err != nil {
		return nil, err
	}

	chUnload, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("NewRabbitMq: Error creating channel 'unload': %v", err)
	}

	// Объявляем exchange
	if err := chUnload.ExchangeDeclare(exchange, "direct", true, false, false, false, nil); err != nil {
		return nil, fmt.Errorf("NewRabbitMq: Error creating exchange 'unload': %v", err)
	}

	// Объявляем очередь unload
	_, err = chUnload.QueueDeclare(queueUnload, true, false, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("NewRabbitMq: Error creating queue 'unload': %v", err)
	}

	// Привязываем очередь к exchange
	if err := chUnload.QueueBind(queueUnload, rkeyUnload, exchange, false, nil); err != nil {
		return nil, fmt.Errorf("NewRabbitMq: Error binding 'unload': %v", err)
	}

	// Настройка QoS
	if err := chUnload.Qos(1, 0, false); err != nil {
		return nil, fmt.Errorf("NewRabbitMq: Error set Qos 'unload': %v", err)
	}

	chDelete, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("NewRabbitMq: Error creating channel 'delete': %v", err)
	}

	// Объявляем exchange
	if err := chDelete.ExchangeDeclare(exchange, "direct", true, false, false, false, nil); err != nil {
		return nil, fmt.Errorf("NewRabbitMq: Error creating exchange 'delete': %v", err)
	}

	// Объявляем очередь delete
	_, err = chDelete.QueueDeclare(queueDelete, true, false, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("NewRabbitMq: Error creating queue 'delete': %v", err)
	}

	// Привязываем очередь к exchange
	if err := chDelete.QueueBind(queueDelete, rkeyDelete, exchange, false, nil); err != nil {
		return nil, fmt.Errorf("NewRabbitMq: Error binding 'delete': %v", err)
	}

	// Настройка QoS
	if err := chDelete.Qos(1, 0, false); err != nil {
		return nil, fmt.Errorf("NewRabbitMq: Error set Qos 'delete': %v", err)
	}

	rbt := RabbitMQ{
		conn:        conn,
		chUnload:    chUnload,
		chDelete:    chDelete,
		queueUnload: queueUnload,
		rkeyUnload:  rkeyUnload,
		queueDelete: queueDelete,
		rkeyDelete:  rkeyDelete,
		exchange:    exchange,
	}

	return &rbt, nil
}

func (rbt *RabbitMQ) Close() {
	rbt.chDelete.Close()
	rbt.chUnload.Close()
	rbt.conn.Close()
}

func (rbt *RabbitMQ) PublishUnloadEvent(event *models.AvatarUploadEvent) error {

	msg, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return rbt.chUnload.Publish(
		rbt.exchange,
		rbt.rkeyUnload,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        msg,
		},
	)
}

func (rbt *RabbitMQ) PublishDeleteEvent(event *models.AvatarDeleteEvent) error {

	msg, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return rbt.chDelete.Publish(
		rbt.exchange,
		rbt.rkeyDelete,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        msg,
		},
	)
}

func (rbt *RabbitMQ) GetChUnloadEvent(ctx context.Context) (<-chan *models.AvatarUploadEventHandle, error) {
	consumerTag := "unload-event-reader"
	msgs, err := rbt.chUnload.Consume(rbt.queueUnload, consumerTag, false, false, false, false, nil)
	if err != nil {
		return nil, err
	}

	queue := make(chan *models.AvatarUploadEventHandle)

	go func() {

		defer close(queue)

		for {
			select {
			case <-ctx.Done():
				if err := rbt.chUnload.Cancel(consumerTag, false); err != nil {
					logger.Log.Error("GetChUnloadEvent error cancel consume unload: %v", err)
				}
				return
			case msg, ok := <-msgs:
				if !ok {
					return
				}
				var event models.AvatarUploadEvent
				if err := json.Unmarshal(msg.Body, &event); err != nil {
					logger.Log.Error("GetChUnloadEvent error JSON: %v", err)
					msg.Nack(false, false)
					continue
				}

				ack := func() {
					err := msg.Ack(false)
					if err != nil {
						logger.Log.Error("AvatarUploadEventHandle ack error: %v", err)
					}
				}

				nack := func() {
					err := msg.Nack(false, true)
					if err != nil {
						logger.Log.Error("AvatarUploadEventHandle nack error: %v", err)
					}
				}

				eventH := models.AvatarUploadEventHandle{
					Event: &event,
					Ack:   ack,
					Nack:  nack,
				}

				queue <- &eventH
			}
		}
	}()

	return queue, nil

}

func (rbt *RabbitMQ) GetChDeleteEvent(ctx context.Context) (<-chan *models.AvatarDeleteEventHandle, error) {
	consumerTag := "delete-event-reader"
	msgs, err := rbt.chDelete.Consume(rbt.queueDelete, "delete-event-reader", false, false, false, false, nil)
	if err != nil {
		return nil, err
	}

	queue := make(chan *models.AvatarDeleteEventHandle)

	go func() {

		defer close(queue)

		for {
			select {
			case <-ctx.Done():
				if err := rbt.chDelete.Cancel(consumerTag, false); err != nil {
					logger.Log.Error("GetChDeleteEvent error cancel consume delete: %v", err)
				}
				return
			case msg, ok := <-msgs:
				if !ok {
					return
				}

				var event models.AvatarDeleteEvent
				if err := json.Unmarshal(msg.Body, &event); err != nil {
					logger.Log.Error("GetChDeleteEvent error JSON: %v", err)
					msg.Nack(false, false)
					continue
				}

				ack := func() {
					err := msg.Ack(false)
					if err != nil {
						logger.Log.Error("AvatarDeleteEventHandle ack error: %v", err)
					}
				}

				nack := func() {
					err := msg.Nack(false, true)
					if err != nil {
						logger.Log.Error("AvatarDeleteEventHandle nack error: %v", err)
					}
				}

				eventH := models.AvatarDeleteEventHandle{
					Event: &event,
					Ack:   ack,
					Nack:  nack,
				}

				queue <- &eventH
			}
		}
	}()

	return queue, nil
}

func (rbt *RabbitMQ) Check() string {
	if rbt.conn.IsClosed() {
		return "Connection closed"
	}
	return "ok"
}
