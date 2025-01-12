package models

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"sync"
	"text/template"

	"github.com/google/uuid"
)

type Hub struct {
	mu sync.RWMutex

	clients     map[*Client]bool
	clientChats map[*Client]bool
	Id          uuid.UUID

	Broadcast      chan *Message
	Register       chan *Client
	Unregister     chan *Client
	RegisterChat   chan *Client
	UnregisterChat chan *Client

	Messages []*MessageWithTimeCode
	Users    *UserList
}

func NewHub(ul *UserList) *Hub {
	return &Hub{
		clients:        make(map[*Client]bool),
		clientChats:    make(map[*Client]bool),
		Broadcast:      make(chan *Message),
		Register:       make(chan *Client),
		Unregister:     make(chan *Client),
		RegisterChat:   make(chan *Client),
		UnregisterChat: make(chan *Client),
		Users:          ul,
	}
}

func (h *Hub) Start() {
	for {
		select {
		case msg, ok := <-h.Broadcast:
			if !ok {
				log.Printf("HUB: broadcast channel has been closed. Closing hub %v", h.Id)
				for client := range h.clients {
					log.Printf("HUB: closing client %v", client.Id)
					close(client.Send)
					delete(h.clients, client)
				}
				for client := range h.clientChats {
					log.Printf("HUB: closing client chat %v", client.Id)
					close(client.Send)
					delete(h.clients, client)
				}
				return
			}
			log.Printf("HUB: hub %v getting broadcast: %v", h.Id, msg)
			h.mu.RLock()

			msgWithTimeCode := NewMessageWithTimeCode(msg.Id, msg.Content)
			h.Messages = append(h.Messages, msgWithTimeCode)

			hours, minutes, seconds := msgWithTimeCode.TimeCode.Clock()
			timeString := fmt.Sprintf("%d%02d%02d", hours, minutes, seconds)
			user := h.Users.GetUserById(msg.Id)

			timeString = timeString[:len(timeString)-2] + ":" + timeString[len(timeString)-2:]
			timeString = timeString[:len(timeString)-5] + ":" + timeString[len(timeString)-5:]

			data := struct {
				TimeCode   string
				UserName   string
				Content    string
				AvatarPath string
			}{
				TimeCode:   timeString,
				UserName:   user.Name,
				Content:    msg.Content,
				AvatarPath: user.AvatarPath,
			}

			// This loop is for notifications
			// Target: room-card and it's notification bubble component
			for client := range h.clients {
				// Also make notificationByteTemplate to send to notification clients
				byteTemplate, err := GetTemplateBytes("message-card", data)
				log.Printf("%v", string(byteTemplate))
				if err != nil {
					log.Printf("HUB: failed to convert broadcasted message to bytes: %v", err)
					// byteTemplate = []byte(msg.Content)
				}

				select {
				// Replace []byte(msg.Content) with notificationByteTemplate
				case client.Send <- []byte(msg.Content):
				default:
					log.Printf("HUB: client unresponsive. Closing client %v", client.Id)
					close(client.Send)
					delete(h.clients, client)
				}
			}

			// This loop is for actual formatted chat messages
			// Target: chat-window
			for client := range h.clientChats {
				// Also make notificationByteTemplate to send to notification clients
				var err error = nil
				var byteTemplate []byte
				if client.Id == msg.Id {
					byteTemplate, err = GetTemplateBytes("message-card", data)
				} else {
					byteTemplate, err = GetTemplateBytes("message-card-other", data)
				}
				log.Printf("%v", string(byteTemplate))
				if err != nil {
					log.Printf("HUB: failed to convert broadcasted message to bytes: %v", err)
					byteTemplate = []byte(msg.Content)
				}

				select {
				case client.Send <- byteTemplate:
				default:
					log.Printf("HUB: client chat unresponsive. Closing client chat %v", client.Id)
					close(client.Send)
					delete(h.clients, client)
				}
			}

			h.mu.RUnlock()

		case client := <-h.Register:
			log.Printf("HUB: hub %v getting register of user %v", h.Id, client.Id)
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()

			// Change to notification template
			log.Printf("HUB: sending message history from hub %v to client %v", h.Id, client.Id)
			for _, msg := range h.Messages {
				byteTemplate, err := GetTemplateBytes("message-card", msg)
				log.Printf("%v", string(byteTemplate))
				if err != nil {
					client.Send <- []byte(msg.Content)
					continue
				}

				client.Send <- byteTemplate
			}

		case client := <-h.Unregister:
			log.Printf("HUB: hub %v getting UNregister of user %v", h.Id, client.Id)
			h.mu.Lock()

			h.clients[client] = false
			if _, ok := h.clients[client]; ok {
				log.Printf("HUB: client unregistered. Closing client %v", client.Id)
				close(client.Send)
				delete(h.clients, client)
			}

			h.mu.Unlock()

		case client := <-h.RegisterChat:
			log.Printf("HUB: hub %v getting register of chat %v", h.Id, client.Id)
			h.mu.Lock()
			h.clientChats[client] = true
			h.mu.Unlock()

			log.Printf("HUB: sending message history from hub %v to client chat %v", h.Id, client.Id)
			for _, msg := range h.Messages {
				hours, minutes, seconds := msg.TimeCode.Clock()
				timeString := fmt.Sprintf("%d%02d%02d", hours, minutes, seconds)
				user := h.Users.GetUserById(msg.Id)

				timeString = timeString[:len(timeString)-2] + ":" + timeString[len(timeString)-2:]
				timeString = timeString[:len(timeString)-5] + ":" + timeString[len(timeString)-5:]

				data := struct {
					TimeCode   string
					UserName   string
					Content    string
					AvatarPath string
				}{
					TimeCode:   timeString,
					UserName:   user.Name,
					Content:    msg.Content,
					AvatarPath: user.AvatarPath,
				}

				var err error = nil
				var byteTemplate []byte
				if client.Id == msg.Id {
					byteTemplate, err = GetTemplateBytes("message-card", data)
				} else {
					byteTemplate, err = GetTemplateBytes("message-card-other", data)
				}
				log.Printf("%v", string(byteTemplate))
				if err != nil {
					log.Printf("HUB: failed to convert broadcasted message to bytes: %v", err)
					client.Send <- []byte(msg.Content)
					continue
				}

				client.Send <- byteTemplate
			}

		case client := <-h.UnregisterChat:
			log.Printf("HUB: hub %v getting UNregister of chat %v", h.Id, client.Id)
			h.mu.Lock()

			h.clientChats[client] = false
			if _, ok := h.clientChats[client]; ok {
				log.Printf("HUB: client unregistered. Closing client chat %v", client.Id)
				close(client.Send)
				delete(h.clientChats, client)
			}

			h.mu.Unlock()
		}
	}
}

func GetTemplateBytes(name string, data interface{}) ([]byte, error) {
	t := template.New("")
	_, err := t.ParseGlob("views/*.html")
	if err != nil {
		log.Printf("error parsing blob for template bytes: %v", err)
		return nil, err
	}
	_, err = t.ParseGlob("views/components/*.html")
	if err != nil {
		log.Printf("error parsing blob for template bytes: %v", err)
		return nil, err
	}

	tmpl := t.Lookup(name)
	if tmpl == nil {
		err := errors.New("error parsing template for bytes: template with given name not found")
		log.Printf("%v", err)
		return nil, err
	}

	var renderedMessage bytes.Buffer
	err = tmpl.Execute(&renderedMessage, data)
	if err != nil {
		log.Printf("error executing template for bytes: %v", err)
		return nil, err
	}

	return renderedMessage.Bytes(), nil
}
