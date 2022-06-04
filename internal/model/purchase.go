package model

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"purchase/internal/pkg/app"
	"purchase/pb/purchases"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Purchase struct {
	Pb purchases.Purchase
}

func (u *Purchase) Get(ctx context.Context, db *sql.DB) error {
	query := `
		SELECT purchases.id, purchases.company_id, purchases.branch_id, purchases.branch_name, purchases.supplier_id, purchases.code, 
		purchases.purchase_date, purchases.remark, purchases.created_at, purchases.created_by, purchases.updated_at, purchases.updated_by,
		json_agg(DISTINCT jsonb_build_object(
			'id', purchase_details.id,
			'purchase_id', purchase_details.purchase_id,
			'product_id', purchase_details.product_id,
			'product_name', purchase_details.product_name,
			'product_code', purchase_details.product_code
		)) as details
		FROM purchases 
		JOIN purchase_details ON purchases.id = purchase_details.purchase_id
		WHERE purchases.id = $1
	`

	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		return status.Errorf(codes.Internal, "Prepare statement Get purchase: %v", err)
	}
	defer stmt.Close()

	var datePurchase, createdAt, updatedAt time.Time
	var companyID, details string
	err = stmt.QueryRowContext(ctx, u.Pb.GetId()).Scan(
		&u.Pb.Id, &companyID, &u.Pb.BranchId, &u.Pb.BranchName, &u.Pb.GetSupplier().Id, &u.Pb.Code, &datePurchase, &u.Pb.Remark,
		&createdAt, &u.Pb.CreatedBy, &updatedAt, &u.Pb.UpdatedBy, &details,
	)

	if err == sql.ErrNoRows {
		return status.Errorf(codes.NotFound, "Query Raw get by code purchase: %v", err)
	}

	if err != nil {
		return status.Errorf(codes.Internal, "Query Raw get by code purchase: %v", err)
	}

	if companyID != ctx.Value(app.Ctx("companyID")).(string) {
		return status.Error(codes.Unauthenticated, "its not your company")
	}

	u.Pb.PurchaseDate = datePurchase.String()
	u.Pb.CreatedAt = createdAt.String()
	u.Pb.UpdatedAt = updatedAt.String()

	detailPurchases := []struct {
		ID          string
		PurchaseID  string
		ProductID   string
		ProductName string
		ProductCode string
	}{}
	err = json.Unmarshal([]byte(details), &detailPurchases)
	if err != nil {
		return status.Errorf(codes.Internal, "unmarshal access: %v", err)
	}

	for _, detail := range detailPurchases {
		u.Pb.Details = append(u.Pb.Details, &purchases.PurchaseDetail{
			Id:          detail.ID,
			ProductId:   detail.ProductID,
			ProductCode: detail.ProductCode,
			ProductName: detail.ProductName,
			PurchaseId:  detail.PurchaseID,
		})
	}

	return nil
}

func (u *Purchase) GetByCode(ctx context.Context, db *sql.DB) error {
	query := `
		SELECT id, branch_id, branch_name, supplier_id, code, purchase_date, remark, created_at, created_by, updated_at, updated_by 
		FROM purchases WHERE purchases.code = $1 AND purchases.company_id = $2
	`

	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		return status.Errorf(codes.Internal, "Prepare statement Get by code purchase: %v", err)
	}
	defer stmt.Close()

	var datePurchase, createdAt, updatedAt time.Time
	err = stmt.QueryRowContext(ctx, u.Pb.GetCode(), ctx.Value(app.Ctx("companyID")).(string)).Scan(
		&u.Pb.Id, &u.Pb.BranchId, &u.Pb.BranchName, &u.Pb.GetSupplier().Id, &u.Pb.Code, &datePurchase, &u.Pb.Remark,
		&createdAt, &u.Pb.CreatedBy, &updatedAt, &u.Pb.UpdatedBy,
	)

	if err == sql.ErrNoRows {
		return status.Errorf(codes.NotFound, "Query Raw get by code purchase: %v", err)
	}

	if err != nil {
		return status.Errorf(codes.Internal, "Query Raw get by code purchase: %v", err)
	}

	u.Pb.PurchaseDate = datePurchase.String()
	u.Pb.CreatedAt = createdAt.String()
	u.Pb.UpdatedAt = updatedAt.String()

	return nil
}

func (u *Purchase) getCode(ctx context.Context, tx *sql.Tx) (string, error) {
	var count int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM purchases 
			WHERE company_id = $1 AND to_char(created_at, 'YYYY-mm') = to_char(now(), 'YYYY-mm')`,
		ctx.Value(app.Ctx("companyID")).(string)).Scan(&count)

	if err != nil {
		return "", status.Error(codes.Internal, err.Error())
	}

	return fmt.Sprintf("DO%d%d%d",
		time.Now().UTC().Year(),
		int(time.Now().UTC().Month()),
		(count + 1)), nil
}

func (u *Purchase) Create(ctx context.Context, tx *sql.Tx) error {
	u.Pb.Id = uuid.New().String()
	now := time.Now().UTC()
	u.Pb.CreatedBy = ctx.Value(app.Ctx("userID")).(string)
	u.Pb.UpdatedBy = ctx.Value(app.Ctx("userID")).(string)
	datePurchase, err := time.Parse("2006-01-02T15:04:05.000Z", u.Pb.GetPurchaseDate())
	if err != nil {
		return status.Errorf(codes.Internal, "convert Date: %v", err)
	}

	u.Pb.Code, err = u.getCode(ctx, tx)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO purchases (id, company_id, branch_id, branch_name, supplier_id, code, purchase_date, remark, price, additional_disc_amount, additional_disc_prosentation, created_at, created_by, updated_at, updated_by) 
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return status.Errorf(codes.Internal, "Prepare insert purchase: %v", err)
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx,
		u.Pb.GetId(),
		ctx.Value(app.Ctx("companyID")).(string),
		u.Pb.GetBranchId(),
		u.Pb.GetBranchName(),
		u.Pb.GetSupplier().GetId(),
		u.Pb.GetCode(),
		datePurchase,
		u.Pb.GetRemark(),
		u.Pb.GetPrice(),
		u.Pb.GetAdditionalDiscAmount(),
		u.Pb.GetAdditionalDiscProsentation(),
		now,
		u.Pb.GetCreatedBy(),
		now,
		u.Pb.GetUpdatedBy(),
	)
	if err != nil {
		return status.Errorf(codes.Internal, "Exec insert purchase: %v", err)
	}

	u.Pb.CreatedAt = now.String()
	u.Pb.UpdatedAt = u.Pb.CreatedAt

	for _, detail := range u.Pb.GetDetails() {
		purchaseDetailModel := PurchaseDetail{}
		purchaseDetailModel.Pb = purchases.PurchaseDetail{
			PurchaseId:       u.Pb.GetId(),
			ProductId:        detail.GetProductId(),
			ProductCode:      detail.GetProductCode(),
			ProductName:      detail.GetProductName(),
			Price:            detail.GetPrice(),
			DiscAmount:       detail.GetDiscAmount(),
			DiscProsentation: detail.GetDiscProsentation(),
		}
		purchaseDetailModel.PbPurchase = purchases.Purchase{
			Id:                         u.Pb.Id,
			BranchId:                   u.Pb.BranchId,
			BranchName:                 u.Pb.BranchName,
			Supplier:                   u.Pb.GetSupplier(),
			Code:                       u.Pb.Code,
			PurchaseDate:               u.Pb.PurchaseDate,
			Remark:                     u.Pb.Remark,
			Price:                      u.Pb.Price,
			AdditionalDiscAmount:       u.Pb.AdditionalDiscAmount,
			AdditionalDiscProsentation: u.Pb.AdditionalDiscProsentation,
			CreatedAt:                  u.Pb.CreatedAt,
			CreatedBy:                  u.Pb.CreatedBy,
			UpdatedAt:                  u.Pb.UpdatedAt,
			UpdatedBy:                  u.Pb.UpdatedBy,
		}
		err = purchaseDetailModel.Create(ctx, tx)
		if err != nil {
			return err
		}
	}

	return nil
}

func (u *Purchase) Update(ctx context.Context, tx *sql.Tx) error {
	now := time.Now().UTC()
	u.Pb.UpdatedBy = ctx.Value(app.Ctx("userID")).(string)
	datePurchase, err := time.Parse("2006-01-02T15:04:05.000Z", u.Pb.GetPurchaseDate())
	if err != nil {
		return status.Errorf(codes.Internal, "convert purchase date: %v", err)
	}

	query := `
		UPDATE purchases SET
		supplier_id = $1,
		purchase_date = $2,
		remark = $3, 
		updated_at = $4, 
		updated_by= $5
		WHERE id = $6
	`
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return status.Errorf(codes.Internal, "Prepare update purchase: %v", err)
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx,
		u.Pb.GetSupplier().GetId(),
		datePurchase,
		u.Pb.GetRemark(),
		now,
		u.Pb.GetUpdatedBy(),
		u.Pb.GetId(),
	)
	if err != nil {
		return status.Errorf(codes.Internal, "Exec update purchase: %v", err)
	}

	u.Pb.UpdatedAt = now.String()

	return nil
}

func (u *Purchase) Delete(ctx context.Context, db *sql.DB) error {
	stmt, err := db.PrepareContext(ctx, `DELETE FROM purchases WHERE id = $1`)
	if err != nil {
		return status.Errorf(codes.Internal, "Prepare delete purchase: %v", err)
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, u.Pb.GetId())
	if err != nil {
		return status.Errorf(codes.Internal, "Exec delete purchase: %v", err)
	}

	return nil
}

func (u *Purchase) ListQuery(ctx context.Context, db *sql.DB, in *purchases.ListPurchaseRequest) (string, []interface{}, *purchases.PurchasePaginationResponse, error) {
	var paginationResponse purchases.PurchasePaginationResponse
	query := `SELECT id, company_id, branch_id, branch_name, supplier_id, code, purchase_date, remark, created_at, created_by, updated_at, updated_by FROM purchases`

	where := []string{"company_id = $1"}
	paramQueries := []interface{}{ctx.Value(app.Ctx("companyID")).(string)}

	if len(in.GetBranchId()) > 0 {
		paramQueries = append(paramQueries, in.GetBranchId())
		where = append(where, fmt.Sprintf(`branch_id = $%d`, len(paramQueries)))
	}

	if len(in.GetSupplierId()) > 0 {
		paramQueries = append(paramQueries, in.GetSupplierId())
		where = append(where, fmt.Sprintf(`sales_order_id = $%d`, len(paramQueries)))
	}

	if len(in.GetPagination().GetSearch()) > 0 {
		paramQueries = append(paramQueries, in.GetPagination().GetSearch())
		where = append(where, fmt.Sprintf(`(code ILIKE $%d OR remark ILIKE $%d)`, len(paramQueries), len(paramQueries)))
	}

	{
		qCount := `SELECT COUNT(*) FROM purchases`
		if len(where) > 0 {
			qCount += " WHERE " + strings.Join(where, " AND ")
		}
		var count int
		err := db.QueryRowContext(ctx, qCount, paramQueries...).Scan(&count)
		if err != nil && err != sql.ErrNoRows {
			return query, paramQueries, &paginationResponse, status.Error(codes.Internal, err.Error())
		}

		paginationResponse.Count = uint32(count)
	}

	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, " AND ")
	}

	if len(in.GetPagination().GetOrderBy()) == 0 || !(in.GetPagination().GetOrderBy() == "code") {
		if in.GetPagination() == nil {
			in.Pagination = &purchases.Pagination{OrderBy: "created_at"}
		} else {
			in.GetPagination().OrderBy = "created_at"
		}
	}

	query += ` ORDER BY ` + in.GetPagination().GetOrderBy() + ` ` + in.GetPagination().GetSort().String()

	if in.GetPagination().GetLimit() > 0 {
		query += fmt.Sprintf(` LIMIT $%d OFFSET $%d`, (len(paramQueries) + 1), (len(paramQueries) + 2))
		paramQueries = append(paramQueries, in.GetPagination().GetLimit(), in.GetPagination().GetOffset())
	}

	return query, paramQueries, &paginationResponse, nil
}
