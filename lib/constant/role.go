package constant

type RoleRequired string

const (
	RoleOwner  RoleRequired = "owner"
	RoleAdmin  RoleRequired = "admin"
	RoleMember RoleRequired = "member"
)

func (require RoleRequired) CanUse(user RoleRequired) bool {
	if require == "" {
		return true
	}
	switch require {
	case RoleOwner:
		return user == RoleOwner
	case RoleAdmin:
		return user == RoleAdmin || user == RoleOwner
	default:
		return true
	}
}
