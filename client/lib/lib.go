package lib

func Drain[T any](c chan T, value T) {
	for {
		select {
		case c <- value:
		default:
			return
		}
	}
}
