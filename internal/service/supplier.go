package service

import (
	"context"
	"database/sql"
	"time"

	"github.com/jacky-htg/erp-pkg/app"
	"github.com/jacky-htg/erp-proto/go/pb/purchases"
	"github.com/jacky-htg/purchase-service/internal/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Supplier struct {
	Db *sql.DB
	purchases.UnimplementedSupplierServiceServer
}

func (u *Supplier) SupplierCreate(ctx context.Context, in *purchases.Supplier) (*purchases.Supplier, error) {
	var supplierModel model.Supplier
	var err error

	if len(in.GetName()) == 0 {
		return &supplierModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid name")
	}

	if len(in.GetAddress()) == 0 {
		return &supplierModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid address")
	}

	if len(in.GetPhone()) == 0 {
		return &supplierModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid phone")
	}

	// code validation
	{
		if len(in.GetCode()) == 0 {
			return &supplierModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid code")
		}

		supplierModel = model.Supplier{}
		supplierModel.Pb.Code = in.GetCode()
		err = supplierModel.GetByCode(ctx, u.Db)
		if err != nil {
			if st, ok := status.FromError(err); ok && st.Code() != codes.NotFound {
				return &supplierModel.Pb, err
			}
		}

		if len(supplierModel.Pb.GetId()) > 0 {
			return &supplierModel.Pb, status.Error(codes.AlreadyExists, "code must be unique")
		}
	}

	supplierModel.Pb = purchases.Supplier{
		Code:    in.GetCode(),
		Name:    in.GetName(),
		Address: in.GetAddress(),
		Phone:   in.GetPhone(),
	}
	err = supplierModel.Create(ctx, u.Db)
	if err != nil {
		return &supplierModel.Pb, err
	}

	return &supplierModel.Pb, nil
}

func (u *Supplier) SupplierUpdate(ctx context.Context, in *purchases.Supplier) (*purchases.Supplier, error) {
	var supplierModel model.Supplier
	var err error

	if len(in.GetId()) == 0 {
		return &supplierModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid id")
	}
	supplierModel.Pb.Id = in.GetId()

	err = supplierModel.Get(ctx, u.Db)
	if err != nil {
		return &supplierModel.Pb, err
	}

	if len(in.GetName()) > 0 {
		supplierModel.Pb.Name = in.GetName()
	}

	if len(in.GetAddress()) > 0 {
		supplierModel.Pb.Address = in.GetAddress()
	}

	if len(in.GetPhone()) > 0 {
		supplierModel.Pb.Phone = in.GetPhone()
	}

	err = supplierModel.Update(ctx, u.Db)
	if err != nil {
		return &supplierModel.Pb, err
	}

	return &supplierModel.Pb, nil
}

func (u *Supplier) SupplierView(ctx context.Context, in *purchases.Id) (*purchases.Supplier, error) {
	var supplierModel model.Supplier
	var err error

	if len(in.GetId()) == 0 {
		return &supplierModel.Pb, status.Error(codes.InvalidArgument, "Please supply valid id")
	}
	supplierModel.Pb.Id = in.GetId()

	err = supplierModel.Get(ctx, u.Db)
	if err != nil {
		return &supplierModel.Pb, err
	}

	return &supplierModel.Pb, nil
}

func (u *Supplier) SupplierDelete(ctx context.Context, in *purchases.Id) (*purchases.MyBoolean, error) {
	var output purchases.MyBoolean
	output.Boolean = false

	var supplierModel model.Supplier
	var err error

	if len(in.GetId()) == 0 {
		return &output, status.Error(codes.InvalidArgument, "Please supply valid id")
	}
	supplierModel.Pb.Id = in.GetId()

	err = supplierModel.Get(ctx, u.Db)
	if err != nil {
		return &output, err
	}

	err = supplierModel.Delete(ctx, u.Db)
	if err != nil {
		return &output, err
	}

	output.Boolean = true
	return &output, nil
}

func (u *Supplier) SupplierList(in *purchases.ListSupplierRequest, stream purchases.SupplierService_SupplierListServer) error {
	ctx := stream.Context()
	var supplierModel model.Supplier
	query, paramQueries, paginationResponse, err := supplierModel.ListQuery(ctx, u.Db, in.Pagination)
	if err != nil {
		return err
	}

	rows, err := u.Db.QueryContext(ctx, query, paramQueries...)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	defer rows.Close()
	paginationResponse.Pagination = in.Pagination

	for rows.Next() {
		err := app.ContextError(ctx)
		if err != nil {
			return err
		}

		var pbSupplier purchases.Supplier
		var companyID string
		var createdAt, updatedAt time.Time
		err = rows.Scan(&pbSupplier.Id, &companyID, &pbSupplier.Code, &pbSupplier.Name, &pbSupplier.Address, &pbSupplier.Phone, &createdAt, &pbSupplier.CreatedBy, &updatedAt, &pbSupplier.UpdatedBy)
		if err != nil {
			return status.Errorf(codes.Internal, "scan data: %v", err)
		}

		pbSupplier.CreatedAt = createdAt.String()
		pbSupplier.UpdatedAt = updatedAt.String()

		res := &purchases.ListSupplierResponse{
			Pagination: paginationResponse,
			Supplier:   &pbSupplier,
		}

		err = stream.Send(res)
		if err != nil {
			return status.Errorf(codes.Unknown, "cannot send stream response: %v", err)
		}
	}
	return nil
}
