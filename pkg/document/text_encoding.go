package document

import (
	"bytes"
	"errors"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

func DecodeTextFileToUTF8(content []byte) (string, error) {
	if len(content) == 0 {
		return "", nil
	}

	return decodeTextBytesToUTF8(content)
}

func decodeTextBytesToUTF8(content []byte) (string, error) {
	c := content
	if bytes.HasPrefix(c, utf8BOM) {
		c = c[len(utf8BOM):]
	}

	if utf8.Valid(c) {
		return string(c), nil
	}

	if len(c) >= 2 {
		switch {
		case c[0] == 0xFF && c[1] == 0xFE:
			s, err := utf16ToUTF8(c[2:], false)
			if err == nil {
				return s, nil
			}
		case c[0] == 0xFE && c[1] == 0xFF:
			s, err := utf16ToUTF8(c[2:], true)
			if err == nil {
				return s, nil
			}
		}
	}

	if looksLikeUTF16String(c) {
		if s, err := utf16ToUTF8(c, false); err == nil {
			return s, nil
		}

		if s, err := utf16ToUTF8(c, true); err == nil {
			return s, nil
		}
	}

	out, err := charmap.Windows1252.NewDecoder().Bytes(c)
	if err != nil {
		return "", ErrInvalidTextEncoding
	}

	if !utf8.Valid(out) {
		return "", ErrInvalidTextEncoding
	}

	return string(out), nil
}

func utf16ToUTF8(b []byte, bigEndian bool) (string, error) {
	order := unicode.LittleEndian
	if bigEndian {
		order = unicode.BigEndian
	}

	dec := unicode.UTF16(order, unicode.IgnoreBOM).NewDecoder()
	out, err := dec.Bytes(b)
	if err != nil {
		return "", err
	}

	if !utf8.Valid(out) {
		return "", errors.New("utf16 decode not utf8")
	}

	return string(out), nil
}

func looksLikeUTF16String(b []byte) bool {
	if len(b) < 4 || len(b)%2 != 0 {
		return false
	}

	n := 0
	for i := range b {
		if b[i] == 0 {
			n++
		}
	}

	return float64(n)/float64(len(b)) >= 0.12
}
