package monitor

import (
	"fmt"
	"strconv"
	"strings"
)

// 复合采集命令协议：把固定命令表合并为一条 /bin/sh 兼容的复合命令，
// 一次 SSH 执行后在 Go 侧按分隔符切分，喂给原有 parse_* 解析器（解析器零改动）。
// 每个采集步骤在远端展开为三行：
//
//	echo '__OWL_SECTION__<name>__'     段开始标记
//	<command>                          原采集命令（保留原样，含管道/重定向）
//	echo '__OWL_RC__<name>__'$?        段结束标记，附带该命令的退出码
//
// 全部为 POSIX sh 语法（纯 echo + $?，无 bash-ism）。切分规则：
//   - 段内容 = 开始标记之后到 rc 标记之前的文本（去首尾换行）；
//   - rc 标记行缺失（命令中途被杀/输出截断）→ rcPresent=false，按执行失败处理；
//   - 复合命令整体的 stderr 由执行器追加在全部输出末尾，位于最后一个
//     rc 标记之后，切分时自然丢弃，不会污染任何段。
const (
	sectionMarkerPrefix = "__OWL_SECTION__"
	rcMarkerPrefix      = "__OWL_RC__"
	sectionMarkerSuffix = "__"
)

// compositeSection 复合输出中单个采集命令的切分结果。
type compositeSection struct {
	output    string // 该命令的标准输出（去 rc 标记行）
	rc        int    // 该命令的退出码（$?）
	rcPresent bool   // rc 标记是否存在（false = 输出被截断/命令未跑完）
}

// buildCompositeCommand 把采集命令表合并为一条复合命令。
// 段内命令失败不会中断后续命令（无 set -e），各自退出码经 rc 标记上报。
func buildCompositeCommand(steps []collectStep) string {
	var b strings.Builder
	for _, step := range steps {
		fmt.Fprintf(&b, "echo '%s%s%s'\n", sectionMarkerPrefix, step.name, sectionMarkerSuffix)
		b.WriteString(step.command)
		fmt.Fprintf(&b, "\necho '%s%s%s'$?\n", rcMarkerPrefix, step.name, sectionMarkerSuffix)
	}
	return b.String()
}

// splitCompositeOutput 按分隔符协议切分复合输出。
// steps 决定期望的段与顺序；缺失的段不出现在返回 map 中。
func splitCompositeOutput(raw string, steps []collectStep) map[string]compositeSection {
	// 按期望顺序定位每个段的开始标记（各段标记文本互异，顺序定位互不干扰）
	starts := make([]int, len(steps))
	searchFrom := 0
	for i, step := range steps {
		marker := sectionMarkerPrefix + step.name + sectionMarkerSuffix
		idx := strings.Index(raw[searchFrom:], marker)
		if idx < 0 {
			starts[i] = -1
			continue
		}
		starts[i] = searchFrom + idx
		searchFrom = starts[i] + len(marker)
	}

	sections := make(map[string]compositeSection, len(steps))
	for i, step := range steps {
		if starts[i] < 0 {
			continue
		}
		contentStart := starts[i] + len(sectionMarkerPrefix) + len(step.name) + len(sectionMarkerSuffix)
		contentEnd := len(raw)
		for j := i + 1; j < len(steps); j++ {
			if starts[j] >= 0 {
				contentEnd = starts[j]
				break
			}
		}
		content := raw[contentStart:contentEnd]

		rcMarker := rcMarkerPrefix + step.name + sectionMarkerSuffix
		ridx := strings.LastIndex(content, rcMarker)
		if ridx < 0 {
			// rc 标记缺失：命令被截断，输出保留供调试，rcPresent=false
			sections[step.name] = compositeSection{
				output:    strings.TrimPrefix(strings.TrimRight(content, "\n"), "\n"),
				rcPresent: false,
			}
			continue
		}
		rcPart := content[ridx+len(rcMarker):]
		if nl := strings.IndexByte(rcPart, '\n'); nl >= 0 {
			rcPart = rcPart[:nl]
		}
		rc, err := strconv.Atoi(strings.TrimSpace(rcPart))
		if err != nil {
			// rc 值损坏，按未知处理
			sections[step.name] = compositeSection{
				output:    strings.TrimPrefix(strings.TrimRight(content[:ridx], "\n"), "\n"),
				rcPresent: false,
			}
			continue
		}
		sections[step.name] = compositeSection{
			output:    strings.TrimPrefix(strings.TrimRight(content[:ridx], "\n"), "\n"),
			rc:        rc,
			rcPresent: true,
		}
	}
	return sections
}
