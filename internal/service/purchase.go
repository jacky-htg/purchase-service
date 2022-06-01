package service

import (
	"context"
	"database/sql"
	"purchase/pb/users"
	"time"

	"purchase/internal/model"
	"purchase/pb/purchases"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Purchase struct {
	Db           *sql.DB
	UserClient   users.UserServiceClient
	RegionClient users.RegionServiceClient
	BranchClient users.BranchServiceClient
}

func (u *Purchase) Create(ctx context.Context, in *purchases.Purchase) (*purchases.Purchase, error) {
	var purchaseModel model.Purchase
	var err error

	// TODO : if this month any closing stock, create transaction for thus month will be blocked

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

	ctx, err = getMetadata(ctx)
	if err != nil {
		return &purchaseModel.Pb, err
	}

	for _, detail := range in.GetDetails() {
		// product validation
		if len(detail.GetProductId()) == 0 {
			return &purchaseModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid product")
		}

		// TODO : validation product by call grpc product

	}

	err = isYourBranch(ctx, u.UserClient, u.RegionClient, u.BranchClient, in.GetBranchId())
	if err != nil {
		return &purchaseModel.Pb, err
	}

	branch, err := getBranch(ctx, u.BranchClient, in.GetBranchId())
	if err != nil {
		return &purchaseModel.Pb, err
	}

	purchaseModel.Pb = purchases.Purchase{
		BranchId:     in.GetBranchId(),
		BranchName:   branch.GetName(),
		Code:         in.GetCode(),
		PurchaseDate: in.GetPurchaseDate(),
		Supplier:     in.GetSupplier(),
		Remark:       in.GetRemark(),
		Details:      in.GetDetails(),
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

	// TODO : if this month any closing stock, create transaction for thus month will be blocked

	// basic validation
	{
		if len(in.GetId()) == 0 {
			return &purchaseModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid id")
		}
		purchaseModel.Pb.Id = in.GetId()
	}

	// TODO : if any return do update will be blocked

	ctx, err = getMetadata(ctx)
	if err != nil {
		return &purchaseModel.Pb, err
	}

	err = purchaseModel.Get(ctx, u.Db)
	if err != nil {
		return &purchaseModel.Pb, err
	}

	if len(in.GetSupplier().Id) > 0 {
		purchaseModel.Pb.GetSupplier().Id = in.GetSupplier().GetId()
	}

	if _, err := time.Parse("2006-01-02T15:04:05.000Z", in.GetPurchaseDate()); err == nil {
		purchaseModel.Pb.PurchaseDate = in.GetPurchaseDate()
	}

	tx, err := u.Db.BeginTx(ctx, nil)
	if err != nil {
		return &purchaseModel.Pb, status.Errorf(codes.Internal, "begin transaction: %v", err)
	}

	err = purchaseModel.Update(ctx, tx)
	if err != nil {
		tx.Rollback()
		return &purchaseModel.Pb, err
	}

	var newDetails []*purchases.PurchaseDetail
	for _, detail := range in.GetDetails() {
		// product validation
		if len(detail.GetProductId()) == 0 {
			tx.Rollback()
			return &purchaseModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid product")
		}

		// TODO : call grpc product

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
				PurchaseId:  purchaseModel.Pb.GetId(),
				ProductId:   detail.ProductId,
				ProductCode: detail.ProductCode,
				ProductName: detail.ProductName,
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

	ctx, err = getMetadata(ctx)
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
	ctx, err := getMetadata(ctx)
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
		err := contextError(ctx)
		if err != nil {
			return err
		}

		var pbPurchase purchases.Purchase
		var companyID string
		var createdAt, updatedAt time.Time
		err = rows.Scan(&pbPurchase.Id, &companyID, &pbPurchase.BranchId, &pbPurchase.BranchName,
			&pbPurchase.Code, &pbPurchase.PurchaseDate, &pbPurchase.Remark,
			&createdAt, &pbPurchase.CreatedBy, &updatedAt, &pbPurchase.UpdatedBy)
		if err != nil {
			return status.Errorf(codes.Internal, "scan data: %v", err)
		}

		pbPurchase.CreatedAt = createdAt.String()
		pbPurchase.UpdatedAt = updatedAt.String()

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
