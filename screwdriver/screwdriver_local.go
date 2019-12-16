package screwdriver

// localApi is a Dummy Screwdriver API endpoint for local mode.
type localApi struct{}

func NewLocal() (API, error) {
	return API(localApi{}), nil
}

// Don't update step lines in local-mode
func (a localApi) UpdateStepLines(stepName string, lineCount int) error {
	return nil
}
