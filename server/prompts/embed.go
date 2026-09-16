// Package prompts 内嵌基座与阶段 prompt（核心资产，改动需用户确认话术）。
package prompts

import (
	"embed"
	"fmt"
)

//go:embed base.md stages/*.md
var fs embed.FS

// Base 基座 prompt（角色+语气铁律+方法论速览）
func Base() string {
	b, err := fs.ReadFile("base.md")
	if err != nil {
		panic(fmt.Sprintf("base.md 缺失: %v", err))
	}
	return string(b)
}

// Stage 阶段规范（S1-S7）；S8/S9 M4 再补
func Stage(s string) string {
	b, err := fs.ReadFile("stages/" + s + ".md")
	if err != nil {
		panic(fmt.Sprintf("stages/%s.md 缺失: %v", s, err))
	}
	return string(b)
}
