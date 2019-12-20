package screwdriver

import "testing"

func TestUpdateStepLinesLocal(t *testing.T) {
	testAPI := localApi{}

	actual := testAPI.UpdateStepLines("test", 0)
	if actual != nil {
		t.Errorf(
			"There are something wrong with localApi.UpdateStepLines\nexpected: %v \nactual: %v",
			nil,
			actual,
		)
	}
}
