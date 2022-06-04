package model

import (
	"context"
	"io"
	"purchase/internal/pkg/app"
	"purchase/pb/users"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Branch struct {
	UserClient   users.UserServiceClient
	RegionClient users.RegionServiceClient
	BranchClient users.BranchServiceClient
	Id           string
}

func (u *Branch) IsYourBranch(ctx context.Context) error {
	userLogin, err := getUserLogin(ctx, u.UserClient)
	if err != nil {
		return err
	}

	if len(userLogin.GetBranchId()) > 0 {
		if userLogin.GetBranchId() != u.Id {
			return status.Error(codes.Unauthenticated, "its not your branch")
		}
	} else if len(userLogin.GetRegionId()) > 0 {
		region, err := getRegion(ctx, u.RegionClient, &users.Region{Id: userLogin.GetRegionId()})
		if err != nil {
			return err
		}
		err = checkYourBranch(region.GetBranches(), u.Id)
		if err != nil {
			return err
		}
	} else {
		branches, err := getBranches(ctx, u.BranchClient)
		if err != nil {
			return err
		}
		err = checkYourBranch(branches, u.Id)
		if err != nil {
			return err
		}
	}

	return nil
}

func checkYourBranch(branches []*users.Branch, branchID string) error {
	isYourBranch := false
	for _, branch := range branches {
		if branch.GetId() == branchID {
			isYourBranch = true
			break
		}
	}

	if !isYourBranch {
		return status.Error(codes.Unauthenticated, "its not your branch")
	}

	return nil
}

func getUserLogin(ctx context.Context, userClient users.UserServiceClient) (*users.User, error) {
	userLogin, err := userClient.View(app.SetMetadata(ctx), &users.Id{Id: ctx.Value(app.Ctx("userID")).(string)})

	if err != nil {
		return &users.User{}, status.Errorf(codes.Internal, "Error when calling user service: %v", err)
	}

	return userLogin, nil
}

func getRegion(ctx context.Context, regionClient users.RegionServiceClient, r *users.Region) (*users.Region, error) {
	region, err := regionClient.View(app.SetMetadata(ctx), &users.Id{Id: r.GetId()})

	if err != nil {
		return &users.Region{}, status.Errorf(codes.Internal, "Error when calling region service: %v", err)
	}

	return region, nil
}

func getBranches(ctx context.Context, branchClient users.BranchServiceClient) ([]*users.Branch, error) {
	var list []*users.Branch
	var err error
	stream, err := branchClient.List(app.SetMetadata(ctx), &users.ListBranchRequest{})
	if err != nil {
		return list, status.Errorf(codes.Internal, "Error when calling branches service: %s", err)
	}

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return list, status.Errorf(codes.Internal, "cannot receive %v", err)
		}
		list = append(list, resp.GetBranch())
	}
	return list, err
}

func (u *Branch) Get(ctx context.Context) (*users.Branch, error) {
	branch, err := u.BranchClient.View(app.SetMetadata(ctx), &users.Id{Id: u.Id})
	if err != nil {
		return &users.Branch{}, status.Errorf(codes.Internal, "Error when calling branch service: %v", err)
	}

	return branch, nil
}
