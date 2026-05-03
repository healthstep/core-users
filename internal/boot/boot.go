package boot

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/helthtech/core-users/internal/middleware"
	"github.com/helthtech/core-users/internal/migration"
	"github.com/helthtech/core-users/internal/obs"
	"github.com/helthtech/core-users/internal/repository"
	"github.com/helthtech/core-users/internal/server"
	"github.com/helthtech/core-users/internal/service"
	pb "github.com/helthtech/core-users/pkg/proto/users"
	"github.com/nats-io/nats.go"
	"github.com/porebric/configs"
	"github.com/porebric/logger"
	"github.com/porebric/resty"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"google.golang.org/grpc"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlog "gorm.io/gorm/logger"
)

func Run(ctx context.Context) error {
	db, err := initDB(ctx)
	if err != nil {
		return fmt.Errorf("init db: %w", err)
	}
	if err = migration.Run(db); err != nil {
		return fmt.Errorf("migration: %w", err)
	}

	rdb := initRedis(ctx)
	nc, err := initNATS(ctx)
	if err != nil {
		return fmt.Errorf("init nats: %w", err)
	}

	tp, err := initTracer(ctx)
	if err != nil {
		obs.BG("tracer").Error(err, "tracer init failed (non-fatal)")
	} else {
		otel.SetTracerProvider(tp)
		defer func() { _ = tp.Shutdown(context.Background()) }()
	}

	jwtSecret := configs.Value(ctx, "jwt_secret").String()
	tokenTTL := configs.Value(ctx, "token_ttl").Duration()
	if tokenTTL == 0 {
		tokenTTL = 24 * time.Hour
	}
	authKeyTTL := configs.Value(ctx, "auth_key_ttl").Duration()
	if authKeyTTL == 0 {
		authKeyTTL = 5 * time.Minute
	}

	jwtSvc := service.NewJWTService(jwtSecret, tokenTTL)
	repo := repository.NewUserRepository(db)
	authSvc := service.NewAuthService(repo, jwtSvc, rdb, nc, authKeyTTL)
	userSvc := service.NewUserService(repo, jwtSvc)

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			middleware.GRPCUnaryAccessLog(),
			middleware.PanicRecoveryInterceptor(),
			middleware.AuthUnaryInterceptor(jwtSvc),
		),
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	)
	pb.RegisterUserServiceServer(grpcServer, server.NewUserServer(authSvc, userSvc))

	grpcPort := configs.Value(ctx, "grpc_port").String()
	lis, err := net.Listen("tcp", "0.0.0.0:"+grpcPort)
	if err != nil {
		return fmt.Errorf("listen grpc: %w", err)
	}

	go func() {
		obs.L.Info("gRPC server listening", "addr", grpcPort)
		if err := grpcServer.Serve(lis); err != nil {
			obs.L.Error(err, "gRPC serve error")
		}
	}()

	router := resty.NewRouter(func() *logger.Logger { return obs.L }, nil)
	resty.RunServer(ctx, router, func(ctx context.Context) error {
		grpcServer.GracefulStop()
		nc.Close()
		return nil
	})

	return nil
}

func initDB(ctx context.Context) (*gorm.DB, error) {
	dsn := configs.Value(ctx, "db_dsn").String()
	return gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlog.Default.LogMode(gormlog.Warn),
	})
}

func initRedis(ctx context.Context) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     configs.Value(ctx, "redis_addr").String(),
		Password: configs.Value(ctx, "redis_password").String(),
	})
}

func initNATS(ctx context.Context) (*nats.Conn, error) {
	url := configs.Value(ctx, "nats_url").String()
	return nats.Connect(url)
}

func initTracer(ctx context.Context) (*sdktrace.TracerProvider, error) {
	host := configs.Value(ctx, "tracer_host").String()
	port := configs.Value(ctx, "tracer_port").String()
	svcName := configs.Value(ctx, "service_name").String()

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(host+":"+port),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(svcName),
		)),
	)
	return tp, nil
}
