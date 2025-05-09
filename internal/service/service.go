package service

import (
	"context"
	"sync"

	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/AlexandrZlnov/pubsub-service/config"
	"github.com/AlexandrZlnov/pubsub-service/proto"
	"github.com/AlexandrZlnov/pubsub-service/subpub"
)

type PubSubService struct {
	proto.UnimplementedPubSubServer
	bus    subpub.SubPub
	logger zerolog.Logger
	config *config.Config
	subs   map[string]map[*sub]struct{}
	mu     sync.RWMutex
}

type sub struct {
	stream proto.PubSub_SubscribeServer
	done   chan struct{}
}

func NewPubSubService(
	bus subpub.SubPub,
	logger zerolog.Logger,
	config *config.Config,
) *PubSubService {
	return &PubSubService{
		bus:    bus,
		logger: logger,
		config: config,
		subs:   make(map[string]map[*sub]struct{}),
	}
}

func (s *PubSubService) Subscribe(
	req *proto.SubscribeRequest,
	stream proto.PubSub_SubscribeServer,
) error {
	key := req.GetKey()

	s.mu.Lock()
	if _, exists := s.subs[key]; !exists {
		s.subs[key] = make(map[*sub]struct{})
	}

	currentSub := &sub{
		stream: stream,
		done:   make(chan struct{}),
	}
	s.subs[key][currentSub] = struct{}{}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.subs[key], currentSub)
		if len(s.subs[key]) == 0 {
			delete(s.subs, key)
		}
		s.mu.Unlock()
	}()

	// Подписываемся на шину событий
	subscription, err := s.bus.Subscribe(key, func(msg interface{}) {
		data, ok := msg.(string)
		if !ok {
			s.logger.Error().Msg("invalid message type")
			return
		}

		err := stream.Send(&proto.Event{Data: data})
		if err != nil {
			s.logger.Error().Err(err).Msg("failed to send event")
		}
	})

	if err != nil {
		return status.Errorf(codes.Internal, "failed to subscribe: %v", err)
	}
	defer subscription.Unsubscribe()

	s.logger.Info().Str("key", key).Msg("new subscription")

	// Ждем завершения стрима или отмены контекста
	select {
	case <-stream.Context().Done():
		s.logger.Info().Str("key", key).Msg("subscription closed by client")
		return nil
	case <-currentSub.done:
		s.logger.Info().Str("key", key).Msg("subscription closed by server")
		return nil
	}
}

func (s *PubSubService) Publish(
	ctx context.Context,
	req *proto.PublishRequest,
) (*emptypb.Empty, error) {
	key := req.GetKey()
	data := req.GetData()

	if err := s.bus.Publish(key, data); err != nil {
		s.logger.Error().
			Err(err).
			Str("key", key).
			Msg("failed to publish event")
		return nil, status.Errorf(codes.Internal, "failed to publish: %v", err)
	}

	s.logger.Debug().
		Str("key", key).
		Str("data", data).
		Msg("event published")

	return &emptypb.Empty{}, nil
}

func (s *PubSubService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, subs := range s.subs {
		for sub := range subs {
			close(sub.done)
		}
		delete(s.subs, key)
	}

	return nil
}
