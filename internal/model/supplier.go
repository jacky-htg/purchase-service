package model

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jacky-htg/erp-pkg/app"
	"github.com/jacky-htg/erp-proto/go/pb/purchases"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Supplier struct {
	Pb purchases.Supplier
}

func (u *Supplier) Get(ctx context.Context, db *sql.DB) error {
	query := `
		SELECT id, company_id, code, name, address, phone, created_at, created_by, updated_at, updated_by 
		FROM suppliers WHERE id = $1 AND company_id = $2
	`

	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		return status.Errorf(codes.Internal, "Prepare statement Get supplier: %v", err)
	}
	defer stmt.Close()

	var companyID string
	var createdAt, updatedAt time.Time
	err = stmt.QueryRowContext(ctx, u.Pb.GetId(), ctx.Value(app.Ctx("companyID")).(string)).Scan(
		&u.Pb.Id, &companyID, &u.Pb.Code, &u.Pb.Name, &u.Pb.Address, &u.Pb.Phone, &createdAt, &u.Pb.CreatedBy, &updatedAt, &u.Pb.UpdatedBy,
	)

	if err == sql.ErrNoRows {
		return status.Errorf(codes.NotFound, "Query Raw get supplier: %v", err)
	}

	if err != nil {
		return status.Errorf(codes.Internal, "Query Raw get supplier: %v", err)
	}

	if companyID != ctx.Value(app.Ctx("companyID")).(string) {
		return status.Error(codes.Unauthenticated, "its not your company data")
	}

	u.Pb.CreatedAt = createdAt.String()
	u.Pb.UpdatedAt = updatedAt.String()

	return nil
}

func (u *Supplier) GetByCode(ctx context.Context, db *sql.DB) error {
	query := `
		SELECT id, company_id, code, name, address, phone, created_at, created_by, updated_at, updated_by 
		FROM suppliers WHERE company_id = $1 AND code = $2
	`

	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		return status.Errorf(codes.Internal, "Prepare statement Get supplier by code: %v", err)
	}
	defer stmt.Close()

	var companyID string
	var createdAt, updatedAt time.Time
	err = stmt.QueryRowContext(ctx, ctx.Value(app.Ctx("companyID")).(string), u.Pb.GetCode()).Scan(
		&u.Pb.Id, &companyID, &u.Pb.Code, &u.Pb.Name, &u.Pb.Address, &u.Pb.Phone, &createdAt, &u.Pb.CreatedBy, &updatedAt, &u.Pb.UpdatedBy,
	)

	if err == sql.ErrNoRows {
		return status.Errorf(codes.NotFound, "Query Raw get supplier by code: %v", err)
	}

	if err != nil {
		return status.Errorf(codes.Internal, "Query Raw get supplier by code: %v", err)
	}

	u.Pb.CreatedAt = createdAt.String()
	u.Pb.UpdatedAt = updatedAt.String()

	return nil
}

func (u *Supplier) Create(ctx context.Context, db *sql.DB) error {
	u.Pb.Id = uuid.New().String()
	now := time.Now().UTC()
	u.Pb.CreatedBy = ctx.Value(app.Ctx("userID")).(string)
	u.Pb.UpdatedBy = ctx.Value(app.Ctx("userID")).(string)

	query := `
		INSERT INTO suppliers (id, company_id, code, name, address, phone, created_at, created_by, updated_at, updated_by) 
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		return status.Errorf(codes.Internal, "Prepare insert supplier: %v", err)
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx,
		u.Pb.GetId(),
		ctx.Value(app.Ctx("companyID")).(string),
		u.Pb.GetCode(),
		u.Pb.GetName(),
		u.Pb.GetAddress(),
		u.Pb.GetPhone(),
		now,
		u.Pb.GetCreatedBy(),
		now,
		u.Pb.GetUpdatedBy(),
	)
	if err != nil {
		return status.Errorf(codes.Internal, "Exec insert supplier: %v", err)
	}

	u.Pb.CreatedAt = now.String()
	u.Pb.UpdatedAt = u.Pb.CreatedAt

	return nil
}

func (u *Supplier) Update(ctx context.Context, db *sql.DB) error {
	now := time.Now().UTC()
	u.Pb.UpdatedBy = ctx.Value(app.Ctx("userID")).(string)

	query := `
		UPDATE suppliers SET
		name = $1,
		address = $2,
		phone = $3, 
		updated_at = $4, 
		updated_by= $5
		WHERE id = $6 AND company_id = $7
	`
	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		return status.Errorf(codes.Internal, "Prepare update supplier: %v", err)
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx,
		u.Pb.GetName(),
		u.Pb.GetAddress(),
		u.Pb.GetPhone(),
		now,
		u.Pb.GetUpdatedBy(),
		u.Pb.GetId(),
		ctx.Value(app.Ctx("companyID")).(string),
	)
	if err != nil {
		return status.Errorf(codes.Internal, "Exec update supplier: %v", err)
	}

	u.Pb.UpdatedAt = now.String()

	return nil
}

func (u *Supplier) Delete(ctx context.Context, db *sql.DB) error {
	stmt, err := db.PrepareContext(ctx, `DELETE FROM suppliers WHERE company_id = $1 AND id = $2`)
	if err != nil {
		return status.Errorf(codes.Internal, "Prepare delete supplier: %v", err)
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, ctx.Value(app.Ctx("companyID")).(string), u.Pb.GetId())
	if err != nil {
		return status.Errorf(codes.Internal, "Exec delete supplier: %v", err)
	}

	return nil
}

func (u *Supplier) ListQuery(ctx context.Context, db *sql.DB, in *purchases.Pagination) (string, []interface{}, *purchases.SupplierPaginationResponse, error) {
	var paginationResponse purchases.SupplierPaginationResponse
	query := `SELECT id, company_id, code, name, address, phone, created_at, created_by, updated_at, updated_by FROM suppliers`
	where := []string{"company_id = $1"}
	paramQueries := []interface{}{ctx.Value(app.Ctx("companyID")).(string)}

	if len(in.GetSearch()) > 0 {
		paramQueries = append(paramQueries, "%"+in.GetSearch()+"%")
		where = append(where, fmt.Sprintf(`(name ILIKE $%d OR code ILIKE $%d OR address ILIKE $%d OR phone ILIKE $%d)`, len(paramQueries), len(paramQueries), len(paramQueries), len(paramQueries)))
	}

	{
		qCount := `SELECT COUNT(*) FROM suppliers`
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

	if len(in.GetOrderBy()) == 0 || !(in.GetOrderBy() == "name" || in.GetOrderBy() == "code") {
		if in == nil {
			in = &purchases.Pagination{OrderBy: "created_at"}
		} else {
			in.OrderBy = "created_at"
		}
	}

	query += ` ORDER BY ` + in.GetOrderBy() + ` ` + in.GetSort().String()

	if in.GetLimit() > 0 {
		query += fmt.Sprintf(` LIMIT $%d OFFSET $%d`, (len(paramQueries) + 1), (len(paramQueries) + 2))
		paramQueries = append(paramQueries, in.GetLimit(), in.GetOffset())
	}

	return query, paramQueries, &paginationResponse, nil
}
