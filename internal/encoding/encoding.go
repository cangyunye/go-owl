package encoding

// Charset 表征字节层面的字符编码。独立于 Language 解析：英文用户可能仍是
// GBK，中文用户可能是 UTF-8。
type Charset int

const (
	UTF8 Charset = iota
	GBK
	Big5
)

func (c Charset) String() string {
	switch c {
	case GBK:
		return "GBK"
	case Big5:
		return "Big5"
	default:
		return "UTF-8"
	}
}