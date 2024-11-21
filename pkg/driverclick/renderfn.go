package driverclick

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/AlxBystrov/go-lucene/pkg/lucene/expr"
)

// RenderFN is a rendering function. It takes the left and right side of the operator serialized to a string
// and serializes the entire expression
type RenderFN func(left, right string) (string, error)

func literal(left, right string) (string, error) {
	if !utf8.ValidString(left) {
		return "", fmt.Errorf("literal contains invalid utf8: %q", left)
	}
	if strings.ContainsRune(left, 0) {
		return "", fmt.Errorf("literal contains null byte: %q", left)
	}
	return left, nil
}

func equals(left, right string) (string, error) {

	if left == "'_source'" {
		return fmt.Sprintf("match(lowerUTF8(_source), lowerUTF8(%s))", right), nil
	} else if _, err := strconv.ParseInt(right, 0, 64); err == nil {
		left = "numbers.value[indexOf(numbers.name," + left + ")]"
	} else if _, err := strconv.ParseBool(right); err == nil {
		left = "bools.value[indexOf(bools.name," + left + ")]"
	} else {
		left = "lowerUTF8(strings.value[indexOf(strings.name," + left + ")])"
		return fmt.Sprintf("%s = lowerUTF8(%s)", left, right), nil
	}

	return fmt.Sprintf("%s = %s", left, right), nil
}

func noop(left, right string) (string, error) {
	return left, nil
}

func like(left, right string) (string, error) {
	if len(right) >= 4 && right[1] == '/' && right[len(right)-2] == '/' {
		right = strings.Replace(right, "'/", "'", 1)
		right = strings.Replace(right, "/'", "'", 1)
		return fmt.Sprintf("match(lowerUTF8(strings.value[indexOf(strings.name,%s)]),lowerUTF8(%s))", left, right), nil
	}

	right = strings.ReplaceAll(right, "*", "%")
	right = strings.ReplaceAll(right, "?", "_")
	return fmt.Sprintf("lowerUTF8(strings.value[indexOf(strings.name,%s)]) like lowerUTF8(%s)", left, right), nil
}

func inFn(left, right string) (string, error) {
	if _, err := strconv.ParseInt(right, 0, 64); err == nil {
		left = "numbers.value[indexOf(numbers.name," + left + ")]"
	} else if _, err := strconv.ParseBool(right); err == nil {
		left = "bools.value[indexOf(bools.name," + left + ")]"
	} else {
		left = "strings.value[indexOf(strings.name," + left + ")]"
	}
	return fmt.Sprintf("%s IN %s", left, right), nil
}

func list(left, right string) (string, error) {
	return fmt.Sprintf("(%s)", left), nil
}

func greater(left, right string) (string, error) {
	if _, err := strconv.ParseInt(right, 0, 64); err == nil {
		left = "numbers.value[indexOf(numbers.name," + left + ")]"
	} else {
		return "", nil
	}
	return fmt.Sprintf("%s > %s", left, right), nil
}

func less(left, right string) (string, error) {
	if _, err := strconv.ParseInt(right, 0, 64); err == nil {
		left = "numbers.value[indexOf(numbers.name," + left + ")]"
	} else {
		return "", nil
	}
	return fmt.Sprintf("%s < %s", left, right), nil
}

func greaterEq(left, right string) (string, error) {
	if _, err := strconv.ParseInt(right, 0, 64); err == nil {
		left = "numbers.value[indexOf(numbers.name," + left + ")]"
	} else {
		return "", nil
	}
	return fmt.Sprintf("%s >= %s", left, right), nil
}

func lessEq(left, right string) (string, error) {
	if _, err := strconv.ParseInt(right, 0, 64); err == nil {
		left = "numbers.value[indexOf(numbers.name," + left + ")]"
	} else {
		return "", nil
	}
	return fmt.Sprintf("%s <= %s", left, right), nil
}

// rang is more complicated than the others because it has to handle inclusive and exclusive ranges,
// number and string ranges, and ranges that only have one bound
func rang(left, right string) (string, error) {
	inclusive := true
	if right[0] == '(' && right[len(right)-1] == ')' {
		inclusive = false
	}

	stripped := right[1 : len(right)-1]
	rangeSlice := strings.Split(stripped, ",")

	if len(rangeSlice) != 2 {
		return "", fmt.Errorf("the BETWEEN operator needs a two item list in the right hand side, have %s", right)
	}

	rawMin := strings.Trim(rangeSlice[0], " ")
	rawMax := strings.Trim(rangeSlice[1], " ")

	iMin, iMax, err := toInts(rawMin, rawMax)
	if err == nil {
		if rawMin == "'*'" {
			if inclusive {
				return fmt.Sprintf("numbers.value[indexOf(numbers.name,%s)] <= %d", left, iMax), nil
			}
			return fmt.Sprintf("numbers.value[indexOf(numbers.name,%s)] < %d", left, iMax), nil
		}

		if rawMax == "'*'" {
			if inclusive {
				return fmt.Sprintf("numbers.value[indexOf(numbers.name,%s)] >= %d", left, iMin), nil
			}
			return fmt.Sprintf("numbers.value[indexOf(numbers.name,%s)] > %d", left, iMin), nil
		}

		if inclusive {
			return fmt.Sprintf("numbers.value[indexOf(numbers.name,%s)] >= %d AND numbers.value[indexOf(numbers.name,%s)] <= %d",
					left,
					iMin,
					left,
					iMax,
				),
				nil
		}

		return fmt.Sprintf("numbers.value[indexOf(numbers.name,%s)] > %d AND numbers.value[indexOf(numbers.name,%s)] < %d",
				left,
				iMin,
				left,
				iMax,
			),
			nil
	}

	fMin, fMax, err := toFloats(rawMin, rawMax)
	if err == nil {
		if rawMin == "'*'" {
			if inclusive {
				return fmt.Sprintf("numbers.value[indexOf(numbers.name,%s)] <= %.2f", left, fMax), nil
			}
			return fmt.Sprintf("numbers.value[indexOf(numbers.name,%s)] < %.2f", left, fMax), nil
		}

		if rawMax == "'*'" {
			if inclusive {
				return fmt.Sprintf("numbers.value[indexOf(numbers.name,%s)] >= %.2f", left, fMin), nil
			}
			return fmt.Sprintf("numbers.value[indexOf(numbers.name,%s)] > %.2f", left, fMin), nil
		}

		if inclusive {
			return fmt.Sprintf("numbers.value[indexOf(numbers.name,%s)] >= %.2f AND numbers.value[indexOf(numbers.name,%s)] <= %.2f",
					left,
					fMin,
					left,
					fMax,
				),
				nil
		}

		return fmt.Sprintf("numbers.value[indexOf(numbers.name,%s)] > %.2f AND numbers.value[indexOf(numbers.name,%s)] < %.2f",
				left,
				fMin,
				left,
				fMax,
			),
			nil
	}

	return fmt.Sprintf(`strings.value[indexOf(strings.name,%s)] BETWEEN %s AND %s`,
			left,
			strings.Trim(rangeSlice[0], " "),
			strings.Trim(rangeSlice[1], " "),
		),
		nil
}

func basicCompound(op expr.Operator) RenderFN {
	return func(left, right string) (string, error) {
		return fmt.Sprintf("%s %s %s", left, op, right), nil
	}
}

func basicWrap(op expr.Operator) RenderFN {
	return func(left, right string) (string, error) {
		return fmt.Sprintf("%s(%s)", op, left), nil
	}
}

func toInts(rawMin, rawMax string) (iMin, iMax int, err error) {
	iMin, err = strconv.Atoi(rawMin)
	if rawMin != "'*'" && err != nil {
		return 0, 0, err
	}

	iMax, err = strconv.Atoi(rawMax)
	if rawMax != "'*'" && err != nil {
		return 0, 0, err
	}

	return iMin, iMax, nil
}

func toFloats(rawMin, rawMax string) (fMin, fMax float64, err error) {
	fMin, err = strconv.ParseFloat(rawMin, 64)
	if rawMin != "'*'" && err != nil {
		return 0, 0, err
	}

	fMax, err = strconv.ParseFloat(rawMax, 64)
	if rawMax != "'*'" && err != nil {
		return 0, 0, err
	}

	return fMin, fMax, nil
}
