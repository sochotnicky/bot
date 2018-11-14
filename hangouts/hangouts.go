package hangouts

import (
	"cloud.google.com/go/pubsub"
	"context"
	"encoding/json"
	"github.com/go-chat-bot/bot"
	"golang.org/x/oauth2/google"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	chatAuthScope = "https://www.googleapis.com/auth/chat.bot"
	apiEndpoint   = "https://chat.googleapis.com/v1/"
)

var (
	httpChatClient *http.Client
	b              *bot.Bot
)

// Config must contain basic configuration for the bot to be able to work
type Config struct {
	PubSubProject    string
	TopicName        string
	SubscriptionName string
	Token            string
}

func responseHandler(space string, message string, sender *bot.User) {
	resp, err := httpChatClient.Post(apiEndpoint+space+"/messages",
		"application/json",
		strings.NewReader("{\"text\":\""+message+"\"}"))
	if err != nil {
		log.Printf("Error posting reply: %v", err)
	}

	log.Printf("Response: %s\n", resp.Status)
}

// Run reads the config, establishes OAuth connection & Pub/Sub subscription to
// the message queue
func Run(config *Config) {
	var err error
	ctx := context.Background()
	httpChatClient, err = google.DefaultClient(ctx, chatAuthScope)
	if err != nil {
		log.Printf("Error setting http client: %v\n", err)
		return
	}

	client, err := pubsub.NewClient(ctx, config.PubSubProject)
	if err != nil {
		log.Printf("error creating client: %v\n", err)
		return
	}

	topic := client.Topic(config.TopicName)

	// Create a new subscription to the previously created topic
	// with the given name.
	sub := client.Subscription(config.SubscriptionName)
	ok, err := sub.Exists(ctx)
	if err != nil {
		log.Printf("Error getting subscription: %v\n", err)
		return
	}
	if !ok {
		// Subscription doesn't exist.
		sub, err = client.CreateSubscription(ctx, config.SubscriptionName,
			pubsub.SubscriptionConfig{
				Topic:       topic,
				AckDeadline: 10 * time.Second,
			})
		if err != nil {
			log.Printf("error subscribing: %v\n", err)
			return
		}
	}

	b = bot.New(&bot.Handlers{
		Response: responseHandler,
	})

	err = sub.Receive(context.Background(),
		func(ctx context.Context, m *pubsub.Message) {
			log.Printf("Got message: %s\n\n", m.Data)
			var msg ChatMessage
			err = json.Unmarshal(m.Data, &msg)
			if err != nil {
				log.Printf("Failed message unmarshal: %v\n", err)
				m.Ack()
				return
			}
			if msg.Token != config.Token {
				log.Printf("Failed to verify token: %s", msg.Token)
				m.Ack()
				return
			}

			log.Printf("Space: %s\n", msg.Space.Name)
			switch msg.Type {
			case "ADDED_TO_SPACE":
				break
			case "REMOVED_FROM_SPACE":
				break
			case "MESSAGE":
				b.MessageReceived(
					&bot.ChannelData{
						Protocol:  "hangouts",
						Server:    "chat.google.com",
						HumanName: msg.Space.DisplayName,
						Channel:   msg.Space.Name,
						IsPrivate: msg.Space.Type == "DM",
					},
					&bot.Message{
						Text:     msg.Message.Text,
						IsAction: false,
					},
					&bot.User{
						ID:       msg.User.Name,
						Nick:     msg.User.DisplayName,
						RealName: msg.User.DisplayName,
					})
			}

			m.Ack()
		})
	if err != nil {
		log.Printf("error setting up receiving: %v\n", err)
		return
	}
	// Wait indefinetely
	select {}
}
