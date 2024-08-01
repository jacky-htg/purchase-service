package model

import (
	"context"
	"io"

	"github.com/jacky-htg/erp-proto/go/pb/inventories"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Receive struct {
	Client inventories.ReceiveServiceClient
}

func (u *Receive) HasTransactionByPurchase(ctx context.Context, purchaseId string) (bool, error) {
	streamClient, err := u.Client.List(ctx, &inventories.ListReceiveRequest{PurchaseId: purchaseId})
	if s, ok := status.FromError(err); !ok {
		if s.Code() == codes.Unknown {
			err = status.Errorf(codes.Internal, "Error when calling Purchase.HasTreansaction service: %s", err)
		}

		return false, err
	}

	var response []*inventories.ListReceiveResponse
	for {
		resp, err := streamClient.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, status.Errorf(codes.Internal, "cannot receive %v", err)
		}

		response = append(response, resp)
	}

	if len(response) > 0 {
		return true, nil
	}

	return false, nil
}
