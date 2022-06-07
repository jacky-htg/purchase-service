package model

import (
	"context"
	"database/sql"
	"purchase/internal/pkg/app"
	"purchase/pb/purchases"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PurchaseReturnDetail struct {
	Pb               purchases.PurchaseReturnDetail
	PbPurchaseReturn purchases.PurchaseReturn
}

func (u *PurchaseReturnDetail) Get(ctx context.Context, tx *sql.Tx) error {
	query := `
		SELECT purchase_return_details.id, purchase_returns.company_id, purchase_return_details.purchase_return_id, purchase_return_details.product_id, purchase_return_details.quantity,
			purchase_return_details.price, purchase_return_details.disc_amount, purchase_return_details.disc_percentage, purchase_return_details.total_price
		FROM purchase_return_details 
		JOIN purchase_returns ON purchase_return_details.purchase_return_id = purchase_returns.id
		WHERE purchase_return_details.id = $1 AND purchase_return_details.purchase_return_id = $2
	`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return status.Errorf(codes.Internal, "Prepare statement Get purchase return detail: %v", err)
	}
	defer stmt.Close()

	var companyID string
	err = stmt.QueryRowContext(ctx, u.Pb.GetId(), u.Pb.GetPurchaseReturnId()).Scan(
		&u.Pb.Id, &companyID, &u.Pb.PurchaseReturnId, &u.Pb.ProductId, &u.Pb.Quantity,
		&u.Pb.Price, &u.Pb.DiscAmount, &u.Pb.DiscPercentage, &u.Pb.TotalPrice,
	)

	if err == sql.ErrNoRows {
		return status.Errorf(codes.NotFound, "Query Raw get by code purchase return detail: %v", err)
	}

	if err != nil {
		return status.Errorf(codes.Internal, "Query Raw get by code purchase return detail: %v", err)
	}

	if companyID != ctx.Value(app.Ctx("companyID")).(string) {
		return status.Error(codes.Unauthenticated, "its not your company")
	}

	return nil
}

func (u *PurchaseReturnDetail) Create(ctx context.Context, tx *sql.Tx) error {
	u.Pb.Id = uuid.New().String()
	query := `
		INSERT INTO purchase_return_details (id, purchase_return_id, product_id, quantity, price, disc_amount, disc_percentage, total_price) 
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return status.Errorf(codes.Internal, "Prepare insert purchase return detail: %v", err)
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx,
		u.Pb.GetId(),
		u.Pb.GetPurchaseReturnId(),
		u.Pb.GetProductId(),
		u.Pb.Quantity,
		u.Pb.Price,
		u.Pb.DiscAmount,
		u.Pb.DiscPercentage,
		u.Pb.TotalPrice,
	)
	if err != nil {
		return status.Errorf(codes.Internal, "Exec insert purchase return detail: %v", err)
	}

	return nil
}

func (u *PurchaseReturnDetail) Update(ctx context.Context, tx *sql.Tx) error {
	query := `
		UPDATE purchase_return_details SET
		quantity = $1,
		total_price = $2
		WHERE id = $3
	`
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return status.Errorf(codes.Internal, "Prepare update purchase return detail: %v", err)
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx,
		u.Pb.GetQuantity(),
		u.Pb.TotalPrice,
		u.Pb.GetId(),
	)
	if err != nil {
		return status.Errorf(codes.Internal, "Exec update purchase return detail: %v", err)
	}

	return nil
}

func (u *PurchaseReturnDetail) Delete(ctx context.Context, tx *sql.Tx) error {
	stmt, err := tx.PrepareContext(ctx, `DELETE FROM purchase_return_details WHERE id = $1 AND purchase_return_id = $2`)
	if err != nil {
		return status.Errorf(codes.Internal, "Prepare delete purchase return detail: %v", err)
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, u.Pb.GetId(), u.Pb.GetPurchaseReturnId())
	if err != nil {
		return status.Errorf(codes.Internal, "Exec delete purchase return detail: %v", err)
	}

	return nil
}
