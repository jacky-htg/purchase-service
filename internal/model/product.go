package model

import (
	"context"
	"purchase/internal/pkg/app"
	"purchase/pb/inventories"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Product struct {
	Client inventories.ProductServiceClient
	Id     string
}

func (u *Product) Get(ctx context.Context) (*inventories.Product, error) {
	product, err := u.Client.View(app.SetMetadata(ctx), &inventories.Id{Id: u.Id})
	if err != nil {
		// TODO : set status error only if respone status error from product service is 'unknow'
		return &inventories.Product{}, status.Errorf(codes.Internal, "Error when calling product service: %v", err)
	}

	return product, nil
}
