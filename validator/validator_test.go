package validator

import "testing"

func Test_Validator(t *testing.T) {
	err := ValidateEndpoint()

	if err != nil {
		t.Fatal(err)
	}
}
