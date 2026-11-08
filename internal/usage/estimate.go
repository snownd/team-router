package usage

import (
	"strings"

	"github.com/snownd/team-router/internal/protocol"
)

// EstimateTokens 粗略 token 估算（~4 字符/token）。
func EstimateTokens(text string) int {
	n := len([]rune(text))
	if n == 0 {
		return 0
	}
	return (n + 3) / 4
}

// 图片 token 估算参数（粗粒度；预扣宁多勿少，多扣部分由结算退还）。
const (
	// imageBytesPerToken 图片字节 -> token 折算分母：按中等压缩率约 150B/token
	// （照片类约 200-400B/token、截图类 PNG 约 50-100B/token，取偏保守中间值）
	imageBytesPerToken = 150
	// imageTokenFloor 单图最低 token（对齐 OpenAI low-detail 85 token 下限）
	imageTokenFloor = 85
	// urlImageTokens URL 引用图片无法得知实际尺寸，按 1024x1024 高细节保守估算
	urlImageTokens = 1100
)

// estimateImageTokens 估算单个图片块的 token 成本。
// base64 数据按解码后字节数折算；URL 引用（OpenAI image_url 形态）按固定保守值。
func estimateImageTokens(b protocol.Block) int {
	if b.MimeType == "image_url" || strings.HasPrefix(b.Data, "http://") || strings.HasPrefix(b.Data, "https://") {
		return urlImageTokens
	}
	if b.Data == "" {
		return imageTokenFloor
	}
	raw := len(b.Data) * 3 / 4 // base64 解码后近似字节数
	t := raw / imageBytesPerToken
	if t < imageTokenFloor {
		t = imageTokenFloor
	}
	return t
}

// EstimateRequestTokens 估算请求输入 token（system + 全部消息文本 + 图片 + 工具定义）。
func EstimateRequestTokens(req *protocol.Request) int {
	total := EstimateTokens(req.System)
	for _, m := range req.Messages {
		total += EstimateTokens(m.Text)
		for _, b := range m.Blocks {
			if b.Type == protocol.BlockTypeImage {
				total += estimateImageTokens(b)
				continue
			}
			total += EstimateTokens(b.Text)
		}
		for _, tc := range m.ToolCalls {
			total += EstimateTokens(tc.Name) + EstimateTokens(tc.Arguments)
		}
	}
	for _, t := range req.Tools {
		total += EstimateTokens(t.Name) + EstimateTokens(t.Description) + EstimateTokens(string(t.Parameters))
	}
	return total
}
