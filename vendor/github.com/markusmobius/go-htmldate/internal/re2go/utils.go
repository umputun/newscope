package re2go

func getAllSubmatch(input string, ncap int, indexes []int) ([]string, int) {
	submatch := make([]string, ncap)
	for i := 0; i < ncap; i++ {
		submatch[i] = input[indexes[2*i]:indexes[2*i+1]]
	}
	return submatch, indexes[0]
}

func copyIndexes(src []int) []int {
	indexes := make([]int, len(src))
	copy(indexes, src)
	return indexes
}
