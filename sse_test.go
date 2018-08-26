package sse

import (
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

type CloseNotifyResponseRecorder struct {
	httptest.ResponseRecorder
	closeChan chan bool
	flushChan chan bool
}

func (r *CloseNotifyResponseRecorder) CloseNotify() <-chan bool {
	return r.closeChan
}

func (r *CloseNotifyResponseRecorder) Flush() {
	r.flushChan <- true
}

func NewRecorder() *CloseNotifyResponseRecorder {
	r := httptest.NewRecorder()
	return &CloseNotifyResponseRecorder{
		ResponseRecorder: *r,
		closeChan:        make(chan bool),
		flushChan:        make(chan bool),
	}
}

func TestSingleClient(t *testing.T) {
	s := NewService()
	r := NewRecorder()
	clientID := "client1"
	closeChan, err := s.HandleClient(clientID, r)
	assert.NoError(t, err)
	s.Send(clientID, Event{
		Data: "123",
	})
	<-r.flushChan
	assert.Equal(t, r.Body.String(), "data:123\n\n")
	go func() {
		s.CloseClient(clientID)
	}()
	<-closeChan
	assert.Equal(t, s.GetClientCount(), 0)
}

func TestSingleClientWithConfig(t *testing.T) {
	s := NewServiceWithOption(Option{
		Headers: map[string]string{"X-Accel-Buffering": "no"},
	})
	r := NewRecorder()
	clientID := "client1"
	closeChan, err := s.HandleClient(clientID, r)
	assert.NoError(t, err)
	s.Send(clientID, Event{
		Data: "123",
	})
	<-r.flushChan
	assert.Equal(t, "no", r.HeaderMap.Get("X-Accel-Buffering"))
	assert.Equal(t, r.Body.String(), "data:123\n\n")
	go func() {
		s.CloseClient(clientID)
	}()
	<-closeChan
	assert.Equal(t, s.GetClientCount(), 0)
}

func TestSingleClientWithClose(t *testing.T) {
	s := NewService()
	r := NewRecorder()
	clientID := "client1"
	closeChan, err := s.HandleClient(clientID, r)
	assert.NoError(t, err)
	s.Send(clientID, Event{
		Data: "123",
	})
	<-r.flushChan
	assert.Equal(t, r.Body.String(), "data:123\n\n")
	go func() {
		r.closeChan <- true
	}()
	<-closeChan
	assert.Equal(t, s.GetClientCount(), 0)
}

func TestMultiClients(t *testing.T) {
	var wg sync.WaitGroup
	var wgAdd sync.WaitGroup
	s := NewService()
	clientID1 := "client1"
	wg.Add(1)
	wgAdd.Add(1)
	go func() {
		defer wg.Done()
		addClient(t, s, clientID1, &wgAdd)
	}()
	clientID2 := "client2"
	wg.Add(1)
	wgAdd.Add(1)
	go func() {
		defer wg.Done()
		addClient(t, s, clientID2, &wgAdd)
	}()
	clientID3 := "client3"
	wg.Add(1)
	wgAdd.Add(1)
	go func() {
		defer wg.Done()
		addClient(t, s, clientID3, &wgAdd)
	}()
	wgAdd.Wait()
	assert.Equal(t, s.GetClientCount(), 3)
	s.Broadcast(Event{
		Data: "123",
	})
	s.CloseAllClients()
	wg.Wait()
	assert.Equal(t, s.GetClientCount(), 0)
}

func addClient(t *testing.T, s *Service, clientID string, wgAdd *sync.WaitGroup) {
	r := NewRecorder()
	closeChan, err := s.HandleClient(clientID, r)
	assert.NoError(t, err)
	wgAdd.Done()
	<-r.flushChan
	assert.Equal(t, r.Body.String(), "data:123\n\n")
	<-closeChan
}
