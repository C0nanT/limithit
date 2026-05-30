package attacks

import (
	"fmt"
	"strconv"
)

func ValidatePosInt(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("must be an integer")
	}
	if n <= 0 {
		return fmt.Errorf("must be > 0")
	}
	return nil
}

func ValidateNonNegInt(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("must be an integer")
	}
	if n < 0 {
		return fmt.Errorf("must be >= 0")
	}
	return nil
}

func ValidatePosFloat(s string) error {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("must be a number")
	}
	if f <= 0 {
		return fmt.Errorf("must be > 0")
	}
	return nil
}
