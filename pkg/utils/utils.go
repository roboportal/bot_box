package utils

func TriggerChannel(c chan struct{}) {
	c <- struct{}{}
}
