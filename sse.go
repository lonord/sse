package sse

import (
	"fmt"
	"net/http"
	"sync"

	msse "github.com/manucorporat/sse"
)

// Event is the Server-Sent Event data struct
type Event struct {
	Event string
	ID    string
	Retry uint
	Data  interface{}
}

// CloseType present the type of closing
type CloseType int

const (
	_ CloseType = iota
	// ClientClose present client initiative to disconnect
	ClientClose
	// ServerClose present server initiative to disconnect
	ServerClose
)

// Service is the type of sse service instance
type Service struct {
	clients map[interface{}]clientInstance
	lck     sync.RWMutex
}

// NewService is used to create a sse.Service instance
func NewService() *Service {
	return &Service{
		clients: make(map[interface{}]clientInstance),
	}
}

// HandleClient is used to handle a client with streaming and returns a chan of closing
func (s *Service) HandleClient(clientID interface{}, w http.ResponseWriter) (chan CloseType, error) {
	s.lck.RLock()
	defer s.lck.RUnlock()
	_, has := s.clients[clientID]
	if has {
		return nil, fmt.Errorf("client with id %v is already exist", clientID)
	}
	f, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming unsupported")
	}
	notify := w.(http.CloseNotifier).CloseNotify()
	outChan := make(chan CloseType)
	closeChan := make(chan bool)
	msgChan := make(chan Event)
	client := clientInstance{
		msgChan:   msgChan,
		closeChan: closeChan,
	}
	s.clients[clientID] = client
	go func() {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		for {
			select {
			case <-notify:
				s.lck.Lock()
				delete(s.clients, clientID)
				s.lck.Unlock()
				outChan <- ClientClose
				break
			case <-closeChan:
				s.lck.Lock()
				delete(s.clients, clientID)
				s.lck.Unlock()
				outChan <- ServerClose
				break
			case d := <-msgChan:
				msse.Encode(w, convertToMSSE(d))
				f.Flush()
			}
		}
	}()
	return outChan, nil
}

// CloseClient is used to disconnect client by server
func (s *Service) CloseClient(clientID interface{}) error {
	client, has := s.clients[clientID]
	if !has {
		return fmt.Errorf("client with id %v is not found", clientID)
	}
	client.closeChan <- true
	return nil
}

// Send is used to send data to client
func (s *Service) Send(clientID interface{}, e Event) error {
	client, has := s.clients[clientID]
	if !has {
		return fmt.Errorf("client with id %v is not found", clientID)
	}
	client.msgChan <- e
	return nil
}

type clientInstance struct {
	msgChan   chan Event
	closeChan chan bool
}

func convertToMSSE(e Event) msse.Event {
	return msse.Event{
		Event: e.Event,
		Id:    e.ID,
		Retry: e.Retry,
		Data:  e.Data,
	}
}
