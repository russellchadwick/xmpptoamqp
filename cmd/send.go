package main

import (
	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/russellchadwick/xmpptoamqp"
)

func main() {
	app := cli.NewApp()
	app.Usage = "Sends a chat command via AMQP"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "AmqpUri",
			Value:  "amqp://guest:guest@localhost:5672/",
			Usage:  "Amqp uri binding address",
			EnvVar: "AMQP_URI",
		},
		cli.StringFlag{
			Name:   "AmqpExchange",
			Value:  "chatSend",
			Usage:  "Exchange name",
			EnvVar: "AMQP_EXCHANGE",
		},
		cli.StringFlag{
			Name:   "AmqpKey",
			Value:  "send",
			Usage:  "Routing key to send commands to",
			EnvVar: "AMQP_KEY",
		},
		cli.StringFlag{
			Name:   "User",
			Usage:  "User to send chat to",
			EnvVar: "CHAT_USER",
		},
		cli.StringFlag{
			Name:   "Text",
			Usage:  "Body to send",
			EnvVar: "CHAT_TEXT",
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

	user := c.String("User")
	if 0 == len(user) {
		cli.ShowAppHelp(c)
		log.Fatal("User must be provided")
	}

	text := c.String("Text")
	if 0 == len(text) {
		cli.ShowAppHelp(c)
		log.Fatal("Text must be provided")
	}

	amqpUri := c.String("AmqpUri")
	amqpExchange := c.String("AmqpExchange")
	amqpType := "direct"
	key := c.String("AmqpKey")
	amqpService := xmpptoamqp.NewAmqpService(&amqpUri, nil, nil, nil)
	amqpService.ExchangeDeclare(&amqpExchange, &amqpType)
	amqpService.Send(&amqpExchange, &key, &user, &text)

	log.WithFields(log.Fields{
		"user": c.String("User"),
	}).Info("Chat sent")

}
