package subpub

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewSubPub(t *testing.T) {
	sp := NewSubPub()
	if sp == nil {
		t.Error("NewSubPub() returned nil")
	}
}

func TestSubscribePublish(t *testing.T) {
	sp := NewSubPub()
	defer sp.Close(context.Background())

	var received bool
	sub, err := sp.Subscribe("test", func(msg interface{}) {
		if msg.(string) == "hello" {
			received = true
		}
	})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	err = sp.Publish("test", "hello")
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Даем время на обработку сообщения
	time.Sleep(100 * time.Millisecond)

	if !received {
		t.Error("Message was not received by subscriber")
	}

	sub.Unsubscribe()
}

func TestMultipleSubscribers(t *testing.T) {
	sp := NewSubPub()
	defer sp.Close(context.Background())

	var counter int
	var mu sync.Mutex

	// Создаем 5 подписчиков
	for i := 0; i < 5; i++ {
		_, err := sp.Subscribe("multi", func(msg interface{}) {
			mu.Lock()
			counter++
			mu.Unlock()
		})
		if err != nil {
			t.Fatalf("Subscribe %d failed: %v", i, err)
		}
	}

	err := sp.Publish("multi", "message")
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Даем время на обработку сообщения
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if counter != 5 {
		t.Errorf("Expected 5 subscribers to receive message, got %d", counter)
	}
	mu.Unlock()
}

func TestUnsubscribe(t *testing.T) {
	sp := NewSubPub()
	defer sp.Close(context.Background())

	var received bool
	sub, err := sp.Subscribe("unsub", func(msg interface{}) {
		received = true
	})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	sub.Unsubscribe()

	err = sp.Publish("unsub", "message")
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Даем время на обработку сообщения
	time.Sleep(100 * time.Millisecond)

	if received {
		t.Error("Message was received by unsubscribed handler")
	}
}

func TestOrdering(t *testing.T) {
	sp := NewSubPub()
	defer sp.Close(context.Background())

	var results []int
	var mu sync.Mutex

	sub, err := sp.Subscribe("order", func(msg interface{}) {
		mu.Lock()
		results = append(results, msg.(int))
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer sub.Unsubscribe()

	// Публикуем 10 сообщений
	for i := 0; i < 10; i++ {
		err := sp.Publish("order", i)
		if err != nil {
			t.Fatalf("Publish %d failed: %v", i, err)
		}
	}

	// Даем время на обработку всех сообщений
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	if len(results) != 10 {
		t.Fatalf("Expected 10 messages, got %d", len(results))
	}
	for i := 0; i < 10; i++ {
		if results[i] != i {
			t.Errorf("Message out of order at position %d: expected %d, got %d", i, i, results[i])
		}
	}
	mu.Unlock()
}

func TestSlowSubscriber(t *testing.T) {
	sp := NewSubPub()
	defer sp.Close(context.Background())

	slowDone := make(chan struct{})
	fastDone := make(chan struct{})

	// Медленный подписчик
	_, err := sp.Subscribe("slow", func(msg interface{}) {
		time.Sleep(200 * time.Millisecond)
		close(slowDone)
	})
	if err != nil {
		t.Fatalf("Subscribe slow failed: %v", err)
	}

	// Быстрый подписчик
	_, err = sp.Subscribe("slow", func(msg interface{}) {
		close(fastDone)
	})
	if err != nil {
		t.Fatalf("Subscribe fast failed: %v", err)
	}

	err = sp.Publish("slow", "message")
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Проверяем, что быстрый подписчик завершился до медленного
	select {
	case <-fastDone:
		select {
		case <-slowDone:
			t.Error("Slow subscriber finished before fast one")
		default:
			// Ожидаемый случай - fastDone закрылся, slowDone еще нет
		}
	case <-time.After(300 * time.Millisecond):
		t.Error("Test timed out")
	}
}

func TestClose(t *testing.T) {
	sp := NewSubPub()

	var handlerRan bool
	_, err := sp.Subscribe("close", func(msg interface{}) {
		time.Sleep(100 * time.Millisecond)
		handlerRan = true
	})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	err = sp.Publish("close", "message")
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Закрываем с коротким таймаутом
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = sp.Close(ctx)
	if err == nil {
		t.Error("Expected context cancellation error, got nil")
	}

	// Проверяем, что обработчик все равно выполнился
	time.Sleep(200 * time.Millisecond)
	if !handlerRan {
		t.Error("Handler did not run after Close")
	}

	// Проверяем, что после закрытия нельзя подписаться или опубликовать
	_, err = sp.Subscribe("closed", func(msg interface{}) {})
	if err == nil {
		t.Error("Expected error when subscribing to closed subpub")
	}

	err = sp.Publish("closed", "message")
	if err == nil {
		t.Error("Expected error when publishing to closed subpub")
	}
}

func TestConcurrentAccess(t *testing.T) {
	sp := NewSubPub()
	defer sp.Close(context.Background())

	var wg sync.WaitGroup
	wg.Add(3)

	// Горутина для подписок
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_, err := sp.Subscribe("concurrent", func(msg interface{}) {})
			if err != nil {
				t.Errorf("Subscribe failed: %v", err)
				return
			}
		}
	}()

	// Горутина для публикаций
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			err := sp.Publish("concurrent", i)
			if err != nil {
				t.Errorf("Publish failed: %v", err)
				return
			}
		}
	}()

	// Горутина для отписок
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			sub, err := sp.Subscribe("concurrent_unsub", func(msg interface{}) {})
			if err != nil {
				t.Errorf("Subscribe for unsub failed: %v", err)
				return
			}
			sub.Unsubscribe()
		}
	}()

	wg.Wait()
}
