package queue

// CutDrawAfter makes the next draws stop with err after a batch that reached `at` (tests).
func CutDrawAfter(at int, err error) func() {
	afterDrawBatch = func(drawn int) error {
		if drawn >= at {
			return err
		}
		return nil
	}
	return func() { afterDrawBatch = nil }
}
