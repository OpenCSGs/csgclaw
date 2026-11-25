package builtin

import (
	"context"
	"fmt"
	"io/fs"
	"os"

	"csgclaw/internal/skill/system"
)

// TreeFingerprint of the complete historical embedded trees. Legacy workers
// can lack an origin record. A name or a matching SKILL.md alone is not proof
// that the user's scripts and other assets are still the shipped versions.
var systemUpgradeFrom = map[string][]string{
	"skill-installer": {
		"e85a8549c027b198915a6ad933339b5f82fc28433ff7eedb13b63f0e210e9123", // 9e9c6b7a
		"099b5dde06b3dca3e257e426b7e0f8cd50af18945213226f5ebe7213afccab3c", // ffdf821f
		"0ee82532fe5ff0027a305a198293e58a7e2af2de7cff0dd10d97b2df1410601d", // 5c857203
		"146ada624c0259e0c373124e975bf6cf19d79b78e3fc0a8102d6980374f0531d", // 70836bae
		"c7781d79f8ef501b46aead1797cb7c251aed940887f3136aa1617119b2a804d6", // a93f89a0
		"6fa88da021e3c5a09a5f4ac06632caf69c9c86c68498063c1719379823623fa9", // bddad5a5
		"57003e42114b5f570d4f163995b07415e64c486fc056f499baac1d3d1614bfa9", // 1c12dbff
		"d59c9faf46166350f9c3ed9b1019c31b8abc6ab05491648f5234125b067f3c1d", // 30ecd187
		"7f79d744277103568553b5983a085f013072546372608292d6908d1f546852fd", // c6b998cc
	},
	"skill-creator": {
		"e9f695889c52cbef3f18fd6d02ed815cbe3e41adf98e0b38e8a435b411472308", // 14e284e4
		"b173d1c26da864b326ecd965e5a0312d954e817018833e97aa99aa07fbfbd02a", // 5c857203
	},
}

// UpgradeSystemCopies runs before starting an existing host worker. Skill
// selection and enablement are owned by the Agent and are not modified.
func UpgradeSystemCopies(ctx context.Context, root string) error {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	for name, legacy := range systemUpgradeFrom {
		source, err := system.ResolveSource(name)
		if err != nil {
			return err
		}
		tree, err := fs.Sub(source.FS, source.RootPath)
		if err != nil {
			return err
		}
		if err := syncBundle(ctx, root, name, "", tree, legacy); err != nil {
			return fmt.Errorf("upgrade system skill %s: %w", name, err)
		}
	}
	return nil
}
