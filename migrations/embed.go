// Package migrations 内嵌全部 SQL 迁移文件，供 migrator 使用。
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
