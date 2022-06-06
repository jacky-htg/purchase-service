package model

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"purchase/internal/pkg/app"
	"purchase/internal/pkg/util"
	"purchase/pb/purchases"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PurchaseReturn struct {
	Pb purchases.PurchaseReturn
}

func (u *PurchaseReturn) Get(ctx context.Context, db *sql.DB) error {
	query := `
		SELECT purchase_returns.id, purchase_returns.company_id, purchase_returns.branch_id, purchase_returns.branch_name, purchase_returns.purchase_id, purchase_returns.code, 
		purchase_returns.return_date, purchase_returns.remark, purchase_returns.created_at, purchase_returns.created_by, purchase_returns.updated_at, purchase_returns.updated_by,
		json_agg(DISTINCT jsonb_build_object(
			'id', purchase_return_details.id,
			'purchase_return_id', purchase_return_details.purchase_return_id,
			'product_id', purchase_return_details.product_id
		)) as details
		FROM purchase_returns 
		JOIN purchase_return_details ON purchase_returns.id = purchase_return_details.purchase_return_id
		WHERE purchase_returns.id = $1
	`

	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		return status.Errorf(codes.Internal, "Prepare statement Get purchase return: %v", err)
	}
	defer stmt.Close()

	var dateReturn, createdAt, updatedAt time.Time
	var companyID, details string
	err = stmt.QueryRowContext(ctx, u.Pb.GetId()).Scan(
		&u.Pb.Id, &companyID, &u.Pb.BranchId, &u.Pb.BranchName, &u.Pb.Purchase.Id, &u.Pb.Code, &dateReturn, &u.Pb.Remark,
		&createdAt, &u.Pb.CreatedBy, &updatedAt, &u.Pb.UpdatedBy, &details,
	)

	if err == sql.ErrNoRows {
		return status.Errorf(codes.NotFound, "Query Raw get by code purchase return: %v", err)
	}

	if err != nil {
		return status.Errorf(codes.Internal, "Query Raw get by code purchase return: %v", err)
	}

	if companyID != ctx.Value(app.Ctx("companyID")).(string) {
		return status.Error(codes.Unauthenticated, "its not your company")
	}

	u.Pb.ReturnDate = dateReturn.String()
	u.Pb.CreatedAt = createdAt.String()
	u.Pb.UpdatedAt = updatedAt.String()

	detailPurchaseReturns := []struct {
		ID               string
		PurchaseReturnID string
		ProductID        string
		ProductName      string
		ProductCode      string
	}{}
	err = json.Unmarshal([]byte(details), &detailPurchaseReturns)
	if err != nil {
		return status.Errorf(codes.Internal, "unmarshal detailPurchaseReturns: %v", err)
	}

	for _, detail := range detailPurchaseReturns {
		u.Pb.Details = append(u.Pb.Details, &purchases.PurchaseReturnDetail{
			Id:               detail.ID,
			ProductId:        detail.ProductID,
			ProductCode:      detail.ProductCode,
			ProductName:      detail.ProductName,
			PurchaseReturnId: detail.PurchaseReturnID,
		})
	}

	return nil
}

func (u *PurchaseReturn) HasReturn(ctx context.Context, db *sql.DB) (bool, error) {
	query := `
		SELECT purchase_returns.id
		FROM purchase_returns 
		WHERE purchase_returns.purchase_id = $1 
		LIMIT 0,1
	`

	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		return false, status.Errorf(codes.Internal, "Prepare statement 'Has Purchase Return': %v", err)
	}
	defer stmt.Close()

	var myId string
	err = stmt.QueryRowContext(ctx, u.Pb.Purchase.GetId()).Scan(&myId)

	if err == sql.ErrNoRows {
		return false, nil
	}

	if err != nil {
		return false, status.Errorf(codes.Internal, "Query Raw get by code purchase return: %v", err)
	}

	return true, nil
}

func (u *PurchaseReturn) Create(ctx context.Context, tx *sql.Tx) error {
	u.Pb.Id = uuid.New().String()
	now := time.Now().UTC()
	u.Pb.CreatedBy = ctx.Value(app.Ctx("userID")).(string)
	u.Pb.UpdatedBy = ctx.Value(app.Ctx("userID")).(string)
	dateReturn, err := time.Parse("2006-01-02T15:04:05.000Z", u.Pb.GetReturnDate())
	if err != nil {
		return status.Errorf(codes.Internal, "convert Date: %v", err)
	}

	u.Pb.Code, err = util.GetCode(ctx, tx, "purchase_returns", "DR")
	if err != nil {
		return err
	}

	query := `
		INSERT INTO purchase_returns (
			id, company_id, branch_id, branch_name, purchase_id, code, return_date, remark, 
			total_price, additional_disc_amount, additional_disc_percentage, 
			created_at, created_by, updated_at, updated_by
		) 
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return status.Errorf(codes.Internal, "Prepare insert purchase return: %v", err)
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx,
		u.Pb.GetId(),
		ctx.Value(app.Ctx("companyID")).(string),
		u.Pb.GetBranchId(),
		u.Pb.GetBranchName(),
		u.Pb.GetPurchase().GetId(),
		u.Pb.GetCode(),
		dateReturn,
		u.Pb.GetRemark(),
		u.Pb.GetTotalPrice(),
		u.Pb.GetAdditionalDiscAmount(),
		u.Pb.GetAdditionalDiscPercentage(),
		now,
		u.Pb.GetCreatedBy(),
		now,
		u.Pb.GetUpdatedBy(),
	)
	if err != nil {
		return status.Errorf(codes.Internal, "Exec insert purchase return: %v", err)
	}

	u.Pb.CreatedAt = now.String()
	u.Pb.UpdatedAt = u.Pb.CreatedAt

	for _, detail := range u.Pb.GetDetails() {
		purchaseReturnDetailModel := PurchaseReturnDetail{}
		purchaseReturnDetailModel.Pb = purchases.PurchaseReturnDetail{
			PurchaseReturnId: u.Pb.GetId(),
			ProductId:        detail.ProductId,
			ProductCode:      detail.ProductCode,
			ProductName:      detail.ProductName,
		}
		purchaseReturnDetailModel.PbPurchaseReturn = purchases.PurchaseReturn{
			Id:         u.Pb.Id,
			BranchId:   u.Pb.BranchId,
			BranchName: u.Pb.BranchName,
			Purchase:   u.Pb.Purchase,
			Code:       u.Pb.Code,
			ReturnDate: u.Pb.ReturnDate,
			Remark:     u.Pb.Remark,
			CreatedAt:  u.Pb.CreatedAt,
			CreatedBy:  u.Pb.CreatedBy,
			UpdatedAt:  u.Pb.UpdatedAt,
			UpdatedBy:  u.Pb.UpdatedBy,
		}
		err = purchaseReturnDetailModel.Create(ctx, tx)
		if err != nil {
			return err
		}
	}

	return nil
}

func (u *PurchaseReturn) Update(ctx context.Context, tx *sql.Tx) error {
	now := time.Now().UTC()
	u.Pb.UpdatedBy = ctx.Value(app.Ctx("userID")).(string)
	dateReturn, err := time.Parse("2006-01-02T15:04:05.000Z", u.Pb.GetReturnDate())
	if err != nil {
		return status.Errorf(codes.Internal, "convert purchase return date: %v", err)
	}

	query := `
		UPDATE purchase_returns SET
		purchase_id = $1,
		return_date = $2,
		remark = $3, 
		updated_at = $4, 
		updated_by= $5
		WHERE id = $6
	`
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return status.Errorf(codes.Internal, "Prepare update purchase return: %v", err)
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx,
		u.Pb.GetPurchase().GetId(),
		dateReturn,
		u.Pb.GetRemark(),
		now,
		u.Pb.GetUpdatedBy(),
		u.Pb.GetId(),
	)
	if err != nil {
		return status.Errorf(codes.Internal, "Exec update purchase return: %v", err)
	}

	u.Pb.UpdatedAt = now.String()

	return nil
}

func (u *PurchaseReturn) Delete(ctx context.Context, db *sql.DB) error {
	stmt, err := db.PrepareContext(ctx, `DELETE FROM purchase_returns WHERE id = $1`)
	if err != nil {
		return status.Errorf(codes.Internal, "Prepare delete purchase return: %v", err)
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, u.Pb.GetId())
	if err != nil {
		return status.Errorf(codes.Internal, "Exec delete purchase return: %v", err)
	}

	return nil
}

// ListQuery builder
func (u *PurchaseReturn) ListQuery(ctx context.Context, db *sql.DB, in *purchases.ListPurchaseReturnRequest) (string, []interface{}, *purchases.PurchaseReturnPaginationResponse, error) {
	var paginationResponse purchases.PurchaseReturnPaginationResponse
	query := `SELECT id, company_id, branch_id, branch_name, purchase_id, code, return_date, remark, created_at, created_by, updated_at, updated_by FROM purchase_returns`

	where := []string{"company_id = $1"}
	paramQueries := []interface{}{ctx.Value(app.Ctx("companyID")).(string)}

	if len(in.GetBranchId()) > 0 {
		paramQueries = append(paramQueries, in.GetBranchId())
		where = append(where, fmt.Sprintf(`branch_id = $%d`, len(paramQueries)))
	}

	if len(in.GetPurchaseId()) > 0 {
		paramQueries = append(paramQueries, in.GetPurchaseId())
		where = append(where, fmt.Sprintf(`purchase_id = $%d`, len(paramQueries)))
	}

	if len(in.GetPagination().GetSearch()) > 0 {
		paramQueries = append(paramQueries, in.GetPagination().GetSearch())
		where = append(where, fmt.Sprintf(`(code ILIKE $%d OR remark ILIKE $%d)`, len(paramQueries), len(paramQueries)))
	}

	{
		qCount := `SELECT COUNT(*) FROM purchase_returns`
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
