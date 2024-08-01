package route

import (
	"database/sql"
	"log"

	"github.com/jacky-htg/erp-proto/go/pb/inventories"
	"github.com/jacky-htg/erp-proto/go/pb/purchases"
	"github.com/jacky-htg/erp-proto/go/pb/users"
	"github.com/jacky-htg/purchase-service/internal/service"
	"google.golang.org/grpc"
)

// GrpcRoute func
func GrpcRoute(grpcServer *grpc.Server, db *sql.DB, log *log.Logger, userConn *grpc.ClientConn, inventoryConn *grpc.ClientConn) {
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
