package service

import (
	"context"
	"database/sql"
	"time"

	"github.com/jacky-htg/erp-pkg/app"
	"github.com/jacky-htg/erp-proto/go/pb/inventories"
	"github.com/jacky-htg/erp-proto/go/pb/purchases"
	"github.com/jacky-htg/erp-proto/go/pb/users"
	"github.com/jacky-htg/purchase-service/internal/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Purchase struct {
	Db            *sql.DB
	UserClient    users.UserServiceClient
	RegionClient  users.RegionServiceClient
	BranchClient  users.BranchServiceClient
	ProductClient inventories.ProductServiceClient
	ReceiveClient inventories.ReceiveServiceClient
	purchases.UnimplementedPurchaseServiceServer
}

func (u *Purchase) PurchaseCreate(ctx context.Context, in *purchases.Purchase) (*purchases.Purchase, error) {
	var purchaseModel model.Purchase
	var err error

	// TODO : if this month any closing account, create transaction for this month will be blocked

	products, err := u.createValidation(ctx, in)
	if err != nil {
		return &purchaseModel.Pb, err
	}

	var sumPrice float64
	for _, detail := range in.GetDetails() {
		for _, p := range products {
			if detail.GetProductId() == p.Product.GetId() {
				detail.ProductCode = p.Product.GetCode()
				detail.ProductName = p.Product.GetName()
			}
		}

		if detail.DiscPercentage > 0 {
			detail.DiscAmount = detail.GetPrice() * float64(detail.Quantity) * float64(detail.DiscPercentage) / 100
		}
		detail.TotalPrice = (detail.GetPrice() * float64(detail.Quantity)) - detail.DiscAmount
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
		Price:                    sumPrice,
		AdditionalDiscAmount:     in.GetAdditionalDiscAmount(),
		AdditionalDiscPercentage: in.GetAdditionalDiscPercentage(),
		TotalPrice:               (sumPrice - in.GetAdditionalDiscAmount()),
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

func (u *Purchase) PurchaseUpdate(ctx context.Context, in *purchases.Purchase) (*purchases.Purchase, error) {
	var purchaseModel model.Purchase
	var err error

	// TODO : if this month any closing account, create transaction for thus month will be blocked

	if len(in.GetId()) == 0 {
		return &purchaseModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid id")
	}
	purchaseModel.Pb.Id = in.GetId()

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
	mReceive := model.Receive{Client: u.ReceiveClient}
	if hasReceive, err := mReceive.HasTransactionByPurchase(ctx, in.GetId()); err != nil {
		return &purchaseModel.Pb, err
	} else if hasReceive {
		return &purchaseModel.Pb, status.Error(codes.PermissionDenied, "Can not updated because the purchase has receiving transaction")
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

	//var newDetails []*purchases.PurchaseDetail
	var productIds []string
	for _, detail := range in.GetDetails() {
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
	if err != nil {
		return &purchaseModel.Pb, err
	}

	if len(products) != len(productIds) {
		return &purchaseModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid product")
	}

	var sumPrice float64
	for _, detail := range in.GetDetails() {
		for _, p := range products {
			if detail.GetProductId() == p.Product.GetId() {
				detail.ProductCode = p.Product.GetCode()
				detail.ProductName = p.Product.GetName()
			}
		}

		if detail.DiscPercentage > 0 {
			detail.DiscAmount = detail.GetPrice() * float64(detail.Quantity) * float64(detail.DiscPercentage) / 100
		}
		detail.TotalPrice = (detail.GetPrice() * float64(detail.Quantity)) - detail.DiscAmount
		sumPrice += detail.TotalPrice

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
						data.DiscAmount = detail.DiscAmount
						data.TotalPrice = detail.TotalPrice
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
					Quantity:       detail.GetQuantity(),
					DiscAmount:     detail.GetDiscAmount(),
					DiscPercentage: detail.GetDiscPercentage(),
					TotalPrice:     detail.GetTotalPrice(),
				},
			}
			err = purchaseDetailModel.Create(ctx, tx)
			if err != nil {
				tx.Rollback()
				return &purchaseModel.Pb, err
			}

			//newDetails = append(newDetails, &purchaseDetailModel.Pb)
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
	if purchaseModel.Pb.AdditionalDiscPercentage > 0 {
		purchaseModel.Pb.AdditionalDiscAmount = sumPrice * float64(purchaseModel.Pb.AdditionalDiscPercentage) / 100
	}
	purchaseModel.Pb.TotalPrice = sumPrice - purchaseModel.Pb.AdditionalDiscAmount

	err = purchaseModel.Update(ctx, tx)
	if err != nil {
		tx.Rollback()
		return &purchaseModel.Pb, err
	}

	tx.Commit()

	return &purchaseModel.Pb, nil
}

func (u *Purchase) PurchaseView(ctx context.Context, in *purchases.Id) (*purchases.Purchase, error) {
	var purchaseModel model.Purchase
	var err error

	if len(in.GetId()) == 0 {
		return &purchaseModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid id")
	}
	purchaseModel.Pb.Id = in.GetId()

	err = purchaseModel.Get(ctx, u.Db)
	if err != nil {
		return &purchaseModel.Pb, err
	}

	mBranch := model.Branch{
		UserClient:   u.UserClient,
		RegionClient: u.RegionClient,
		BranchClient: u.BranchClient,
		Id:           purchaseModel.Pb.BranchId,
	}
	err = mBranch.IsYourBranch(ctx)
	if err != nil {
		return &purchaseModel.Pb, err
	}

	return &purchaseModel.Pb, nil
}

func (u *Purchase) PurchaseList(in *purchases.ListPurchaseRequest, stream purchases.PurchaseService_PurchaseListServer) error {
	ctx := stream.Context()
	var purchaseModel model.Purchase
	query, paramQueries, paginationResponse, err := purchaseModel.ListQuery(ctx, u.Db, in)
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

		var pbPurchase purchases.Purchase
		var pbSupplier purchases.Supplier
		var companyID string
		var createdAt, updatedAt time.Time
		err = rows.Scan(&pbPurchase.Id, &companyID, &pbPurchase.BranchId, &pbPurchase.BranchName,
			&pbSupplier.Id, &pbSupplier.Name,
			&pbPurchase.Code, &pbPurchase.PurchaseDate, &pbPurchase.Remark,
			&pbPurchase.Price, &pbPurchase.AdditionalDiscAmount, &pbPurchase.AdditionalDiscPercentage, &pbPurchase.TotalPrice,
			&createdAt, &pbPurchase.CreatedBy, &updatedAt, &pbPurchase.UpdatedBy)
		if err != nil {
			return status.Errorf(codes.Internal, "scan data: %v", err)
		}

		pbPurchase.CreatedAt = createdAt.String()
		pbPurchase.UpdatedAt = updatedAt.String()
		pbPurchase.Supplier = &pbSupplier

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

func (u *Purchase) GetOutstandingPurchaseDetails(ctx context.Context, in *purchases.OutstandingPurchaseRequest) (*purchases.OutstandingPurchaseDetails, error) {
	var output purchases.OutstandingPurchaseDetails
	{
		mPurchase := model.Purchase{
			Pb: purchases.Purchase{Id: in.Id},
		}
		var returnId *string
		if len(in.ReturnId) > 0 {
			returnId = &in.ReturnId
		}
		details, err := mPurchase.OutstandingDetail(ctx, u.Db, returnId)
		if err != nil {
			return &output, err
		}

		output.Detail = append(output.Detail, details...)
	}

	{
		mProduct := model.Product{
			Client: u.ProductClient,
			Pb:     &inventories.Product{},
		}

		var productIds []string
		for _, v := range output.Detail {
			productIds = append(productIds, v.ProductId)
		}

		inProductList := inventories.ListProductRequest{
			Ids: productIds,
		}
		products, err := mProduct.List(ctx, &inProductList)
		if err != nil {
			return &output, err
		}

		for i, d := range output.Detail {
			for _, v := range products {
				if v.Product.Id == d.ProductId {
					output.Detail[i].ProductName = v.Product.Name
				}
			}
		}
	}
	return &output, nil
}

func (u *Purchase) createValidation(ctx context.Context, in *purchases.Purchase) ([]*inventories.ListProductResponse, error) {
	if len(in.GetBranchId()) == 0 {
		return []*inventories.ListProductResponse{}, status.Error(codes.InvalidArgument, "Please supply valid branch")
	}

	if in.GetSupplier() == nil || len(in.GetSupplier().Id) == 0 {
		return []*inventories.ListProductResponse{}, status.Error(codes.InvalidArgument, "Please supply valid supplier")
	}

	if _, err := time.Parse("2006-01-02T15:04:05.000Z", in.GetPurchaseDate()); err != nil {
		return []*inventories.ListProductResponse{}, status.Error(codes.InvalidArgument, "Please supply valid date")
	}

	// validate bulk product by call product grpc
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
