package protocol

import (
	"bufio"
	"io"
	"strings"
)

// SSEReader 逐条读取 SSE data 负载（兼容 \n 与 \r\n、多行 data）。
type SSEReader struct {
	sc *bufio.Scanner
}

// NewSSEReader 构造。
func NewSSEReader(r io.Reader) *SSEReader {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	return &SSEReader{sc: sc}
}

// Next 返回下一条事件（合并多行 data）；流结束返回 io.EOF。
// 返回值：event 名（可能为空）、data（可能为多行拼接）。
func (s *SSEReader) Next() (event string, data string, err error) {
	var lines []string
	for s.sc.Scan() {
		line := strings.TrimRight(s.sc.Text(), "\r")
		if line == "" {
			// 事件边界
			if len(lines) > 0 {
				return event, strings.Join(lines, "\n"), nil
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, ":"):
			continue // 注释/心跳
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			lines = append(lines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := s.sc.Err(); err != nil {
		return "", "", err
	}
	if len(lines) > 0 {
		return event, strings.Join(lines, "\n"), nil
	}
	return "", "", io.EOF
}
