package ai

import "strings"

func renderMessages(msgs []ChatMsg, width int) string {
	var b strings.Builder
	for i, msg := range msgs {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(msg.Role + ": " + msg.Content)
	}
	return b.String()
}
