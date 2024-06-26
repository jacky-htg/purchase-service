package model

import (
	"context"
	"io"
	"purchase/pb/inventories"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Product struct {
	Client inventories.ProductServiceClient
	Pb     *inventories.Product
	Id     string
}

func (u *Product) Get(ctx context.Context) error {
	product, err := u.Client.View(ctx, &inventories.Id{Id: u.Id})
	if s, ok := status.FromError(err); !ok {
		if s.Code() == codes.Unknown {
			err = status.Errorf(codes.Internal, "Error when calling Product.Get service: %s", err)
		}

		return err
	}

	u.Pb = product

	return nil
}

func (u *Product) List(ctx context.Context, in *inventories.ListProductRequest) ([]*inventories.ListProductResponse, error) {
	var response []*inventories.ListProductResponse
	streamClient, err := u.Client.List(ctx, in)

	if s, ok := status.FromError(err); !ok {
		if s.Code() == codes.Unknown {
			err = status.Errorf(codes.Internal, "Error when calling Product.List service: %s", err)
		}

		return response, err
	}

	for {
		resp, err := streamClient.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return response, status.Errorf(codes.Internal, "cannot receive %v", err)
		}

		response = append(response, resp)
	}

	return response, nil
}
