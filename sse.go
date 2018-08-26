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
	opts    Option
}

// NewService is used to create a sse.Service instance
func NewService() *Service {
	return &Service{
		clients: make(map[interface{}]clientInstance),
	}
}

// NewServiceWithOption is used to create a sse.Service instance with additional option
func NewServiceWithOption(o Option) *Service {
	return &Service{
		clients: make(map[interface{}]clientInstance),
		opts:    o,
	}
}

// HandleClient is used to handle a client with streaming and returns a chan of closing
func (s *Service) HandleClient(clientID interface{}, w http.ResponseWriter) (<-chan CloseType, error) {
	s.lck.Lock()
	defer s.lck.Unlock()
	_, has := s.clients[clientID]
	if has {
		return nil, fmt.Errorf("client with id %v is already exist", clientID)
	}
	f, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming unsupported")
	}
	cn, ok := w.(http.CloseNotifier)
	if !ok {
		return nil, fmt.Errorf("close notify unsupported")
	}
	notify := cn.CloseNotify()
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
		for k, v := range s.opts.Headers {
			w.Header().Set(k, v)
		}
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
	s.lck.RLock()
	client, has := s.clients[clientID]
	s.lck.RUnlock()
	if !has {
		return fmt.Errorf("client with id %v is not found", clientID)
	}
	close(client.closeChan)
	return nil
}

// CloseAllClients is used to disconnect all clients
func (s *Service) CloseAllClients() {
	s.doEachClient(func(ci clientInstance) {
		close(ci.closeChan)
	})
}

// Send is used to send data to client
func (s *Service) Send(clientID interface{}, e Event) error {
	s.lck.RLock()
	client, has := s.clients[clientID]
	s.lck.RUnlock()
	if !has {
		return fmt.Errorf("client with id %v is not found", clientID)
	}
	client.msgChan <- e
	return nil
}

// Broadcast is used to broadcast event to all connected clients
func (s *Service) Broadcast(e Event) {
	s.doEachClient(func(ci clientInstance) {
		ci.msgChan <- e
	})
}

// GetClientCount is used to get count of client
func (s *Service) GetClientCount() int {
	s.lck.RLock()
	defer s.lck.RUnlock()
	return len(s.clients)
}

func (s *Service) doEachClient(fn func(ci clientInstance)) {
	s.lck.RLock()
	cs := make([]clientInstance, len(s.clients))
	i := 0
	for _, c := range s.clients {
		cs[i] = c
		i = i + 1
	}
	s.lck.RUnlock()
	var wg sync.WaitGroup
	for _, c := range cs {
		wg.Add(1)
		go func(ci clientInstance) {
			defer wg.Done()
			fn(ci)
		}(c)
	}
	wg.Wait()
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
