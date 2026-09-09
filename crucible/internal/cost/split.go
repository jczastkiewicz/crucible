// Ported from forge-core/src/main/java/forge/util/TextUtil.java
// (splitWithParenthesis).

package cost

// split reproduces TextUtil.splitWithParenthesis, which is not
// strings.Split with extra steps:
//
//   - A delimiter inside the open/close pair does not split, so a `<...>` body
//     stays whole however many spaces it holds.
//   - Empty segments are dropped, so two delimiters in a row collapse into one
//     and a field written empty shifts every field after it.
//   - maxEntries caps the number of splits, not the number of results: once
//     the cap is reached the rest of the input is one final segment.
//
// open and closing of 0 mean no nesting, which is how Java asks for a plain
// split.
func split(input string, delimiter byte, maxEntries int, open, closing byte) []string {
	var (
		out   []string
		depth int
		start int
		idx   = 1
	)
	for i := 0; i < len(input); i++ {
		c := input[i]
		switch {
		case closing > 0 && c == closing && depth > 0:
			depth--
		case open > 0 && c == open:
			depth++
		}

		if c == delimiter && depth == 0 && idx < maxEntries {
			if i > start {
				out = append(out, input[start:i])
				idx++
			}
			start = i + 1
		}
	}
	if len(input) > start {
		out = append(out, input[start:])
	}
	return out
}
