package service

import (
	"context"
	"database/sql"
	"purchase/internal/model"
	"purchase/internal/pkg/app"
	"purchase/pb/purchases"
	"purchase/pb/users"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PurchaseReturn struct {
	Db           *sql.DB
	UserClient   users.UserServiceClient
	RegionClient users.RegionServiceClient
	BranchClient users.BranchServiceClient
	purchases.UnimplementedPurchaseReturnServiceServer
}

func (u *PurchaseReturn) Create(ctx context.Context, in *purchases.PurchaseReturn) (*purchases.PurchaseReturn, error) {
	var purchaseReturnModel model.PurchaseReturn
	var err error

	// TODO : if this month any closing account, create transaction for thus month will be blocked

	// basic validation
	{
		if len(in.GetBranchId()) == 0 {
			return &purchaseReturnModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid branch")
		}

		if len(in.GetPurchase().GetId()) == 0 {
			return &purchaseReturnModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid purchasing")
		}

		if _, err := time.Parse("2006-01-02T15:04:05.000Z", in.GetReturnDate()); err != nil {
			return &purchaseReturnModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid date")
		}
	}

	ctx, err = app.GetMetadata(ctx)
	if err != nil {
		return &purchaseReturnModel.Pb, err
	}

	// TODO : validate not any receiving order yet

	// TODO : validate outstanding purchase

	for _, detail := range in.GetDetails() {
		// product validation
		if len(detail.GetProductId()) == 0 {
			return &purchaseReturnModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid product")
		}

		// TODO : validate outstanding purchase details

		// TODO : call product from inventory grpc
		detail.ProductCode = ""
		detail.ProductName = ""
	}

	mBranch := model.Branch{
		UserClient:   u.UserClient,
		RegionClient: u.RegionClient,
		BranchClient: u.BranchClient,
		Id:           in.GetBranchId(),
	}
	err = mBranch.IsYourBranch(ctx)
	if err != nil {
		return &purchaseReturnModel.Pb, err
	}

	err = mBranch.Get(ctx)
	if err != nil {
		return &purchaseReturnModel.Pb, err
	}

	purchaseReturnModel.Pb = purchases.PurchaseReturn{
		BranchId:   in.GetBranchId(),
		BranchName: mBranch.Pb.GetName(),
		Code:       in.GetCode(),
		ReturnDate: in.GetReturnDate(),
		Purchase:   in.GetPurchase(),
		Remark:     in.GetRemark(),
		Details:    in.GetDetails(),
	}

	tx, err := u.Db.BeginTx(ctx, nil)
	if err != nil {
		return &purchaseReturnModel.Pb, err
	}

	err = purchaseReturnModel.Create(ctx, tx)
	if err != nil {
		tx.Rollback()
		return &purchaseReturnModel.Pb, err
	}

	tx.Commit()

	return &purchaseReturnModel.Pb, nil
}

func (u *PurchaseReturn) View(ctx context.Context, in *purchases.Id) (*purchases.PurchaseReturn, error) {
	var purchaseReturnModel model.PurchaseReturn
	var err error

	// basic validation
	{
		if len(in.GetId()) == 0 {
			return &purchaseReturnModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid id")
		}
		purchaseReturnModel.Pb.Id = in.GetId()
	}

	ctx, err = app.GetMetadata(ctx)
	if err != nil {
		return &purchaseReturnModel.Pb, err
	}

	err = purchaseReturnModel.Get(ctx, u.Db)
	if err != nil {
		return &purchaseReturnModel.Pb, err
	}

	return &purchaseReturnModel.Pb, nil
}

func (u *PurchaseReturn) Update(ctx context.Context, in *purchases.PurchaseReturn) (*purchases.PurchaseReturn, error) {
	var purchaseReturnModel model.PurchaseReturn
	var err error

	// TODO : if this month any closing stock, create transaction for thus month will be blocked

	// basic validation
	{
		if len(in.GetId()) == 0 {
			return &purchaseReturnModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid id")
		}
		purchaseReturnModel.Pb.Id = in.GetId()
	}

	ctx, err = app.GetMetadata(ctx)
	if err != nil {
		return &purchaseReturnModel.Pb, err
	}

	err = purchaseReturnModel.Get(ctx, u.Db)
	if err != nil {
		return &purchaseReturnModel.Pb, err
	}

	if len(in.GetPurchase().GetId()) > 0 {
		purchaseReturnModel.Pb.Purchase = in.GetPurchase()
	}

	if _, err := time.Parse("2006-01-02T15:04:05.000Z", in.GetReturnDate()); err == nil {
		purchaseReturnModel.Pb.ReturnDate = in.GetReturnDate()
	}

	tx, err := u.Db.BeginTx(ctx, nil)
	if err != nil {
		return &purchaseReturnModel.Pb, status.Errorf(codes.Internal, "begin transaction: %v", err)
	}

	err = purchaseReturnModel.Update(ctx, tx)
	if err != nil {
		tx.Rollback()
		return &purchaseReturnModel.Pb, err
	}

	var newDetails []*purchases.PurchaseReturnDetail
	for _, detail := range in.GetDetails() {
		// product validation
		if len(detail.GetProductId()) == 0 {
			tx.Rollback()
			return &purchaseReturnModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid product")
		}

		// TODO : CALL GRPC PRODUCT FROM INVENTORY SERVICE
		detail.ProductCode = ""
		detail.ProductName = ""

		// TODO : validation outstanding purchase detail

		if len(detail.GetId()) > 0 {
			// operasi update
			purchaseReturnDetailModel := model.PurchaseReturnDetail{}
			purchaseReturnDetailModel.Pb.Id = detail.GetId()
			purchaseReturnDetailModel.Pb.PurchaseReturnId = purchaseReturnModel.Pb.GetId()
			err = purchaseReturnDetailModel.Get(ctx, tx)
			if err != nil {
				tx.Rollback()
				return &purchaseReturnModel.Pb, err
			}

			purchaseReturnDetailModel.PbPurchaseReturn = purchases.PurchaseReturn{
				Id:         purchaseReturnModel.Pb.Id,
				BranchId:   purchaseReturnModel.Pb.BranchId,
				BranchName: purchaseReturnModel.Pb.BranchName,
				Purchase:   purchaseReturnModel.Pb.GetPurchase(),
				Code:       purchaseReturnModel.Pb.Code,
				ReturnDate: purchaseReturnModel.Pb.ReturnDate,
				Remark:     purchaseReturnModel.Pb.Remark,
				CreatedAt:  purchaseReturnModel.Pb.CreatedAt,
				CreatedBy:  purchaseReturnModel.Pb.CreatedBy,
				UpdatedAt:  purchaseReturnModel.Pb.UpdatedAt,
				UpdatedBy:  purchaseReturnModel.Pb.UpdatedBy,
				Details:    purchaseReturnModel.Pb.Details,
			}
			err = purchaseReturnDetailModel.Update(ctx, tx)
			if err != nil {
				tx.Rollback()
				return &purchaseReturnModel.Pb, err
			}

			newDetails = append(newDetails, &purchaseReturnDetailModel.Pb)
			for index, data := range purchaseReturnModel.Pb.GetDetails() {
				if data.GetId() == detail.GetId() {
					purchaseReturnModel.Pb.Details = append(purchaseReturnModel.Pb.Details[:index], purchaseReturnModel.Pb.Details[index+1:]...)
					break
				}
			}

		} else {
			// operasi insert
			purchaseReturnDetailModel := model.PurchaseReturnDetail{Pb: purchases.PurchaseReturnDetail{
				PurchaseReturnId: purchaseReturnModel.Pb.GetId(),
				ProductId:        detail.GetProductId(),
				ProductCode:      detail.GetProductCode(),
				ProductName:      detail.GetProductName(),
			}}
			purchaseReturnDetailModel.PbPurchaseReturn = purchases.PurchaseReturn{
				Id:         purchaseReturnModel.Pb.Id,
				BranchId:   purchaseReturnModel.Pb.BranchId,
				BranchName: purchaseReturnModel.Pb.BranchName,
				Purchase:   purchaseReturnModel.Pb.GetPurchase(),
				Code:       purchaseReturnModel.Pb.Code,
				ReturnDate: purchaseReturnModel.Pb.ReturnDate,
				Remark:     purchaseReturnModel.Pb.Remark,
				CreatedAt:  purchaseReturnModel.Pb.CreatedAt,
				CreatedBy:  purchaseReturnModel.Pb.CreatedBy,
				UpdatedAt:  purchaseReturnModel.Pb.UpdatedAt,
				UpdatedBy:  purchaseReturnModel.Pb.UpdatedBy,
				Details:    purchaseReturnModel.Pb.Details,
			}
			err = purchaseReturnDetailModel.Create(ctx, tx)
			if err != nil {
				tx.Rollback()
				return &purchaseReturnModel.Pb, err
			}

			newDetails = append(newDetails, &purchaseReturnDetailModel.Pb)
		}
	}

	// delete existing detail
	for _, data := range purchaseReturnModel.Pb.GetDetails() {
		purchaseReturnDetailModel := model.PurchaseReturnDetail{Pb: purchases.PurchaseReturnDetail{
			PurchaseReturnId: purchaseReturnModel.Pb.GetId(),
			Id:               data.GetId(),
		}}
		err = purchaseReturnDetailModel.Delete(ctx, tx)
		if err != nil {
			tx.Rollback()
			return &purchaseReturnModel.Pb, err
		}
	}

	tx.Commit()

	return &purchaseReturnModel.Pb, nil
}

func (u *PurchaseReturn) List(in *purchases.ListPurchaseReturnRequest, stream purchases.PurchaseReturnService_PurchaseReturnListServer) error {
	ctx := stream.Context()
	ctx, err := app.GetMetadata(ctx)
	if err != nil {
		return err
	}

	var purchaseReturnModel model.PurchaseReturn
	query, paramQueries, paginationResponse, err := purchaseReturnModel.ListQuery(ctx, u.Db, in)

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

		var pbPurchaseReturn purchases.PurchaseReturn
		var companyID string
		var createdAt, updatedAt time.Time
		err = rows.Scan(&pbPurchaseReturn.Id, &companyID, &pbPurchaseReturn.BranchId, &pbPurchaseReturn.BranchName,
			&pbPurchaseReturn.Code, &pbPurchaseReturn.ReturnDate, &pbPurchaseReturn.Remark,
			&createdAt, &pbPurchaseReturn.CreatedBy, &updatedAt, &pbPurchaseReturn.UpdatedBy)
		if err != nil {
			return status.Errorf(codes.Internal, "scan data: %v", err)
		}

		pbPurchaseReturn.CreatedAt = createdAt.String()
		pbPurchaseReturn.UpdatedAt = updatedAt.String()

		res := &purchases.ListPurchaseReturnResponse{
			Pagination:     paginationResponse,
			PurchaseReturn: &pbPurchaseReturn,
		}

		err = stream.Send(res)
		if err != nil {
			return status.Errorf(codes.Unknown, "cannot send stream response: %v", err)
		}
	}
	return nil
}
