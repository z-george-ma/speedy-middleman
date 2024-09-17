package lib

func BinarySearch[T ~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~float32 | ~float64 | ~string](
	a []T,
	x T,
) (pos int, found bool) {
	start, pos, end := 0, 0, len(a)-1
	for start <= end {
		pos = (start + end) >> 1
		switch {
		case a[pos] > x:
			end = pos - 1
		case a[pos] < x:
			start = pos + 1
		default:
			found = true
			return
		}
	}
	return end, found
}
