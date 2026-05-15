package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	adminpb "chatview/gen/chatview/admin"
	authpb "chatview/gen/chatview/auth"
	chatpb "chatview/gen/chatview/chat"
	eventspb "chatview/gen/chatview/events"
	"chatview/internal/config"
	"chatview/internal/db"
	"chatview/internal/eventhub"
	"chatview/internal/interceptor"
	"chatview/internal/service"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "", "path to YAML config")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := db.Open(ctx, cfg.DBDSN)
	if err != nil {
		slog.Error("open database failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	if !cfg.SkipMigrations {
		if err := store.ApplyMigrations(ctx, cfg.MigrationsDir); err != nil {
			slog.Error("apply migrations failed", "error", err)
			os.Exit(1)
		}
	}
	if err := store.SeedAdmin(ctx, cfg.AdminPubKey); err != nil {
		slog.Error("seed admin failed", "error", err)
		os.Exit(1)
	}
	if err := store.CleanupStaleOnline(ctx); err != nil {
		slog.Warn("cleanup stale online sessions failed", "error", err)
	}

	hub := eventhub.New()
	go store.RunCleanup(ctx, cfg.CleanupInterval)
	go eventhub.RunPresenceHealer(ctx, store, hub, cfg.PresenceHealInterval)

	authInterceptor := interceptor.Auth{Store: store}
	opts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(authInterceptor.Unary(), interceptor.LoggingUnary(logger), interceptor.AdminUnary()),
		grpc.ChainStreamInterceptor(authInterceptor.Stream(), interceptor.LoggingStream(logger)),
	}
	if cfg.TLSCert != "" || cfg.TLSKey != "" {
		creds, err := credentials.NewServerTLSFromFile(cfg.TLSCert, cfg.TLSKey)
		if err != nil {
			slog.Error("load TLS credentials failed", "error", err)
			os.Exit(1)
		}
		opts = append(opts, grpc.Creds(creds))
	}

	grpcServer := grpc.NewServer(opts...)
	authpb.RegisterAuthServiceServer(grpcServer, &service.AuthService{
		Store:        store,
		Hub:          hub,
		ChallengeTTL: cfg.ChallengeTTL,
		SessionTTL:   cfg.SessionTTL,
	})
	chatpb.RegisterChatServiceServer(grpcServer, &service.ChatService{Store: store, Hub: hub})
	eventspb.RegisterEventServiceServer(grpcServer, &service.EventService{Store: store, Hub: hub})
	adminpb.RegisterAdminServiceServer(grpcServer, &service.AdminService{Store: store, Hub: hub})
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	reflection.Register(grpcServer)

	listener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		slog.Error("listen failed", "addr", cfg.ListenAddr, "error", err)
		os.Exit(1)
	}

	go func() {
		slog.Info("chatview grpc server listening", "addr", cfg.ListenAddr)
		if err := grpcServer.Serve(listener); err != nil {
			slog.Error("grpc server stopped", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	done := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		grpcServer.Stop()
	}
	slog.Info("chatview grpc server stopped")
}
