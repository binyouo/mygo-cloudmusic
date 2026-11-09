package localmedia

import (
	"bytes"
	"golang.org/x/text/encoding/simplifiedchinese"
	"unicode/utf8"
)

// decodeLegacy undoes text the library read in the wrong encoding. ID3 has no
// way to say GBK, so Chinese taggers wrote it where ISO-8859-1 belongs, and the
// library hands it back a rune a byte; ID3v1 comes back as the bytes
// themselves. Some taggers wrote UTF-8 there too. A byte that's ASCII after a
// high one is taken for Latin-1, for "Motörhead" not to turn Chinese: GB2312,
// which nearly every such tag keeps to, has both bytes high.
func decodeLegacy(s string) string {
	var b []byte
	if utf8.ValidString(s) {
		high := false
		for _, r := range s {
			if r > 0xff {
				return s
			}
			high = high || r >= 0x80
		}
		if !high {
			return s
		}
		b = make([]byte, 0, len(s))
		for _, r := range s {
			b = append(b, byte(r))
		}
	} else {
		b = []byte(s)
	}
	if utf8.Valid(b) {
		return string(b)
	}
	if gbkPairs(b) {
		if out, err := simplifiedchinese.GBK.NewDecoder().Bytes(b); err == nil && utf8.Valid(out) && !bytes.ContainsRune(out, utf8.RuneError) {
			return string(out)
		}
	}
	// Latin-1 after all.
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

// gbkPairs tells whether every high byte of b is in a pair of two.
func gbkPairs(b []byte) bool {
	for i := 0; i < len(b); i++ {
		if b[i] < 0x80 {
			continue
		}
		if b[i] == 0x80 || b[i] == 0xff || i+1 == len(b) || b[i+1] < 0x80 || b[i+1] == 0xff {
			return false
		}
		i++
	}
	return true
}
