package mappers

import (
	"github.com/magomedcoder/gen/api/pb/commonpb"
	"github.com/magomedcoder/gen/internal/domain"
	"strconv"
)

func UserToProto(user *domain.User) *commonpb.User {
	if user == nil {
		return nil
	}

	return &commonpb.User{
		Id:       strconv.Itoa(user.Id),
		Username: user.Username,
		Name:     user.Name,
		Surname:  user.Surname,
		Role:     int32(user.Role),
	}
}
