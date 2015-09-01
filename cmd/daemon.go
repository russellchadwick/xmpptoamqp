package main

import (
	"os"
	"os/signal"
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/russellchadwick/xmpptoamqp"
)

func main() {
	app := cli.NewApp()
	app.Usage = "Connects to XMPP and Amqp and passes messages between"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "XmppServer",
			Value:  "talk.google.com:443",
			Usage:  "Host and port to Xmpp server",
			EnvVar: "XMPP_SERVER",
		},
		cli.StringFlag{
			Name:   "XmppUser",
			Usage:  "Xmpp user including hostname, for google this should be @gmail.com",
			EnvVar: "XMPP_USER",
		},
		cli.StringFlag{
			Name:   "XmppPassword",
			Usage:  "Xmpp password",
			EnvVar: "XMPP_PASSWORD",
		},
		cli.StringFlag{
			Name:   "AmqpUri",
			Value:  "amqp://guest:guest@localhost:5672/",
			Usage:  "Amqp uri binding address",
			EnvVar: "AMQP_URI",
		},
		cli.StringFlag{
			Name:   "AmqpSendExchange",
			Value:  "chatSend",
			Usage:  "Send command exchange name",
			EnvVar: "AMQP_SEND_EXCHANGE",
		},
		cli.StringFlag{
			Name:   "AmqpReceivedExchange",
			Value:  "chatReceived",
			Usage:  "Received exchange name",
			EnvVar: "AMQP_RECEIVED_EXCHANGE",
		},
		cli.StringFlag{
			Name:   "AmqpSendQueue",
			Value:  "send",
			Usage:  "Queue name to received send commands from",
			EnvVar: "AMQP_SEND_QUEUE",
		},
	}
	app.Authors = []cli.Author{
		cli.Author{
			Name: "Russell Chadwick",
		},
	}
	app.Version = "1.0.0"
	app.Action = run
	app.RunAndExitOnError()
}

func run(c *cli.Context) {

	xmppServer := c.String("XmppServer")

	xmppUser := c.String("XmppUser")
	if "" == xmppUser {
		cli.ShowAppHelp(c)
		log.Fatal("XMPP User must be provided")
	}

	xmppPassword := c.String("XmppPassword")
	if "" == xmppPassword {
		cli.ShowAppHelp(c)
		log.Fatal("XMPP Password must be provided")
	}

	amqpUri := c.String("AmqpUri")
	amqpSendExchange := c.String("AmqpSendExchange")
	amqpSendType := "direct"
	amqpSendQueue := c.String("AmqpSendQueue")
	amqpService := xmpptoamqp.NewAmqpService(&amqpUri, &xmppServer, &xmppUser, &xmppPassword)
	amqpService.ExchangeDeclare(&amqpSendExchange, &amqpSendType)

	// XMPP -> Channel -> AMQP
	go amqpService.ReceiveXmpp()
	amqpReceivedExchange := c.String("AmqpReceivedExchange")
	amqpReceivedType := "fanout"
	amqpService.ExchangeDeclare(&amqpReceivedExchange, &amqpReceivedType)
	go amqpService.ForwardChatReceivedEvents(&amqpReceivedExchange)

	// AMQP -> Channel -> XMPP
	tag := "xmpptoamqp"
	go amqpService.ConsumeSendCommands(&amqpSendExchange, &amqpSendQueue, &tag)
	go amqpService.SendXmpp()

	// Wait for terminating signal
	sc := make(chan os.Signal, 2)
	signal.Notify(sc, syscall.SIGTERM, syscall.SIGINT)
	<-sc

	log.Info("Received terminate signal")
	amqpService.Close()

	log.Info("Gracefully terminating")

}
