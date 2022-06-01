package route

import (
	"database/sql"
	"purchase/internal/service"
	"purchase/pb/purchases"
	"purchase/pb/users"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

// GrpcRoute func
func GrpcRoute(grpcServer *grpc.Server, db *sql.DB, log *logrus.Entry, userConn *grpc.ClientConn) {
	purchaseReturnServer := service.PurchaseReturn{
		Db:           db,
		UserClient:   users.NewUserServiceClient((userConn)),
		RegionClient: users.NewRegionServiceClient(userConn),
		BranchClient: users.NewBranchServiceClient(userConn),
	}
	purchases.RegisterPurchaseReturnServiceServer(grpcServer, &purchaseReturnServer)
}
