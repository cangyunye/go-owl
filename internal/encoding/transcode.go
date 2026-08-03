package encoding

import (
	"bytes"
	"io"
	"os"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

// NewEncoder 返回将 UTF-8 文本写入 w 时转成目标字符集的 Writer。
func NewEncoder(w io.Writer, cs Charset) io.Writer {
	if cs == UTF8 {
		return w
	}
	return transform.NewWriter(w, encFor(cs).NewEncoder())
}

// NewDecoder 返回读取目标字符集字节并解码为 UTF-8 的 Reader。
func NewDecoder(r io.Reader, cs Charset) io.Reader {
	if cs == UTF8 {
		return r
	}
	return transform.NewReader(r, encFor(cs).NewDecoder())
}

func encFor(cs Charset) encoding.Encoding {
	switch cs {
	case GBK:
		return simplifiedchinese.GBK
	case Big5:
		return traditionalchinese.Big5
	default:
		return encoding.Nop
	}
}

// ReadFileBytes 读取文件并返回 UTF-8 字节。自动识别 BOM / 是否合法 UTF-8，
// 否则按 *Charset 或 ResolveCharset() 解码。不修改原文件。
func ReadFileBytes(path string, cs *Charset) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	data = stripBOM(data)
	if utf8.Valid(data) {
		return data, nil
	}
	if cs == nil {
		c := ResolveCharset()
		cs = &c
	}
	return DecodeCharset(data, *cs)
}

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// stripBOM 剥离 UTF-8 BOM。
func stripBOM(data []byte) []byte {
	if bytes.HasPrefix(data, utf8BOM) {
		return data[len(utf8BOM):]
	}
	return data
}

// DecodeCharset 将指定字符集字节解码为 UTF-8。
func DecodeCharset(data []byte, cs Charset) ([]byte, error) {
	return io.ReadAll(NewDecoder(bytes.NewReader(data), cs))
}

// EncodeCharset 将 UTF-8 字节编码为指定字符集。
func EncodeCharset(data []byte, cs Charset) ([]byte, error) {
	if cs == UTF8 {
		return data, nil
	}
	var buf bytes.Buffer
	w := transform.NewWriter(&buf, encFor(cs).NewEncoder())
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}