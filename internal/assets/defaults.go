package assets

const (
	DefaultManagerAvatar      = "avatar/manager.png"
	LegacyManagerAvatarSymbol = "MG"
)

func NormalizeManagerAvatar(avatar string) string {
	if avatar == "" || avatar == LegacyManagerAvatarSymbol {
		return DefaultManagerAvatar
	}
	return avatar
}
