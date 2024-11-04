package sliceutil

// FindValueByIndex returns the value of the index in s,
// or -1 if not present.
func FindValueByIndex[S ~[]E, E any](s S, idx int) (v E) {
	for i := range s {
		if i == idx {
			return s[i]
		}
	}
	return v
}
