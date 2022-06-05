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

	products, err := u.createValidation(ctx, in)
	if err != nil {
		return &purchaseModel.Pb, err
	}

	ctx, err = app.GetMetadata(ctx)
	if err != nil {
		return &purchaseModel.Pb, err
	}

	var sumPrice float64
	for i, detail := range in.GetDetails() {
		for _, p := range products {
			if detail.GetProductId() == p.Product.GetId() {
				in.GetDetails()[i].ProductCode = p.Product.GetCode()
				in.GetDetails()[i].ProductName = p.Product.GetName()
			}
		}

		if detail.DiscPercentage > 0 {
			in.GetDetails()[i].DiscAmount = detail.GetPrice() * float64(detail.DiscPercentage) / 100
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

	err = mBranch.Get(ctx)
	if err != nil {
		return &purchaseModel.Pb, err
	}

	if in.GetAdditionalDiscPercentage() > 0 {
		in.AdditionalDiscAmount = sumPrice * float64(in.GetAdditionalDiscPercentage()) / 100
	}
	purchaseModel.Pb = purchases.Purchase{
		BranchId:                 in.GetBranchId(),
		BranchName:               mBranch.Pb.GetName(),
		Code:                     in.GetCode(),
		PurchaseDate:             in.GetPurchaseDate(),
		Supplier:                 in.GetSupplier(),
		Remark:                   in.GetRemark(),
		TotalPrice:               sumPrice,
		AdditionalDiscAmount:     in.GetAdditionalDiscAmount(),
		AdditionalDiscPercentage: in.GetAdditionalDiscPercentage(),
		Details:                  in.GetDetails(),
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

		if in.GetAdditionalDiscPercentage() > 0 {
			purchaseModel.Pb.AdditionalDiscPercentage = in.AdditionalDiscPercentage
		}
	}

	tx, err := u.Db.BeginTx(ctx, nil)
	if err != nil {
		return &purchaseModel.Pb, status.Errorf(codes.Internal, "begin transaction: %v", err)
	}

	var newDetails []*purchases.PurchaseDetail
	var sumPrice float64
	var productIds []string
	for _, detail := range in.GetDetails() {
		sumPrice += detail.GetPrice()
		if len(detail.GetProductId()) == 0 {
			tx.Rollback()
			return &purchaseModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid product")
		}

		productIds = append(productIds, detail.GetProductId())
	}

	mProduct := model.Product{
		Client: u.ProductClient,
		Pb:     &inventories.Product{},
	}

	inProductList := inventories.ListProductRequest{
		Ids: productIds,
	}
	products, err := mProduct.List(ctx, &inProductList)

	if len(products) != len(productIds) {
		return &purchaseModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid product")
	}

	for i, detail := range in.GetDetails() {
		for _, p := range products {
			if detail.GetProductId() == p.Product.GetId() {
				in.GetDetails()[i].ProductCode = p.Product.GetCode()
				in.GetDetails()[i].ProductName = p.Product.GetName()
			}
		}

		if detail.DiscPercentage > 0 {
			in.GetDetails()[i].DiscAmount = detail.GetPrice() * float64(detail.DiscPercentage) / 100
		}
		sumPrice += detail.GetPrice()

		if len(detail.GetId()) > 0 {
			for index, data := range purchaseModel.Pb.GetDetails() {
				if data.GetId() == detail.GetId() {
					purchaseModel.Pb.Details = append(purchaseModel.Pb.Details[:index], purchaseModel.Pb.Details[index+1:]...)
					// update detail
					if detail.Price > 0 {
						data.Price = detail.Price
					}

					if detail.DiscAmount > 0 {
						data.DiscAmount = detail.DiscAmount
					}

					if detail.Quantity > 0 {
						data.Quantity = detail.Quantity
					}

					if detail.DiscPercentage > 0 {
						data.DiscPercentage = detail.DiscPercentage
						data.DiscAmount = data.Price * float64(data.DiscPercentage) / 100
					}

					var purchaseDetailModel model.PurchaseDetail
					purchaseDetailModel.SetPbFromPointer(data)
					if err := purchaseDetailModel.Update(ctx, tx); err != nil {
						tx.Rollback()
						return &purchaseModel.Pb, err
					}
					break
				}
			}
		} else {
			// operasi insert
			purchaseDetailModel := model.PurchaseDetail{
				Pb: purchases.PurchaseDetail{
					PurchaseId:     purchaseModel.Pb.GetId(),
					ProductId:      detail.ProductId,
					ProductCode:    mProduct.Pb.GetCode(),
					ProductName:    mProduct.Pb.GetName(),
					Price:          detail.GetPrice(),
					DiscAmount:     detail.GetDiscAmount(),
					DiscPercentage: detail.GetDiscPercentage(),
				},
			}

			if purchaseDetailModel.Pb.GetDiscPercentage() > 0 {
				purchaseDetailModel.Pb.DiscAmount = purchaseDetailModel.Pb.Price * float64(purchaseDetailModel.Pb.DiscPercentage) / 100
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

	purchaseModel.Pb.TotalPrice = sumPrice
	if purchaseModel.Pb.AdditionalDiscPercentage > 0 {
		purchaseModel.Pb.AdditionalDiscAmount = sumPrice * float64(purchaseModel.Pb.AdditionalDiscPercentage) / 100
	}

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
		err = rows.Scan(&pbPurchase.Id, &companyID, &pbPurchase.BranchId, &pbPurchase.BranchName,
			&pbPurchase.Code, &pbPurchase.PurchaseDate, &pbPurchase.Remark,
			&pbPurchase.TotalPrice, &pbPurchase.AdditionalDiscAmount, &pbPurchase.AdditionalDiscPercentage,
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

func (u *Purchase) createValidation(ctx context.Context, in *purchases.Purchase) ([]*inventories.ListProductResponse, error) {
	// basic validation
	{
		if len(in.GetBranchId()) == 0 {
			return []*inventories.ListProductResponse{}, status.Error(codes.InvalidArgument, "Please supply valid branch")
		}

		if len(in.GetSupplier().Id) == 0 {
			return []*inventories.ListProductResponse{}, status.Error(codes.InvalidArgument, "Please supply valid supplier")
		}

		if _, err := time.Parse("2006-01-02T15:04:05.000Z", in.GetPurchaseDate()); err != nil {
			return []*inventories.ListProductResponse{}, status.Error(codes.InvalidArgument, "Please supply valid date")
		}
	}

	var productIds []string
	for _, detail := range in.GetDetails() {
		if len(detail.GetProductId()) == 0 {
			return []*inventories.ListProductResponse{}, status.Error(codes.InvalidArgument, "Please supply valid product")
		}

		productIds = append(productIds, detail.GetProductId())
	}

	mProduct := model.Product{
		Client: u.ProductClient,
		Pb:     &inventories.Product{},
	}

	inProductList := inventories.ListProductRequest{
		Ids: productIds,
	}
	products, err := mProduct.List(ctx, &inProductList)
	if err != nil {
		return []*inventories.ListProductResponse{}, err
	}

	if len(products) != len(productIds) {
		return []*inventories.ListProductResponse{}, status.Error(codes.InvalidArgument, "Please supply valid product")
	}

	return products, nil
}
