package expr

import (
	"errors"
	"fmt"
	"reflect"
)

// Expression is an interface over all the different types of expressions
// that we can parse out of lucene
type Expression interface {
	String() string
	Insert(e Expression) (Expression, error)
}

func EQ(a Expression, b Expression) Expression {
	return &Equals{
		Term:  a.(*Literal).Value.(string),
		Value: b,
	}
}

func AND(a, b Expression) Expression {
	return &And{
		Left:  a,
		Right: b,
	}
}

func OR(a, b Expression) Expression {
	return &Or{
		Left:  a,
		Right: b,
	}
}

func Lit(val any) Expression {
	return &Literal{
		Value: val,
	}
}

func Wild(val any) Expression {
	return &WildLiteral{
		Literal: Literal{
			Value: val,
		},
	}
}

func Rang(min, max Expression, inclusive bool) Expression {
	lmin, ok := min.(*Literal)
	if !ok {
		wmin, ok := min.(*WildLiteral)
		if !ok {
			panic("must only pass a *Literal or *WildLiteral to the Rang function")
		}
		lmin = &Literal{Value: wmin.Value}
	}

	lmax, ok := max.(*Literal)
	if !ok {
		wmax, ok := max.(*WildLiteral)
		if !ok {
			panic("must only pass a *Literal or *WildLiteral to the Rang function")
		}
		lmax = &Literal{Value: wmax.Value}
	}
	return &Range{
		Inclusive: inclusive,
		Min:       lmin,
		Max:       lmax,
	}
}

func NOT(e Expression) Expression {
	return &Not{
		Sub: e,
	}
}

func MUST(e Expression) Expression {
	return &Must{
		Sub: e,
	}
}

func MUSTNOT(e Expression) Expression {
	return &MustNot{
		Sub: e,
	}
}

func BOOST(e Expression, power float32) Expression {
	return &Boost{
		Sub:   e,
		Power: power,
	}
}

func FUZZY(e Expression, distance int) Expression {
	return &Fuzzy{
		Sub:      e,
		Distance: distance,
	}
}

func REGEXP(val any) Expression {
	return &RegexpLiteral{
		Literal: Literal{Value: val},
	}
}

// Validate validates the expression is correctly structured.
func Validate(ex Expression) (err error) {
	switch e := ex.(type) {
	case *Equals:
		if e.Term == "" || e.Value == nil {
			return errors.New("EQUALS operator must have both sides of the expression")
		}
		return Validate(e.Value)
	case *And:
		if e.Left == nil || e.Right == nil {
			return errors.New("AND clause must have two sides")
		}
		err = Validate(e.Left)
		if err != nil {
			return err
		}
		err = Validate(e.Right)
		if err != nil {
			return err
		}
	case *Or:
		if e.Left == nil || e.Right == nil {
			return errors.New("OR clause must have two sides")
		}
		err = Validate(e.Left)
		if err != nil {
			return err
		}
		err = Validate(e.Right)
		if err != nil {
			return err
		}
	case *Not:
		if e.Sub == nil {
			return errors.New("NOT expression must have a sub expression to negate")
		}
		return Validate(e.Sub)
	case *Literal:
		// do nothing
	case *WildLiteral:
		// do nothing
	case *RegexpLiteral:
		// do nothing
	case *Range:
		if e.Min == nil || e.Max == nil {
			return errors.New("range clause must have a min and a max")
		}
		err = Validate(e.Min)
		if err != nil {
			return err
		}
		err = Validate(e.Max)
		if err != nil {
			return err
		}
	case *Must:
		if e.Sub == nil {
			return errors.New("MUST expression must have a sub expression")
		}
		_, isMustNot := e.Sub.(*MustNot)
		_, isMust := e.Sub.(*Must)
		if isMust || isMustNot {
			return errors.New("MUST cannot be repeated with itself or MUST NOT")
		}
		return Validate(e.Sub)
	case *MustNot:
		if e.Sub == nil {
			return errors.New("MUST NOT expression must have a sub expression")
		}
		_, isMustNot := e.Sub.(*MustNot)
		_, isMust := e.Sub.(*Must)
		if isMust || isMustNot {
			return errors.New("MUST NOT cannot be repeated with itself or MUST")
		}
		return Validate(e.Sub)
	case *Boost:
		if e.Sub == nil {
			return errors.New("BOOST expression must have a subexpression")
		}
		return Validate(e.Sub)
	case *Fuzzy:
		if e.Sub == nil {
			return errors.New("FUZZY expression must have a subexpression")
		}
		return Validate(e.Sub)
	default:
		return fmt.Errorf("unable to validate Expression type: %s", reflect.TypeOf(e))
	}

	return nil

}