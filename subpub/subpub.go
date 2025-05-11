package subpub

import (
	"context"
	"fmt"
	"sync"
)

// MessageHandler is a callback function that processes messages delivered to subscribers.
type MessageHandler func(msg interface{})

type Subscription interface {
	// Unsubscribe will remove interest in the current subject subscription is for.
	Unsubscribe()
}

type SubPub interface {
	// Subscribe creates an asynchronous queue subscriber on the given subject.
	Subscribe(subject string, cb MessageHandler) (Subscription, error)

	// Publish publishes the msg argument to the given subject.
	// публикует аргумент msg для указанного субъекта.
	Publish(subject string, msg interface{}) error

	// Close will shutdown sub-pub system.
	// May be blocked by data delivery until the context is canceled.
	Close(ctx context.Context) error
}

// реализация подписки
type subscription struct {
	subject  string
	handler  MessageHandler
	msgChan  chan interface{}
	stopChan chan struct{}
	closed   bool
	mu       sync.Mutex
}

func (s *subscription) Unsubscribe() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.closed {
		s.closed = true
		close(s.stopChan)
	}
}

// реализация шины событий
type subPub struct {
	subs      map[string][]*subscription
	subsMu    sync.RWMutex
	wg        sync.WaitGroup
	closeChan chan struct{}
	closed    bool
	closedMu  sync.Mutex
}

func NewSubPub() SubPub {
	return &subPub{
		subs:      make(map[string][]*subscription),
		closeChan: make(chan struct{}),
	}
}

func (s *subPub) Subscribe(subject string, cb MessageHandler) (Subscription, error) {
	s.closedMu.Lock()
	defer s.closedMu.Unlock()

	if s.closed {
		return nil, fmt.Errorf("subpub is closed")
	}

	sub := &subscription{
		subject:  subject,
		handler:  cb,
		msgChan:  make(chan interface{}, 100), // буферизованный канал для FIFO
		stopChan: make(chan struct{}),
	}

	s.subsMu.Lock()
	s.subs[subject] = append(s.subs[subject], sub)
	s.subsMu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case msg := <-sub.msgChan:
				sub.handler(msg)
			case <-sub.stopChan:
				return
			case <-s.closeChan:
				return
			}
		}
	}()

	return sub, nil
}

func (s *subPub) Publish(subject string, msg interface{}) error {
	s.closedMu.Lock()
	defer s.closedMu.Unlock()

	if s.closed {
		return fmt.Errorf("subpub is closed")
	}

	s.subsMu.RLock()
	subs, ok := s.subs[subject]
	s.subsMu.RUnlock()

	if !ok {
		return nil
	}

	for _, sub := range subs {
		sub.mu.Lock()
		if !sub.closed {
			select {
			case sub.msgChan <- msg:
			default:
			}
		}
		sub.mu.Unlock()
	}

	return nil
}

func (s *subPub) Close(ctx context.Context) error {
	s.closedMu.Lock()
	if s.closed {
		s.closedMu.Unlock()
		return nil
	}
	s.closed = true
	close(s.closeChan)
	s.closedMu.Unlock()

	// Ожидаем завершения всех обработчиков или отмены контекста
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
