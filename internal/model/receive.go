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
	if err != nil {
		// TODO : set status error only if respone status error from product service is 'unknow'
		return false, status.Errorf(codes.Internal, "Error when calling receive service: %v", err)
	}

	return anyReceive, nil */

	return false, nil
}
