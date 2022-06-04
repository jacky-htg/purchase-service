package service

import (
	"context"
	"database/sql"
	"purchase/pb/inventories"
	"purchase/pb/users"
	"time"

	"purchase/internal/model"
	"purchase/internal/pkg/app"
	"purchase/pb/purchases"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Purchase struct {
	Db              *sql.DB
	UserClient      users.UserServiceClient
	RegionClient    users.RegionServiceClient
	BranchClient    users.BranchServiceClient
	ProductClient   inventories.ProductServiceClient
	ReceivingClient inventories.ReceiveServiceClient
	purchases.UnimplementedPurchaseServiceServer
}

func (u *Purchase) Create(ctx context.Context, in *purchases.Purchase) (*purchases.Purchase, error) {
	var purchaseModel model.Purchase
	var err error

	// TODO : if this month any closing account, create transaction for thus month will be blocked

	// basic validation
	{
		if len(in.GetBranchId()) == 0 {
			return &purchaseModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid branch")
		}

		if len(in.GetSupplier().Id) == 0 {
			return &purchaseModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid supplier")
		}

		if _, err := time.Parse("2006-01-02T15:04:05.000Z", in.GetPurchaseDate()); err != nil {
			return &purchaseModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid date")
		}
	}

	ctx, err = app.GetMetadata(ctx)
	if err != nil {
		return &purchaseModel.Pb, err
	}

	var sumPrice float64
	for i, detail := range in.GetDetails() {
		// product validation
		if len(detail.GetProductId()) == 0 {
			return &purchaseModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid product")
		}

		{
			mProduct := model.Product{Client: u.ProductClient, Id: detail.GetProductId()}
			if product, err := mProduct.Get(ctx); err != nil {
				return &purchaseModel.Pb, err
			} else {
				in.GetDetails()[i].ProductCode = product.GetCode()
				in.GetDetails()[i].ProductName = product.GetName()
			}
		}

		sumPrice += detail.GetPrice()
	}

	mBranch := model.Branch{
		UserClient:   u.UserClient,
		RegionClient: u.RegionClient,
		BranchClient: u.BranchClient,
		Id:           in.GetBranchId(),
	}
	err = mBranch.IsYourBranch(ctx)
	if err != nil {
		return &purchaseModel.Pb, err
	}

	branch, err := mBranch.Get(ctx)
	if err != nil {
		return &purchaseModel.Pb, err
	}

	purchaseModel.Pb = purchases.Purchase{
		BranchId:                   in.GetBranchId(),
		BranchName:                 branch.GetName(),
		Code:                       in.GetCode(),
		PurchaseDate:               in.GetPurchaseDate(),
		Supplier:                   in.GetSupplier(),
		Remark:                     in.GetRemark(),
		Price:                      sumPrice,
		AdditionalDiscAmount:       in.GetAdditionalDiscAmount(),
		AdditionalDiscProsentation: in.GetAdditionalDiscProsentation(),
		Details:                    in.GetDetails(),
	}

	tx, err := u.Db.BeginTx(ctx, nil)
	if err != nil {
		return &purchaseModel.Pb, err
	}

	err = purchaseModel.Create(ctx, tx)
	if err != nil {
		tx.Rollback()
		return &purchaseModel.Pb, err
	}

	tx.Commit()

	return &purchaseModel.Pb, nil
}

func (u *Purchase) Update(ctx context.Context, in *purchases.Purchase) (*purchases.Purchase, error) {
	var purchaseModel model.Purchase
	var err error

	// TODO : if this month any closing account, create transaction for thus month will be blocked

	// basic validation
	{
		if len(in.GetId()) == 0 {
			return &purchaseModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid id")
		}
		purchaseModel.Pb.Id = in.GetId()
	}

	// if any return, do update will be blocked
	{
		purchaseReturnModel := model.PurchaseReturn{
			Pb: purchases.PurchaseReturn{
				Purchase: &purchases.Purchase{Id: in.GetId()},
			},
		}
		if hasReturn, err := purchaseReturnModel.HasReturn(ctx, u.Db); err != nil {
			return &purchaseModel.Pb, err
		} else if hasReturn {
			return &purchaseModel.Pb, status.Error(codes.PermissionDenied, "Can not updated because the purchase has return transaction")
		}
	}

	// if any receiving transaction, do update will be blocked
	mReceive := model.Receive{Client: u.ReceivingClient}
	if hasReceive, err := mReceive.HasTransactionByPurchase(ctx, in.GetId()); err != nil {
		return &purchaseModel.Pb, err
	} else if hasReceive {
		return &purchaseModel.Pb, status.Error(codes.PermissionDenied, "Can not updated because the purchase has receiving transaction")
	}

	ctx, err = app.GetMetadata(ctx)
	if err != nil {
		return &purchaseModel.Pb, err
	}

	err = purchaseModel.Get(ctx, u.Db)
	if err != nil {
		return &purchaseModel.Pb, err
	}

	// update field of purchase header
	{
		if len(in.GetSupplier().Id) > 0 {
			purchaseModel.Pb.GetSupplier().Id = in.GetSupplier().GetId()
		}

		if _, err := time.Parse("2006-01-02T15:04:05.000Z", in.GetPurchaseDate()); err == nil {
			purchaseModel.Pb.PurchaseDate = in.GetPurchaseDate()
		}

		if len(in.GetRemark()) > 0 {
			purchaseModel.Pb.Remark = in.GetRemark()
		}
	}

	tx, err := u.Db.BeginTx(ctx, nil)
	if err != nil {
		return &purchaseModel.Pb, status.Errorf(codes.Internal, "begin transaction: %v", err)
	}

	var newDetails []*purchases.PurchaseDetail
	var sumPrice float64
	for i, detail := range in.GetDetails() {
		sumPrice += detail.GetPrice()
		// product validation
		if len(detail.GetProductId()) == 0 {
			tx.Rollback()
			return &purchaseModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid product")
		}

		// call grpc product
		mProduct := model.Product{Client: u.ProductClient, Id: detail.GetProductId()}
		product, err := mProduct.Get(ctx)
		if err != nil {
			return &purchaseModel.Pb, err
		} else {
			in.GetDetails()[i].ProductCode = product.GetCode()
			in.GetDetails()[i].ProductName = product.GetName()
		}

		if len(detail.GetId()) > 0 {
			for index, data := range purchaseModel.Pb.GetDetails() {
				if data.GetId() == detail.GetId() {
					purchaseModel.Pb.Details = append(purchaseModel.Pb.Details[:index], purchaseModel.Pb.Details[index+1:]...)
					break
				}
			}
		} else {
			// operasi insert
			purchaseDetailModel := model.PurchaseDetail{Pb: purchases.PurchaseDetail{
				PurchaseId:       purchaseModel.Pb.GetId(),
				ProductId:        detail.ProductId,
				ProductCode:      product.GetCode(),
				ProductName:      product.GetName(),
				Price:            detail.GetPrice(),
				DiscAmount:       detail.GetDiscAmount(),
				DiscProsentation: detail.GetDiscProsentation(),
			}}
			purchaseDetailModel.PbPurchase = purchases.Purchase{
				Id:           purchaseModel.Pb.Id,
				BranchId:     purchaseModel.Pb.BranchId,
				BranchName:   purchaseModel.Pb.BranchName,
				Supplier:     purchaseModel.Pb.GetSupplier(),
				Code:         purchaseModel.Pb.Code,
				PurchaseDate: purchaseModel.Pb.PurchaseDate,
				Remark:       purchaseModel.Pb.Remark,
				CreatedAt:    purchaseModel.Pb.CreatedAt,
				CreatedBy:    purchaseModel.Pb.CreatedBy,
				UpdatedAt:    purchaseModel.Pb.UpdatedAt,
				UpdatedBy:    purchaseModel.Pb.UpdatedBy,
				Details:      purchaseModel.Pb.Details,
			}
			err = purchaseDetailModel.Create(ctx, tx)
			if err != nil {
				tx.Rollback()
				return &purchaseModel.Pb, err
			}

			newDetails = append(newDetails, &purchaseDetailModel.Pb)
		}
	}

	// delete existing detail
	for _, data := range purchaseModel.Pb.GetDetails() {
		purchaseDetailModel := model.PurchaseDetail{Pb: purchases.PurchaseDetail{
			PurchaseId: purchaseModel.Pb.GetId(),
			Id:         data.GetId(),
		}}
		err = purchaseDetailModel.Delete(ctx, tx)
		if err != nil {
			tx.Rollback()
			return &purchaseModel.Pb, err
		}
	}

	purchaseModel.Pb.Price = sumPrice
	err = purchaseModel.Update(ctx, tx)
	if err != nil {
		tx.Rollback()
		return &purchaseModel.Pb, err
	}

	tx.Commit()

	return &purchaseModel.Pb, nil
}

// View Purchase
func (u *Purchase) View(ctx context.Context, in *purchases.Id) (*purchases.Purchase, error) {
	var purchaseModel model.Purchase
	var err error

	// basic validation
	{
		if len(in.GetId()) == 0 {
			return &purchaseModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid id")
		}
		purchaseModel.Pb.Id = in.GetId()
	}

	ctx, err = app.GetMetadata(ctx)
	if err != nil {
		return &purchaseModel.Pb, err
	}

	err = purchaseModel.Get(ctx, u.Db)
	if err != nil {
		return &purchaseModel.Pb, err
	}

	return &purchaseModel.Pb, nil
}

// List Purchase
func (u *Purchase) List(in *purchases.ListPurchaseRequest, stream purchases.PurchaseService_PurchaseListServer) error {
	ctx := stream.Context()
	ctx, err := app.GetMetadata(ctx)
	if err != nil {
		return err
	}

	var purchaseModel model.Purchase
	query, paramQueries, paginationResponse, err := purchaseModel.ListQuery(ctx, u.Db, in)

	rows, err := u.Db.QueryContext(ctx, query, paramQueries...)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	defer rows.Close()
	paginationResponse.Pagination = in.GetPagination()

	for rows.Next() {
		err := app.ContextError(ctx)
		if err != nil {
			return err
		}

		var pbPurchase purchases.Purchase
		var companyID string
		var createdAt, updatedAt time.Time
		var addDiscProsentation *float32
		err = rows.Scan(&pbPurchase.Id, &companyID, &pbPurchase.BranchId, &pbPurchase.BranchName,
			&pbPurchase.Code, &pbPurchase.PurchaseDate, &pbPurchase.Remark,
			&pbPurchase.Price, &pbPurchase.AdditionalDiscAmount, &addDiscProsentation,
			&createdAt, &pbPurchase.CreatedBy, &updatedAt, &pbPurchase.UpdatedBy)
		if err != nil {
			return status.Errorf(codes.Internal, "scan data: %v", err)
		}

		pbPurchase.CreatedAt = createdAt.String()
		pbPurchase.UpdatedAt = updatedAt.String()
		if addDiscProsentation == nil {
			pbPurchase.AdditionalDiscProsentation = 0
		} else {
			pbPurchase.AdditionalDiscProsentation = *addDiscProsentation
		}

		res := &purchases.ListPurchaseResponse{
			Pagination: paginationResponse,
			Purchase:   &pbPurchase,
		}

		err = stream.Send(res)
		if err != nil {
			return status.Errorf(codes.Unknown, "cannot send stream response: %v", err)
		}
	}
	return nil
}
