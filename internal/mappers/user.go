package mappers

import (
	"strconv"

	"github.com/magomedcoder/gen/api/pb"
	"github.com/magomedcoder/gen/internal/domain"
)

func UserToProto(user *domain.User) *pb.User {
	if user == nil {
		return nil
	}

	return &pb.User{
		Id:       strconv.Itoa(user.Id),
		Username: user.Username,
		Name:     user.Name,
		Surname:  user.Surname,
		Role:     int32(user.Role),
	}
}
