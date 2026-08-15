package theme

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type SlotKey string

const (
	SlotSelected    SlotKey = "selected"
	SlotDim         SlotKey = "dim"
	SlotError       SlotKey = "error"
	SlotUser        SlotKey = "user"
	SlotAI          SlotKey = "ai"
	SlotHighlightFg SlotKey = "highlightFg"
	SlotHighlightBg SlotKey = "highlightBg"
	SlotSuccess     SlotKey = "success"
	SlotWarning     SlotKey = "warning"
	SlotBorder      SlotKey = "border"
	SlotTitle       SlotKey = "title"
	SlotAccent      SlotKey = "accent"
	SlotMuted       SlotKey = "muted"
)

func slotKeys() []SlotKey {
	return []SlotKey{
		SlotSelected, SlotDim, SlotError, SlotUser, SlotAI,
		SlotHighlightFg, SlotHighlightBg, SlotSuccess, SlotWarning,
		SlotBorder, SlotTitle, SlotAccent, SlotMuted,
	}
}

type CompleteColor struct {
	TrueColor string
	ANSI256   string
	ANSI      string
}

var hexRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (c CompleteColor) Validate() error {
	if c.TrueColor == "" || !hexRe.MatchString(c.TrueColor) {
		return fmt.Errorf("TrueColor 需为 #RRGGBB, got %q", c.TrueColor)
	}
	if c.ANSI == "" {
		return fmt.Errorf("ANSI 16 色档必填")
	}
	if n, err := strconv.Atoi(c.ANSI); err != nil || n < 0 || n > 255 {
		return fmt.Errorf("ANSI 应为 0-255, got %q", c.ANSI)
	}
	if c.ANSI256 != "" {
		if n, err := strconv.Atoi(c.ANSI256); err != nil || n < 0 || n > 255 {
			return fmt.Errorf("ANSI256 应为 0-255 或空串, got %q", c.ANSI256)
		}
	}
	return nil
}

type Slot struct {
	Light CompleteColor
	Dark  CompleteColor
}

func (s Slot) Validate() error {
	if err := s.Light.Validate(); err != nil {
		return fmt.Errorf("Light: %w", err)
	}
	if err := s.Dark.Validate(); err != nil {
		return fmt.Errorf("Dark: %w", err)
	}
	return nil
}

type Theme struct {
	Name  Name
	Slots map[SlotKey]Slot
}

func (t Theme) Validate() error {
	if strings.TrimSpace(string(t.Name)) == "" {
		return fmt.Errorf("主题名不能为空")
	}
	if len(t.Slots) != len(slotKeys()) {
		return fmt.Errorf("槽位数 %d != 需要 %d", len(t.Slots), len(slotKeys()))
	}
	for _, k := range slotKeys() {
		s, ok := t.Slots[k]
		if !ok {
			return fmt.Errorf("缺少槽位 %q", k)
		}
		if err := s.Validate(); err != nil {
			return fmt.Errorf("槽位 %q: %w", k, err)
		}
	}
	return nil
}
