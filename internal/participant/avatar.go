package participant

import (
	crand "crypto/rand"
	"math/big"
	"strings"
	"time"
)

var builtInAvatarOptions = []string{
	"avatar/3D-1.png",
	"avatar/3D-2.png",
	"avatar/3D-3.png",
	"avatar/3D-4.png",
	"avatar/3D-5.png",
	"avatar/3D-6.png",
	"avatar/3D-7.png",
	"avatar/3D-8.png",
	"avatar/cartoon-1.png",
	"avatar/cartoon-2.png",
	"avatar/cartoon-3.png",
	"avatar/cartoon-4.png",
	"avatar/cartoon-5.png",
	"avatar/cartoon-6.png",
	"avatar/cartoon-7.png",
	"avatar/cartoon-8.png",
	"avatar/pic-1.png",
	"avatar/pic-2.png",
	"avatar/pic-3.png",
	"avatar/pic-4.png",
	"avatar/pic-5.png",
	"avatar/pic-6.png",
	"avatar/pic-7.png",
	"avatar/pic-8.png",
}

func (s *Service) defaultParticipantAvatar(current string) string {
	if avatar := strings.TrimSpace(current); avatar != "" {
		return avatar
	}
	if s == nil || s.store == nil {
		return randomBuiltInAvatar(builtInAvatarOptions)
	}
	used := make(map[string]struct{})
	for _, item := range s.store.List(ListOptions{}) {
		if avatar := strings.TrimSpace(item.Avatar); avatar != "" {
			used[avatar] = struct{}{}
		}
	}
	available := make([]string, 0, len(builtInAvatarOptions))
	for _, avatar := range builtInAvatarOptions {
		if _, ok := used[avatar]; ok {
			continue
		}
		available = append(available, avatar)
	}
	if len(available) > 0 {
		return randomBuiltInAvatar(available)
	}
	return randomBuiltInAvatar(builtInAvatarOptions)
}

func randomBuiltInAvatar(candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}
	return candidates[randomBuiltInAvatarIndex(len(candidates))]
}

func randomBuiltInAvatarIndex(length int) int {
	if length <= 1 {
		return 0
	}
	value, err := crand.Int(crand.Reader, big.NewInt(int64(length)))
	if err == nil {
		return int(value.Int64())
	}
	return int(time.Now().UnixNano() % int64(length))
}
