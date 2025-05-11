package server

import (
	"context"
	"net"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/grpc"

	"github.com/AlexandrZlnov/pubsub-service/proto"

	"github.com/AlexandrZlnov/pubsub-service/config"
	"github.com/AlexandrZlnov/pubsub-service/internal/service"
)

type Server struct {
	grpcServer *grpc.Server
	service    *service.PubSubService
	logger     zerolog.Logger
	config     *config.Config
}

func NewServer(
	service *service.PubSubService,
	logger zerolog.Logger,
	config *config.Config,
) *Server {
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(loggingUnaryInterceptor(logger)),
		grpc.StreamInterceptor(loggingStreamInterceptor(logger)),
	)

	proto.RegisterPubSubServer(grpcServer, service)

	return &Server{
		grpcServer: grpcServer,
		service:    service,
		logger:     logger,
		config:     config,
	}
}

func (s *Server) Run() error {
	lis, err := net.Listen("tcp", ":"+s.config.GRPCPort)
	if err != nil {
		return err
	}

	s.logger.Info().Str("port", s.config.GRPCPort).Msg("starting gRPC server")
	return s.grpcServer.Serve(lis)
}

func (s *Server) GracefulStop(ctx context.Context) error {
	stopped := make(chan struct{})
	go func() {
		s.grpcServer.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
		s.logger.Info().Msg("gRPC server stopped gracefully")
		return nil
	case <-ctx.Done():
		s.grpcServer.Stop()
		s.logger.Warn().Msg("gRPC server stopped forcefully")
		return ctx.Err()
	}
}

func loggingUnaryInterceptor(logger zerolog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		start := time.Now()

		resp, err = handler(ctx, req)

		logger.Info().
			Str("method", info.FullMethod).
			Dur("duration", time.Since(start)).
			Err(err).
			Msg("unary request")

		return resp, err
	}
}

func loggingStreamInterceptor(logger zerolog.Logger) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()

		err := handler(srv, stream)

		logger.Info().
			Str("method", info.FullMethod).
			Dur("duration", time.Since(start)).
			Err(err).
			Msg("stream request")

		return err
	}
}
