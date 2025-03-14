package main

import "strings"

func wrapString(str string, length int) string {
	var b strings.Builder
	var count int

	if length < 1 {
		return str
	}

	for _, c := range str {
		b.WriteRune(c)

		if count == length {
			b.WriteRune('\n')
			count = 0

			continue
		}

		count++
	}

	return b.String()
}
