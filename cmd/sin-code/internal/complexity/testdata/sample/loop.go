package sample

func repeated(n int) []int {
	var s []int
	for i := 0; i < n; i++ {
		s = append(s, 0)
	}
	return s
}
