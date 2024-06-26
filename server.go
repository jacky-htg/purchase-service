package main

import (
	"net"
	"os"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"purchase/internal/config"
	"purchase/internal/middleware"
	"purchase/internal/pkg/db/postgres"
	"purchase/internal/pkg/log/logruslog"
	"purchase/internal/route"
)

const defaultPort = "8002"

func main() {
	// lookup and setup env
	if _, ok := os.LookupEnv("PORT"); !ok {
		config.Setup(".env")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	// init log
	log := logruslog.Init()

	// create postgres database connection
	db, err := postgres.Open()
	if err != nil {
		log.Errorf("connecting to db: %v", err)
		return
	}
	log.Info("connecting to postgresql database")

	defer db.Close()

	// listen tcp port
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	mdInterceptor := middleware.Metadata{}
	serverOptions := []grpc.ServerOption{
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(
			mdInterceptor.Unary(),
		)),
		grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(
			mdInterceptor.Stream(),
		)),
	}

	grpcServer := grpc.NewServer(serverOptions...)

	userConn, err := grpc.NewClient(os.Getenv("USER_SERVICE"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("create user service connection: %v", err)
	}
	defer userConn.Close()

	inventoryConn, err := grpc.NewClient(os.Getenv("INVENTORY_SERVICE"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("create inventory service connection: %v", err)
	}
	defer userConn.Close()

	// routing grpc services
	route.GrpcRoute(grpcServer, db, log, userConn, inventoryConn)

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %s", err)
		return
	}
	log.Info("serve grpc on port: " + port)
}
