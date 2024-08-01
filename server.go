package main

import (
	"log"
	"net"
	"os"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/jacky-htg/erp-pkg/db/postgres"
	"github.com/jacky-htg/purchase-service/internal/config"
	"github.com/jacky-htg/purchase-service/internal/middleware"
	"github.com/jacky-htg/purchase-service/internal/route"
	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
	log := log.New(os.Stdout, "ERROR : ", log.LstdFlags|log.Lmicroseconds|log.Lshortfile)

	// create postgres database connection
	db, err := postgres.Open()
	if err != nil {
		log.Fatalf("connecting to db: %v", err)
		return
	}

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
}
