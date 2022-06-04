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

type PurchaseDetail struct {
	Pb         purchases.PurchaseDetail
	PbPurchase purchases.Purchase
}

func (u *PurchaseDetail) Get(ctx context.Context, tx *sql.Tx) error {
	query := `
		SELECT purchase_details.id, purchases.company_id, purchase_details.purchase_id, purchase_details.product_id, price, disc_amount, disc_prosentation 
		FROM purchase_details 
		JOIN purchases ON purchase_details.purchase_id = purchases.id
		WHERE purchase_details.id = $1 AND purchase_details.purchase_id = $2
	`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return status.Errorf(codes.Internal, "Prepare statement Get purchase detail: %v", err)
	}
	defer stmt.Close()

	var companyID string
	var discProsentation *float32
	err = stmt.QueryRowContext(ctx, u.Pb.GetId(), u.Pb.GetPurchaseId()).Scan(
		&u.Pb.Id, &companyID, &u.Pb.PurchaseId, &u.Pb.ProductId, &u.Pb.Price, &u.Pb.DiscAmount, &discProsentation,
	)

	if err == sql.ErrNoRows {
		return status.Errorf(codes.NotFound, "Query Raw get by code purchase detail: %v", err)
	}

	if err != nil {
		return status.Errorf(codes.Internal, "Query Raw get by code purchase detail: %v", err)
	}

	if companyID != ctx.Value(app.Ctx("companyID")).(string) {
		return status.Error(codes.Unauthenticated, "its not your company")
	}

	if discProsentation == nil {
		u.Pb.DiscProsentation = 0
	} else {
		u.Pb.DiscProsentation = *discProsentation
	}

	return nil
}

func (u *PurchaseDetail) Create(ctx context.Context, tx *sql.Tx) error {
	u.Pb.Id = uuid.New().String()
	query := `
		INSERT INTO purchase_details (id, purchase_id, product_id, price, disc_amount, disc_prosentation) 
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return status.Errorf(codes.Internal, "Prepare insert purchase detail: %v", err)
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx,
		u.Pb.GetId(),
		u.Pb.GetPurchaseId(),
		u.Pb.GetProductId(),
		u.Pb.GetPrice(),
		u.Pb.GetDiscAmount(),
		u.Pb.GetDiscProsentation(),
	)
	if err != nil {
		return status.Errorf(codes.Internal, "Exec insert purchase detail: %v", err)
	}

	return nil
}

func (u *PurchaseDetail) Delete(ctx context.Context, tx *sql.Tx) error {
	stmt, err := tx.PrepareContext(ctx, `DELETE FROM purchase_details WHERE id = $1 AND purchase_id = $2`)
	if err != nil {
		return status.Errorf(codes.Internal, "Prepare delete purchase detail: %v", err)
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, u.Pb.GetId(), u.Pb.GetPurchaseId())
	if err != nil {
		return status.Errorf(codes.Internal, "Exec delete purchase detail: %v", err)
	}

	return nil
}
