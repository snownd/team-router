package logging

import "os"

// stderr 抽出便于测试。
func stderr() *os.File { return os.Stderr }
