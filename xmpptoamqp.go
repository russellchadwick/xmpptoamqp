package xmpptoamqp

import (
	"crypto/tls"
	"strings"
	"sync"

	log "github.com/Sirupsen/logrus"
	"github.com/golang/protobuf/proto"
	"github.com/mattn/go-xmpp"
	"github.com/streadway/amqp"
)

var waitGroup sync.WaitGroup

type AmqpService struct {
	connection         *amqp.Connection
	channel            *amqp.Channel
	xmppCient          *xmpp.Client
	close              chan bool
	SendChatCommands   chan Chat
	ReceivedChatEvents chan Chat
}

func NewAmqpService(uri, server, user, password *string) (amqpService *AmqpService) {

	amqpService = &AmqpService{
		connection:         nil,
		channel:            nil,
		xmppCient:          connectToXmpp(server, user, password),
		close:              make(chan bool, 1),
		SendChatCommands:   make(chan Chat, 4),
		ReceivedChatEvents: make(chan Chat, 4),
	}

	var err error

	log.WithFields(log.Fields{
		"uri": *uri,
	}).Info("Connecting to AMQP")
	amqpService.connection, err = amqp.Dial(*uri)
	failOnError(err, "AMQP connection error")

	log.Info("Acquire AMQP channel")
	amqpService.channel, err = amqpService.connection.Channel()
	failOnError(err, "AMQP channel error")

	return amqpService

}

func (as *AmqpService) ExchangeDeclare(exchange, kind *string) {

	if len(*exchange) > 0 {

		log.WithFields(log.Fields{
			"exchange": *exchange,
			"kind":     *kind,
		}).Info("Declaring AMQP exchange")

		err := as.channel.ExchangeDeclare(
			*exchange,
			*kind,
			false,
			true,
			false,
			false,
			nil,
		)

		failOnError(err, "AMQP exchange error")
	}

}

func (as *AmqpService) ConsumeSendCommands(exchange, queue, tag *string) {

	waitGroup.Add(1)
	defer waitGroup.Done()

	log.WithFields(log.Fields{
		"queue": *queue,
	}).Info("Declaring AMQP send queue")

	amqpQueue, err := as.channel.QueueDeclare(
		*queue,
		false,
		true,
		false,
		false,
		nil,
	)
	failOnError(err, "AMQP send queue error")

	log.WithFields(log.Fields{
		"messages":  amqpQueue.Messages,
		"consumers": amqpQueue.Consumers,
	}).Info("Declared AMQP send queue")

	log.WithFields(log.Fields{
		"queue":    *queue,
		"exchange": *exchange,
	}).Info("Binding AMQP queue")
	err = as.channel.QueueBind(
		*queue,
		*queue,
		*exchange,
		false,
		nil,
	)
	failOnError(err, "AMQP send queue bind")

	log.WithFields(log.Fields{
		"tag": *tag,
	}).Info("Starting AMQP consumer")
	deliveries, err := as.channel.Consume(
		*queue,
		*tag,
		false,
		false,
		false,
		false,
		nil,
	)
	failOnError(err, "AMQP queue consume")

	for {
		select {
		case <-as.close:
			return
		case delivery := <-deliveries:
			go as.handle(&delivery)
		}
	}

	log.Info("AMQP consumer received close signal")

}

func (as *AmqpService) ForwardChatReceivedEvents(exchange *string) {

	waitGroup.Add(1)
	defer waitGroup.Done()

	for {
		select {
		case <-as.close:
			return
		case chat := <-as.ReceivedChatEvents:
			err := as.channel.Publish(
				*exchange,
				"",
				false,
				false,
				amqp.Publishing{
					ContentType: "application/vnd.google.protobuf",
					Body:        []byte(chat.String()),
				})
			failOnError(err, "Failed to publish a message")
		}
	}

}

func (as *AmqpService) handle(delivery *amqp.Delivery) {

	defer delivery.Ack(true)

	log.WithFields(log.Fields{
		"id":   delivery.MessageId,
		"when": delivery.Timestamp,
	}).Info("AMQP delivery received")

	var chat Chat
	err := proto.Unmarshal(delivery.Body, &chat)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Warn("Unable to unmarshall delivery from AMQP")
	} else {
		as.SendChatCommands <- chat
	}

}

func (as *AmqpService) Send(exchange, key, user, text *string) {

	chat := &Chat{
		User: proto.String(*user),
		Text: proto.String(*text),
	}
	data, err := proto.Marshal(chat)
	failOnError(err, "Serialization failure")

	log.WithFields(log.Fields{
		"exchange": *exchange,
		"key":      *key,
		"data":     data,
	}).Info("sending")

	err = as.channel.Publish(*exchange, *key, false, false, amqp.Publishing{
		Headers:      amqp.Table{},
		ContentType:  "application/vnd.google.protobuf",
		Body:         data,
		DeliveryMode: amqp.Transient,
		Priority:     0,
	})
	failOnError(err, "Publish failure")

}

/*
func (as *AmqpService) FakeReply() {
	for sendChatCommand := range as.SendChatCommands {
		reply := fmt.Sprint("Reply to ", *sendChatCommand.Text)
		receivedChatEvent := Chat{
			User: sendChatCommand.User,
			Text: &reply,
		}
		log.WithFields(log.Fields{
			"original": *sendChatCommand.Text,
			"new":      reply,
		}).Info("Replying")
		as.ReceivedChatEvents <- receivedChatEvent
	}
}
*/

func (as *AmqpService) Close() {
	as.close <- true
	as.close <- true
	as.close <- true
	waitGroup.Wait()
}

func connectToXmpp(server, user, password *string) (xmppClient *xmpp.Client) {

	if server == nil {
		return nil
	}

	xmpp.DefaultConfig = tls.Config{
		ServerName:         serverName(*server),
		InsecureSkipVerify: true,
	}

	options := xmpp.Options{
		Host:          *server,
		User:          *user,
		Password:      *password,
		NoTLS:         false,
		Debug:         false,
		Session:       false,
		Status:        "xa",
		StatusMessage: "Xmppbot",
	}

	log.WithFields(log.Fields{
		"server":   *server,
		"user":     *user,
		"password": *password,
	}).Info("Connecting to XMPP")

	xmppClient, err := options.NewClient()
	failOnError(err, "XMPP connection error")

	return xmppClient

}

func (as *AmqpService) ReceiveXmpp() {

	for {
		chat, err := as.xmppCient.Recv()
		failOnError(err, "XMPP receive error")

		switch v := chat.(type) {
		case xmpp.Chat:
			chatReceivedEvent := Chat{
				User: &v.Remote,
				Text: &v.Text,
			}

			if len(*chatReceivedEvent.Text) > 0 {
				log.WithFields(log.Fields{
					"from": *chatReceivedEvent.User,
					"text": *chatReceivedEvent.Text,
				}).Info("XMPP chat received")

				as.ReceivedChatEvents <- chatReceivedEvent
			}
		}
	}

}

func (as *AmqpService) SendXmpp() {

	waitGroup.Add(1)
	defer waitGroup.Done()

	for {
		select {
		case <-as.close:
			return
		case chat := <-as.SendChatCommands:
			_, err := as.xmppCient.Send(xmpp.Chat{
				Remote: chat.GetUser(),
				Type:   "chat",
				Text:   chat.GetText(),
			})

			if err != nil {
				log.WithFields(log.Fields{
					"error": err,
				}).Warn("Unable to send to XMPP")
			}
		}
	}

}

func serverName(host string) string {
	return strings.Split(host, ":")[0]
}

func failOnError(err error, message string) {
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal(message)
	}
}
