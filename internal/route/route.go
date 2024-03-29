package route

import (
	"database/sql"
	"purchase/internal/service"
	"purchase/pb/inventories"
	"purchase/pb/purchases"
	"purchase/pb/users"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

// GrpcRoute func
func GrpcRoute(grpcServer *grpc.Server, db *sql.DB, log *logrus.Entry, userConn *grpc.ClientConn, inventoryConn *grpc.ClientConn) {
	purchaseServer := service.Purchase{
		Db:            db,
		UserClient:    users.NewUserServiceClient((userConn)),
		RegionClient:  users.NewRegionServiceClient(userConn),
		BranchClient:  users.NewBranchServiceClient(userConn),
		ProductClient: inventories.NewProductServiceClient(inventoryConn),
		ReceiveClient: inventories.NewReceiveServiceClient(inventoryConn),
	}
	purchases.RegisterPurchaseServiceServer(grpcServer, &purchaseServer)

	purchaseReturnServer := service.PurchaseReturn{
		Db:            db,
		UserClient:    users.NewUserServiceClient((userConn)),
		RegionClient:  users.NewRegionServiceClient(userConn),
		BranchClient:  users.NewBranchServiceClient(userConn),
		ReceiveClient: inventories.NewReceiveServiceClient(inventoryConn),
	}
	purchases.RegisterPurchaseReturnServiceServer(grpcServer, &purchaseReturnServer)

	supplierServer := service.Supplier{
		Db: db,
	}
	purchases.RegisterSupplierServiceServer(grpcServer, &supplierServer)
}
