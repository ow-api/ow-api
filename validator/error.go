package validator

func NewSelectorError(message, parent, selector string) *parentSelectorError {
	return &parentSelectorError{message, parent, selector}
}

// errorString is a trivial implementation of error.
type parentSelectorError struct {
	message, parent, selector string
}

func (e *parentSelectorError) Error() string {
	str := e.message + ": "

	if e.parent != "" {
		str += e.parent + ","
	}

	str += e.selector

	return str
}
