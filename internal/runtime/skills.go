package runtime

import (
	"context"
	skill "csgclaw/internal/skill/state"
)

// SkillsReconciler 将已保存的 Skill 状态写入 runtime 配置。
// 启动流程也必须应用这些状态，以支持停止后的修改和重建。
type SkillsReconciler interface {
	ReconcileSkills(context.Context, Handle, map[string]skill.State) error
}

// SkillDiskInspector compares the runtime's on-disk configuration with the
// desired setting without creating directories or refreshing the session.
type SkillDiskInspector interface {
	SkillDiskSync(Layout, string, bool, map[string]any) (string, error)
}
