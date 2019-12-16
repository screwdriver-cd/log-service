package screwdriver

type localApi struct{}

func NewLocal() (API, error) {
	return API(localApi{}), nil
}

func (a localApi) UpdateStepLines(stepName string, lineCount int) error {
	return nil
}
