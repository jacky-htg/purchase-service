package service

import (
	"context"
	"database/sql"
	"purchase/internal/model"
	"purchase/internal/pkg/app"
	"purchase/pb/inventories"
	"purchase/pb/purchases"
	"purchase/pb/users"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PurchaseReturn struct {
	Db            *sql.DB
	UserClient    users.UserServiceClient
	RegionClient  users.RegionServiceClient
	BranchClient  users.BranchServiceClient
	ReceiveClient inventories.ReceiveServiceClient
	purchases.UnimplementedPurchaseReturnServiceServer
}

func (u *PurchaseReturn) PurchaseReturnCreate(ctx context.Context, in *purchases.PurchaseReturn) (*purchases.PurchaseReturn, error) {
	var purchaseReturnModel model.PurchaseReturn
	var err error

	// TODO : if this month any closing account, create transaction for thus month will be blocked

	if len(in.GetBranchId()) == 0 {
		return &purchaseReturnModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid branch")
	}

	if len(in.GetPurchase().GetId()) == 0 {
		return &purchaseReturnModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid purchasing")
	}

	if _, err := time.Parse("2006-01-02T15:04:05.000Z", in.GetReturnDate()); err != nil {
		return &purchaseReturnModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid date")
	}

	ctx, err = app.GetMetadata(ctx)
	if err != nil {
		return &purchaseReturnModel.Pb, err
	}

	// validate not any receiving order yet
	mReceive := model.Receive{Client: u.ReceiveClient}
	hasReceive, err := mReceive.HasTransactionByPurchase(ctx, in.Purchase.Id)
	if err != nil {
		return &purchaseReturnModel.Pb, err
	}

	if hasReceive {
		return &purchaseReturnModel.Pb, status.Error(codes.FailedPrecondition, "Purchase has receive transaction ")
	}

	// validate outstanding purchase
	mPurchase := model.Purchase{Pb: purchases.Purchase{Id: in.Purchase.Id}}
	outstandingPurchaseDetails, err := mPurchase.OutstandingDetail(ctx, u.Db, nil)
	if err != nil {
		return &purchaseReturnModel.Pb, err
	}

	if len(outstandingPurchaseDetails) == 0 {
		return &purchaseReturnModel.Pb, status.Error(codes.FailedPrecondition, "Purchase has been returned ")
	}

	err = mPurchase.Get(ctx, u.Db)
	if err != nil {
		return &purchaseReturnModel.Pb, err
	}

	var sumPrice float64
	var purchaseQty, returnQty int32
	for _, detail := range in.GetDetails() {
		if len(detail.GetProductId()) == 0 {
			return &purchaseReturnModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid product")
		}

		if !u.validateOutstandingDetail(detail, outstandingPurchaseDetails) {
			return &purchaseReturnModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid outstanding product")
		}

		for _, p := range mPurchase.Pb.GetDetails() {
			purchaseQty += p.Quantity
			if p.GetProductId() == detail.ProductId {
				detail.Price = p.Price
				if p.DiscPercentage > 0 {
					detail.DiscPercentage = p.DiscPercentage
					detail.DiscAmount = p.GetPrice() * float64(p.DiscPercentage) / 100
				} else if p.DiscAmount > 0 {
					detail.DiscAmount = p.DiscAmount
				}
				detail.TotalPrice = (detail.Price - detail.DiscAmount) * float64(detail.Quantity)
				break
			}
		}

		returnQty += detail.Quantity
		sumPrice += detail.TotalPrice
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

	in.Price = sumPrice
	if mPurchase.Pb.AdditionalDiscPercentage > 0 {
		in.AdditionalDiscAmount = in.Price * float64(in.AdditionalDiscPercentage) / 100
	} else if mPurchase.Pb.AdditionalDiscAmount > 0 {
		additionalDiscPerQty := mPurchase.Pb.AdditionalDiscAmount / float64(purchaseQty)
		in.AdditionalDiscAmount = additionalDiscPerQty * float64(returnQty)
		returnAdditionalDisc, err := mPurchase.GetReturnAdditionalDisc(ctx, u.Db)
		if err != nil {
			return &purchaseReturnModel.Pb, status.Error(codes.Internal, "Error get return additional disc")
		}
		remainingAdditionalDisc := mPurchase.Pb.AdditionalDiscAmount - returnAdditionalDisc
		if in.AdditionalDiscAmount > remainingAdditionalDisc {
			in.AdditionalDiscAmount = remainingAdditionalDisc
		}
	}
	in.TotalPrice = in.Price - in.AdditionalDiscAmount

	purchaseReturnModel.Pb = purchases.PurchaseReturn{
		BranchId:                 in.GetBranchId(),
		BranchName:               mBranch.Pb.GetName(),
		Code:                     in.GetCode(),
		ReturnDate:               in.GetReturnDate(),
		Purchase:                 in.GetPurchase(),
		Remark:                   in.GetRemark(),
		Price:                    in.GetPrice(),
		AdditionalDiscAmount:     in.GetAdditionalDiscAmount(),
		AdditionalDiscPercentage: in.GetAdditionalDiscPercentage(),
		TotalPrice:               in.GetTotalPrice(),
		Details:                  in.GetDetails(),
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

	err = tx.Commit()
	if err != nil {
		return &purchaseReturnModel.Pb, status.Error(codes.Internal, "Error when commit transaction")
	}

	return &purchaseReturnModel.Pb, nil
}

func (u *PurchaseReturn) PurchaseReturnView(ctx context.Context, in *purchases.Id) (*purchases.PurchaseReturn, error) {
	var purchaseReturnModel model.PurchaseReturn
	var err error

	if len(in.GetId()) == 0 {
		return &purchaseReturnModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid id")
	}
	purchaseReturnModel.Pb.Id = in.GetId()

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

func (u *PurchaseReturn) PurchaseReturnUpdate(ctx context.Context, in *purchases.PurchaseReturn) (*purchases.PurchaseReturn, error) {
	var purchaseReturnModel model.PurchaseReturn
	var err error

	// TODO : if this month any closing stock, create transaction for thus month will be blocked

	if len(in.GetId()) == 0 {
		return &purchaseReturnModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid id")
	}
	purchaseReturnModel.Pb.Id = in.GetId()

	ctx, err = app.GetMetadata(ctx)
	if err != nil {
		return &purchaseReturnModel.Pb, err
	}

	// validate not any receiving order yet
	mReceive := model.Receive{Client: u.ReceiveClient}
	hasReceive, err := mReceive.HasTransactionByPurchase(ctx, in.Purchase.Id)
	if err != nil {
		return &purchaseReturnModel.Pb, err
	}

	if hasReceive {
		return &purchaseReturnModel.Pb, status.Error(codes.FailedPrecondition, "Purchase has receive transaction ")
	}

	// validate outstanding purchase
	mPurchase := model.Purchase{Pb: purchases.Purchase{Id: in.Purchase.Id}}
	purchaseReturnId := in.GetId()
	outstandingPurchaseDetails, err := mPurchase.OutstandingDetail(ctx, u.Db, &purchaseReturnId)
	if err != nil {
		return &purchaseReturnModel.Pb, err
	}

	if len(outstandingPurchaseDetails) == 0 {
		return &purchaseReturnModel.Pb, status.Error(codes.FailedPrecondition, "Purchase has been returned ")
	}

	err = mPurchase.Get(ctx, u.Db)
	if err != nil {
		return &purchaseReturnModel.Pb, err
	}

	err = purchaseReturnModel.Get(ctx, u.Db)
	if err != nil {
		return &purchaseReturnModel.Pb, err
	}

	if _, err := time.Parse("2006-01-02T15:04:05.000Z", in.GetReturnDate()); err == nil {
		purchaseReturnModel.Pb.ReturnDate = in.GetReturnDate()
	}

	tx, err := u.Db.BeginTx(ctx, nil)
	if err != nil {
		return &purchaseReturnModel.Pb, status.Errorf(codes.Internal, "begin transaction: %v", err)
	}

	var sumPrice float64
	var purchaseQty, returnQty int32
	// var newDetails []*purchases.PurchaseReturnDetail
	for _, detail := range in.GetDetails() {
		if len(detail.GetProductId()) == 0 {
			tx.Rollback()
			return &purchaseReturnModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid product")
		}

		if !u.validateOutstandingDetail(detail, outstandingPurchaseDetails) {
			return &purchaseReturnModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid outstanding product")
		}

		if len(detail.GetId()) > 0 {
			for _, p := range mPurchase.Pb.GetDetails() {
				purchaseQty += p.Quantity
				if p.GetProductId() == detail.ProductId {
					break
				}
			}

			returnQty += detail.Quantity
			detail.TotalPrice = (detail.Price - detail.DiscAmount) * float64(detail.Quantity)
			sumPrice += detail.TotalPrice

			// operasi update
			purchaseReturnDetailModel := model.PurchaseReturnDetail{
				Pb: purchases.PurchaseReturnDetail{
					Id:               detail.Id,
					Quantity:         detail.Quantity,
					TotalPrice:       detail.TotalPrice,
					PurchaseReturnId: purchaseReturnModel.Pb.Id,
				},
			}

			err = purchaseReturnDetailModel.Update(ctx, tx)
			if err != nil {
				tx.Rollback()
				return &purchaseReturnModel.Pb, err
			}

			// newDetails = append(newDetails, &purchaseReturnDetailModel.Pb)
			for index, data := range purchaseReturnModel.Pb.GetDetails() {
				if data.GetId() == detail.GetId() {
					purchaseReturnModel.Pb.Details = append(purchaseReturnModel.Pb.Details[:index], purchaseReturnModel.Pb.Details[index+1:]...)
					break
				}
			}

		} else {
			for _, p := range mPurchase.Pb.GetDetails() {
				purchaseQty += p.Quantity
				if p.GetProductId() == detail.ProductId {
					detail.Price = p.Price
					if p.DiscPercentage > 0 {
						detail.DiscPercentage = p.DiscPercentage
						detail.DiscAmount = p.GetPrice() * float64(p.DiscPercentage) / 100
					} else if p.DiscAmount > 0 {
						detail.DiscAmount = p.DiscAmount
					}
					detail.TotalPrice = (detail.Price - detail.DiscAmount) * float64(detail.Quantity)
					break
				}
			}

			returnQty += detail.Quantity
			sumPrice += detail.TotalPrice

			// operasi insert
			purchaseReturnDetailModel := model.PurchaseReturnDetail{Pb: purchases.PurchaseReturnDetail{
				PurchaseReturnId: purchaseReturnModel.Pb.GetId(),
				ProductId:        detail.GetProductId(),
				Quantity:         detail.GetQuantity(),
				Price:            detail.GetPrice(),
				DiscAmount:       detail.GetDiscAmount(),
				DiscPercentage:   detail.GetDiscPercentage(),
				TotalPrice:       detail.GetTotalPrice(),
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

			// newDetails = append(newDetails, &purchaseReturnDetailModel.Pb)
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

	err = purchaseReturnModel.Update(ctx, tx)
	if err != nil {
		tx.Rollback()
		return &purchaseReturnModel.Pb, err
	}

	err = tx.Commit()
	if err != nil {
		return &purchaseReturnModel.Pb, status.Error(codes.Internal, "failed commit transaction")
	}

	return &purchaseReturnModel.Pb, nil
}

func (u *PurchaseReturn) PurchaseReturnList(in *purchases.ListPurchaseReturnRequest, stream purchases.PurchaseReturnService_PurchaseReturnListServer) error {
	ctx := stream.Context()
	ctx, err := app.GetMetadata(ctx)
	if err != nil {
		return err
	}

	var purchaseReturnModel model.PurchaseReturn
	query, paramQueries, paginationResponse, err := purchaseReturnModel.ListQuery(ctx, u.Db, in)
	if err != nil {
		return err
	}

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
			&pbPurchaseReturn.Price, &pbPurchaseReturn.AdditionalDiscAmount, &pbPurchaseReturn.AdditionalDiscPercentage, &pbPurchaseReturn.TotalPrice,
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

func (u *PurchaseReturn) validateOutstandingDetail(in *purchases.PurchaseReturnDetail, outstanding []*purchases.PurchaseDetail) bool {
	isValid := false
	for _, out := range outstanding {
		if in.ProductId == out.ProductId && in.Quantity <= out.Quantity {
			isValid = true
			break
		}
	}

	return isValid
}
