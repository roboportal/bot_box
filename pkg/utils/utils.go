package utils

func TriggerChannel(c chan struct{}) {
	c <- struct{}{}
}

func NicelyClose(ch chan struct{}) {
	select {
	case <-ch:
		return
	default:
	}
	close(ch)
}