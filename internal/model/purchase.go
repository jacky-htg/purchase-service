package model

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"purchase/internal/pkg/app"
	"purchase/internal/pkg/util"
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
		SELECT purchases.id, purchases.company_id, purchases.branch_id, purchases.branch_name, suppliers.id, suppliers.name, purchases.code, 
		purchases.purchase_date, purchases.remark, purchases.price, purchases.additional_disc_amount, purchases.additional_disc_percentage, purchases.total_price,
		purchases.created_at, purchases.created_by, purchases.updated_at, purchases.updated_by,
		json_agg(DISTINCT jsonb_build_object(
			'id', purchase_details.id,
			'purchase_id', purchase_details.purchase_id,
			'product_id', purchase_details.product_id,
			'price', purchase_details.price,
			'disc_amount', purchase_details.disc_amount,
			'disc_percentage', purchase_details.disc_percentage,
			'quantity', purchase_details.quantity,
			'total_price', purchase_details.total_price
		)) as details
		FROM purchases JOIN suppliers ON purchases.supplier_id = suppliers.id
		JOIN purchase_details ON purchases.id = purchase_details.purchase_id
		WHERE purchases.id = $1
		GROUP BY purchases.id, suppliers.id
	`

	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		return status.Errorf(codes.Internal, "Prepare statement Get purchase: %v", err)
	}
	defer stmt.Close()

	var datePurchase, createdAt, updatedAt time.Time
	var companyID, details string
	var pbSupplier purchases.Supplier
	err = stmt.QueryRowContext(ctx, u.Pb.GetId()).Scan(
		&u.Pb.Id, &companyID, &u.Pb.BranchId, &u.Pb.BranchName, &pbSupplier.Id, &pbSupplier.Name,
		&u.Pb.Code, &datePurchase, &u.Pb.Remark,
		&u.Pb.Price, &u.Pb.AdditionalDiscAmount, &u.Pb.AdditionalDiscPercentage, &u.Pb.TotalPrice,
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
	u.Pb.Supplier = &pbSupplier
	u.Pb.CreatedAt = createdAt.String()
	u.Pb.UpdatedAt = updatedAt.String()

	detailPurchases := []struct {
		ID             string `json:"id"`
		PurchaseID     string
		ProductID      string `json:"product_id"`
		Price          float64
		DiscAmount     float64 `json:"disc_amount"`
		DiscPercentage float32 `json:"disc_percentage"`
		Quantity       int     `json:"quantity"`
		TotalPrice     float64 `json:"total_price"`
	}{}
	err = json.Unmarshal([]byte(details), &detailPurchases)
	if err != nil {
		return status.Errorf(codes.Internal, "unmarshal access: %v", err)
	}

	for _, detail := range detailPurchases {
		u.Pb.Details = append(u.Pb.Details, &purchases.PurchaseDetail{
			Id:             detail.ID,
			ProductId:      detail.ProductID,
			PurchaseId:     detail.PurchaseID,
			Price:          detail.Price,
			Quantity:       int32(detail.Quantity),
			DiscAmount:     detail.DiscAmount,
			DiscPercentage: detail.DiscPercentage,
			TotalPrice:     detail.TotalPrice,
		})
	}

	return nil
}

func (u *Purchase) GetByCode(ctx context.Context, db *sql.DB) error {
	query := `
		SELECT id, branch_id, branch_name, supplier_id, code, purchase_date, remark, 
			price, additional_disc_amount, additional_disc_percentage, total_price, created_at, created_by, updated_at, updated_by 
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
		&u.Pb.Price, &u.Pb.AdditionalDiscAmount, &u.Pb.AdditionalDiscPercentage, &u.Pb.TotalPrice,
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

func (u *Purchase) Create(ctx context.Context, tx *sql.Tx) error {
	u.Pb.Id = uuid.New().String()
	now := time.Now().UTC()
	u.Pb.CreatedBy = ctx.Value(app.Ctx("userID")).(string)
	u.Pb.UpdatedBy = ctx.Value(app.Ctx("userID")).(string)
	datePurchase, err := time.Parse("2006-01-02T15:04:05.000Z", u.Pb.GetPurchaseDate())
	if err != nil {
		return status.Errorf(codes.Internal, "convert Date: %v", err)
	}

	u.Pb.Code, err = util.GetCode(ctx, tx, "purchases", "PC")
	if err != nil {
		return err
	}

	query := `
		INSERT INTO purchases (id, company_id, branch_id, branch_name, supplier_id, code, purchase_date, remark, price, additional_disc_amount, additional_disc_percentage, total_price, created_at, created_by, updated_at, updated_by) 
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
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
		u.Pb.GetAdditionalDiscPercentage(),
		u.Pb.GetTotalPrice(),
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
			PurchaseId:     u.Pb.GetId(),
			ProductId:      detail.GetProductId(),
			Price:          detail.GetPrice(),
			DiscAmount:     detail.GetDiscAmount(),
			DiscPercentage: detail.GetDiscPercentage(),
			Quantity:       detail.GetQuantity(),
			TotalPrice:     detail.GetTotalPrice(),
		}
		purchaseDetailModel.PbPurchase = purchases.Purchase{
			Id:                       u.Pb.Id,
			BranchId:                 u.Pb.BranchId,
			BranchName:               u.Pb.BranchName,
			Supplier:                 u.Pb.GetSupplier(),
			Code:                     u.Pb.Code,
			PurchaseDate:             u.Pb.PurchaseDate,
			Remark:                   u.Pb.Remark,
			Price:                    u.Pb.Price,
			AdditionalDiscAmount:     u.Pb.AdditionalDiscAmount,
			AdditionalDiscPercentage: u.Pb.AdditionalDiscPercentage,
			TotalPrice:               u.Pb.TotalPrice,
			CreatedAt:                u.Pb.CreatedAt,
			CreatedBy:                u.Pb.CreatedBy,
			UpdatedAt:                u.Pb.UpdatedAt,
			UpdatedBy:                u.Pb.UpdatedBy,
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
		price = $4,
		additional_disc_amount = $5,
		additional_disc_percentage = $6,
		total_price = $7,
		updated_at = $8, 
		updated_by= $9
		WHERE id = $10
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
		u.Pb.GetPrice(),
		u.Pb.GetAdditionalDiscAmount(),
		u.Pb.GetAdditionalDiscPercentage(),
		u.Pb.GetTotalPrice(),
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

func (u *Purchase) ListQuery(ctx context.Context, db *sql.DB, in *purchases.ListPurchaseRequest) (string, []interface{}, *purchases.PurchasePaginationResponse, error) {
	var paginationResponse purchases.PurchasePaginationResponse
	query := `
		SELECT purchases.id, purchases.company_id, purchases.branch_id, purchases.branch_name, 
			purchases.supplier_id, suppliers.name supplier_name, purchases.code, purchases.purchase_date, 
			purchases.remark, purchases.price, purchases.additional_disc_amount, 
			purchases.additional_disc_percentage, purchases.total_price, 
			purchases.created_at, purchases.created_by, purchases.updated_at, purchases.updated_by 
		FROM purchases JOIN suppliers on purchases.supplier_id = suppliers.id
	`

	where := []string{"purchases.company_id = $1"}
	paramQueries := []interface{}{ctx.Value(app.Ctx("companyID")).(string)}

	if len(in.GetBranchId()) > 0 {
		paramQueries = append(paramQueries, in.GetBranchId())
		where = append(where, fmt.Sprintf(`purchases.branch_id = $%d`, len(paramQueries)))
	}

	if len(in.GetSupplierId()) > 0 {
		paramQueries = append(paramQueries, in.GetSupplierId())
		where = append(where, fmt.Sprintf(`purchases.supplier_id = $%d`, len(paramQueries)))
	}

	if len(in.GetPagination().GetSearch()) > 0 {
		paramQueries = append(paramQueries, "%"+in.GetPagination().GetSearch()+"%")
		where = append(where, fmt.Sprintf(`(purchases.code ILIKE $%d OR purchases.remark ILIKE $%d)`, len(paramQueries), len(paramQueries)))
	}

	{
		qCount := `SELECT COUNT(*) FROM purchases JOIN suppliers ON purchases.supplier_id = suppliers.id`
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

	if len(in.GetPagination().GetOrderBy()) == 0 || !(in.GetPagination().GetOrderBy() == "purchases.code") {
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

func (u *Purchase) OutstandingDetail(ctx context.Context, db *sql.DB, purchaseReturnId *string) ([]*purchases.PurchaseDetail, error) {
	var list []*purchases.PurchaseDetail

	queryReturn := `
		SELECT purchase_return_details.product_id, SUM(purchase_return_details.quantity) return_quantity  
		FROM purchase_returns
		JOIN purchase_return_details ON purchase_returns.id = purchase_return_details.purchase_return_id
		WHERE purchase_returns.purchase_id = $1 
	`
	if purchaseReturnId != nil {
		queryReturn += ` AND purchase_returns.id != $3`
	}

	queryReturn += ` GROUP BY purchase_return_details.product_id`

	query := `
		SELECT purchase_details.product_id, (purchase_details.quantity - coalesce(purchase_returns.return_quantity, 0)) quantity,
			purchase_details.price, purchase_details.disc_percentage
		FROM purchase_details 
		JOIN purchases ON purchase_details.purchase_id = purchases.id
		LEFT JOIN (
			` + queryReturn + `
		) AS purchase_returns ON purchase_details.product_id = purchase_returns.product_id
		WHERE purchase_details.purchase_id = $1 
			AND (purchase_details.quantity - coalesce(purchase_returns.return_quantity, 0)) > 0		
			AND purchases.company_id = $2
	`

	params := []interface{}{
		u.Pb.Id,
		ctx.Value(app.Ctx("companyID")).(string),
	}

	if purchaseReturnId != nil {
		params = append(params, *purchaseReturnId)
	}

	rows, err := db.QueryContext(ctx, query, params...)
	if err != nil {
		return list, status.Error(codes.Internal, err.Error())
	}
	defer rows.Close()

	for rows.Next() {
		var pbPurchaseDetail purchases.PurchaseDetail
		err = rows.Scan(
			&pbPurchaseDetail.ProductId,
			&pbPurchaseDetail.Quantity,
			&pbPurchaseDetail.Price,
			&pbPurchaseDetail.DiscPercentage,
		)
		if err != nil {
			return list, status.Errorf(codes.Internal, "scan data: %v", err)
		}

		list = append(list, &pbPurchaseDetail)
	}

	if rows.Err() != nil {
		return list, status.Errorf(codes.Internal, "rows error: %v", err)
	}

	return list, nil
}

func (u *Purchase) GetReturnAdditionalDisc(ctx context.Context, db *sql.DB) (float64, error) {
	var returnAdditionalDisc float64
	query := `
		SELECT SUM(purchase_returns.additional_disc_amount) return_additional_disc
		FROM purchases
		JOIN purchase_returns ON purchases.id = purchase_returns.purchase_id
		WHERE purchases.id = $1
		GROUP BY purchases.id
	`
	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		return returnAdditionalDisc, status.Errorf(codes.Internal, "Prepare statement Get purchase: %v", err)
	}
	defer stmt.Close()

	err = stmt.QueryRowContext(ctx, u.Pb.GetId()).Scan(&returnAdditionalDisc)

	if err == sql.ErrNoRows {
		return returnAdditionalDisc, nil
	}

	if err != nil {
		return returnAdditionalDisc, status.Errorf(codes.Internal, "Query Raw get returnAdditionalDisc: %v", err)
	}

	return returnAdditionalDisc, nil
}
