package helper

// DeferedErrorIgnore executes the provided function and ignores any error it
// returns. This is useful for deferred cleanup functions where the error is not
// critical and should not be handled.
func DeferedErrorIgnore(fn func() error) { _ = fn() }
