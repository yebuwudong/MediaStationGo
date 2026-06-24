package service

func firstIndexFunc(values []string, match func(string) bool) int {
	for i, value := range values {
		if match(value) {
			return i
		}
	}
	return -1
}

func firstQuery(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func lastIndexFunc(values []string, match func(string) bool) int {
	for i := len(values) - 1; i >= 0; i-- {
		if match(values[i]) {
			return i
		}
	}
	return -1
}
