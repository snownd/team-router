package sso

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/snownd/team-router/internal/model"
	"github.com/snownd/team-router/internal/repository"
)

// UserProvisioner 的仓储实现：JIT 建号 + 资料更新。
type userProvisioner struct {
	userRepo *repository.UserRepo
	deptRepo *repository.DepartmentRepo
}

// NewUserProvisioner 构造。
func NewUserProvisioner(userRepo *repository.UserRepo, deptRepo *repository.DepartmentRepo) UserProvisioner {
	return &userProvisioner{userRepo: userRepo, deptRepo: deptRepo}
}

// FindBySSO 查找 SSO 用户。
func (p *userProvisioner) FindBySSO(ctx context.Context, provider, externalID string) (*model.User, error) {
	return p.userRepo.FindBySSO(ctx, provider, externalID)
}

// ProvisionOrCreate 首次登录自动建号并挂根部门（JIT）。
func (p *userProvisioner) ProvisionOrCreate(ctx context.Context, providerName string, id *Identity) (*model.User, error) {
	if id == nil || id.ExternalID == "" {
		return nil, fmt.Errorf("SSO 身份信息不完整")
	}
	user, err := p.userRepo.FindBySSO(ctx, providerName, id.ExternalID)
	if err == nil {
		// 更新资料
		if user.DisplayName != id.DisplayName || user.Email != id.Email {
			user.DisplayName = id.DisplayName
			user.Email = id.Email
			if err := p.userRepo.Update(ctx, user, "display_name", "email"); err != nil {
				return nil, err
			}
		}
		return user, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	// 建号：username 用 provider-externalId 截断保证唯一
	username := fmt.Sprintf("%s_%s", providerName, id.ExternalID)
	if len(username) > 60 {
		username = username[:60]
	}
	user = &model.User{
		Username:      username,
		DisplayName:   orStr(id.DisplayName, username),
		Email:         id.Email,
		Role:          model.RoleUser,
		Status:        model.UserStatusEnabled,
		SSOProvider:   providerName,
		SSOExternalID: id.ExternalID,
	}
	if err := p.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}
	// JIT 用户挂到根部门（parent_id IS NULL 的第一个）
	depts, err := p.deptRepo.Children(ctx, 0)
	if err == nil && len(depts) > 0 {
		_ = p.deptRepo.SetUserDepartments(ctx, user.ID, []int64{depts[0].ID}, depts[0].ID)
	}
	return user, nil
}

func orStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
