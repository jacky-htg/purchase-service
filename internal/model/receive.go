package model

import (
	"context"
	"purchase/pb/inventories"
)

type Receive struct {
	Client inventories.ReceiveServiceClient
}

func (u *Receive) HasTransactionByPurchase(ctx context.Context, purchaseId string) (bool, error) {
	/* anyReceive, err := u.Client.HasReceive(app.SetMetadata(ctx), &inventories.Id{Id: purchaseId})
	if s, ok := status.FromError(err); ok {
		if s.Code() == codes.Unknown {
			err = status.Errorf(codes.Internal, "Error when calling Purchase.HasTreansaction service: %s", err)
		}

		return response, err
	}

	return anyReceive, nil */

	return false, nil
}
