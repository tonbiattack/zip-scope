package archive

import (
	"strings"
	"unicode"
)

// NaturalCompare orders strings the way a file manager does: runs of digits are
// compared by numeric value, everything else by rune. It is case-insensitive
// for the primary comparison so that "a.jpg" and "A.jpg" stay adjacent, and it
// treats both '/' and '\\' as path separators so that directory components are
// compared before their contents.
//
// The comparison is total: it never reports equality for distinct strings,
// because a final case-sensitive tie-break is applied.
func NaturalCompare(a, b string) int {
	if c := naturalFold(a, b); c != 0 {
		return c
	}
	// Stable, deterministic tie-break for names that differ only by case.
	return strings.Compare(a, b)
}

func naturalFold(a, b string) int {
	ar, br := []rune(a), []rune(b)
	i, j := 0, 0
	for i < len(ar) && j < len(br) {
		ca, cb := ar[i], br[j]
		if isDigit(ca) && isDigit(cb) {
			// Compare the full digit runs numerically, ignoring leading zeros.
			si, sj := i, j
			for i < len(ar) && isDigit(ar[i]) {
				i++
			}
			for j < len(br) && isDigit(br[j]) {
				j++
			}
			da := trimZeros(ar[si:i])
			db := trimZeros(br[sj:j])
			if len(da) != len(db) {
				if len(da) < len(db) {
					return -1
				}
				return 1
			}
			for k := range da {
				if da[k] != db[k] {
					if da[k] < db[k] {
						return -1
					}
					return 1
				}
			}
			// Same numeric value: fewer leading zeros sorts first.
			if la, lb := i-si, j-sj; la != lb {
				if la < lb {
					return -1
				}
				return 1
			}
			continue
		}
		// Separators sort before any other character at the same position so
		// that "a/b" precedes "a-b" consistently.
		ra, rb := foldRune(ca), foldRune(cb)
		if ra != rb {
			if ra < rb {
				return -1
			}
			return 1
		}
		i++
		j++
	}
	switch {
	case i < len(ar):
		return 1
	case j < len(br):
		return -1
	}
	return 0
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func trimZeros(r []rune) []rune {
	k := 0
	for k < len(r)-1 && r[k] == '0' {
		k++
	}
	return r[k:]
}

func foldRune(r rune) rune {
	if r == '\\' {
		r = '/'
	}
	return unicode.ToLower(r)
}
