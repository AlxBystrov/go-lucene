package lucene

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// Expression ...
type Expression interface {
	// String() string
	// Render() (string, error)
	insert(e Expression) (Expression, error)
}

// Equals ...
type Equals struct {
	term  string
	value Expression

	isMust    bool
	isMustNot bool
}

func (eq Equals) String() string {
	return fmt.Sprintf("%v = %v", eq.term, eq.value)
}

func (eq *Equals) insert(e Expression) (Expression, error) {
	literal, isLiteral := e.(*Literal)
	if eq.term == "" && !isLiteral {
		return nil, errors.New("an equals expression must have a string as a term")
	}

	if eq.term == "" && isLiteral {
		str, ok := literal.val.(string)
		if !ok {
			return nil, fmt.Errorf("unable to insert non string [%v] into equals term", reflect.TypeOf(literal.val))
		}

		eq.term = str
		return eq, nil
	}

	// if we are inserting a term into an equals then we are in the implicit boolean case
	if eq.term != "" && eq.value != nil {
		return &And{left: eq, right: e}, nil
	}

	eq.value = e
	// this is a hack but idk how to do it otherwise. The must and must nots must only
	// apply to the equals directly following them
	if eq.isMust {
		eq.isMust = false
		return &Must{expr: eq}, nil
	}

	if eq.isMustNot {
		eq.isMustNot = false
		return &MustNot{expr: eq}, nil
	}
	return eq, nil
}

// And ...
type And struct {
	left  Expression
	right Expression
}

func (a And) String() string {
	return fmt.Sprintf("(%v) AND (%v)", a.left, a.right)
}

func (a *And) insert(e Expression) (Expression, error) {
	if a.left == nil {
		a.left = e
		return a, nil
	}

	if a.right == nil {
		a.right = e
		return a, nil
	}

	// if we are inserting a term into a full and then we are doing an implicit compound operation
	if a.left != nil && a.right != nil {
		return &And{left: a, right: e}, nil
	}

	return nil, errors.New("attempting to insert an expression into a full AND clause")
}

// Or ...
type Or struct {
	left  Expression
	right Expression
}

func (o Or) String() string {
	return fmt.Sprintf("(%v) OR (%v)", o.left, o.right)
}

func (o *Or) insert(e Expression) (Expression, error) {
	if o.left == nil {
		o.left = e
		return o, nil
	}

	if o.right == nil {
		o.right = e
		return o, nil
	}

	// if we are inserting a term into a full and then we are doing an implicit compound operation
	if o.left != nil && o.right != nil {
		return &And{left: o, right: e}, nil
	}

	return nil, errors.New("attempting to insert an expression into a full OR clause")
}

// Not ...
type Not struct {
	expr Expression
}

func (n Not) String() string {
	return fmt.Sprintf("NOT(%v)", n.expr)
}

func (n *Not) insert(e Expression) (Expression, error) {
	n.expr = e
	return n, nil
}

// Literal ...
type Literal struct {
	val any
}

func (l Literal) String() string {
	return fmt.Sprintf("%v", l.val)
}

func (l *Literal) insert(e Expression) (Expression, error) {
	switch exp := e.(type) {
	case *Equals:
		return exp.insert(l)
	// if we are inserting a term into a literal then we must be doing an implicit compound
	default:
		return &And{left: l, right: e}, nil
		// default:
		// 	return nil, fmt.Errorf("unable to insert [%v] into literal expression", reflect.TypeOf(e)))
	}
}

// WildLiteral indicates the literal has regex values in it and should be matched as a loose wildcard
type WildLiteral struct{ Literal }

// RegexpLiteral indicates the literal has regex values in it and should be matched as a regex
type RegexpLiteral struct{ Literal }

// Range ...
type Range struct {
	Min       Expression
	Max       Expression
	Inclusive bool
}

func (r Range) String() string {
	return fmt.Sprintf("[%s TO %s]", r.Min, r.Max)
}

func (r *Range) insert(e Expression) (Expression, error) {
	if r.Min == nil {
		return nil, errors.New("should not be able to have a TO expression without a minimum")
	}

	// if we are inserting an expression into a full range query we must be trying to do a compound operation
	if r.Min != nil && r.Max != nil {
		return &And{left: r, right: e}, nil
	}

	switch exp := e.(type) {
	case *Literal, *WildLiteral:
		r.Max = exp
		return r, nil
	default:
		return nil, fmt.Errorf("unable to insert [%v] expression as max in a range", reflect.TypeOf(exp))
	}
}

// Must ...
type Must struct {
	expr Expression
}

func (m Must) String() string {
	return fmt.Sprintf("+%v", m.expr)
}

func (m *Must) insert(e Expression) (Expression, error) {
	m.expr = e
	return m, nil
}

// MustNot ...
type MustNot struct {
	expr Expression
}

func (m MustNot) String() string {
	return fmt.Sprintf("-%v", m.expr)
}

func (m *MustNot) insert(e Expression) (Expression, error) {
	m.expr = e
	return m, nil
}

// Boost ...
type Boost struct {
	expr  Expression
	power float32
}

func (b Boost) String() string {
	return fmt.Sprintf("Boost(%s^%v)", b.expr, b.power)
}

func (b *Boost) insert(e Expression) (Expression, error) {
	// if we are inserting a value into a boost then we must be doing a compound operation
	return &And{left: b, right: e}, nil
}

// Fuzzy ...
type Fuzzy struct {
	expr     Expression
	distance int
}

func (b Fuzzy) String() string {
	if b.distance == 1 {
		return fmt.Sprintf("Fuzzy(%s~)", b.expr)
	}
	return fmt.Sprintf("Fuzzy(%s~%v)", b.expr, b.distance)
}

func (b *Fuzzy) insert(e Expression) (Expression, error) {
	// if we are inserting a value into a fuzzy then we must be doing a compound operation
	return &And{left: b, right: e}, nil
}

type parser struct {
	// keep an internal representation of tokens in case we need to backtrack
	tokIdx int
	tokens []token
	lex    *lexer

	hasMust    bool
	hasMustNot bool

	// this tracks how many open subexpressions we are in. It must be 0 at the end of the parse.
	subExpressionCount int
}

func (p *parser) next() (t token) {
	if p.tokIdx < len(p.tokens)-1 {
		p.tokIdx++
		t = p.tokens[p.tokIdx]

		return t
	}

	// if we have parsed all existing tokens get another
	t = p.lex.nextToken()
	p.tokens = append(p.tokens, t)
	p.tokIdx++
	return t

}

func (p *parser) backup() {
	if p.tokIdx < 0 {
		return
	}
	p.tokIdx--
}

func (p *parser) peek() (t token) {
	// if we have parsed all existing tokens get another but don't increment the pointer
	if p.tokIdx == len(p.tokens)-1 {
		t = p.lex.nextToken()
		p.tokens = append(p.tokens, t)
		return t
	}

	// just return what is at the current pointer
	return p.tokens[p.tokIdx]
}

func canAcceptNextToken(curr Expression, token token) bool {
	if curr == nil {
		return true
	}
	switch expr := curr.(type) {
	case *Literal, *WildLiteral, *Range, *RegexpLiteral:
		return true
	case *Equals:
		if expr.value == nil {
			return token.typ == tLITERAL ||
				token.typ == tQUOTED ||
				token.typ == tREGEXP ||
				token.typ == tLCURLY ||
				token.typ == tLSQUARE ||
				token.typ == tLPAREN
		}
		return token.typ == tAND ||
			token.typ == tOR ||
			token.typ == tCARROT ||
			token.typ == tTILDE ||
			token.typ == tRPAREN
	default:
		return token.typ == tAND ||
			token.typ == tOR ||
			token.typ == tRPAREN ||
			token.typ == tCARROT ||
			token.typ == tTILDE
	}
}

func (p *parser) parse() (e Expression, err error) {
	for {
		token := p.next()
		if token.typ == tEOF {
			return e, p.checkExpressionStack()
		}

		if !canAcceptNextToken(e, token) {
			p.backup()
			sub, err := p.parse()
			if err != nil {
				return e, err
			}

			return e.insert(sub)
		}

		switch token.typ {
		case tERR:
			return e, errors.New(token.val)
		// case tEOF:
		// 	if err != nil {
		// 		return e, fmt.Errorf("sub expression not complete: %w", err)
		// 	}
		// 	return e, nil

		// literal value:
		// 		- we parse the literal to a real type rather than a string representation
		// 		  and then transition the expression state based on seeing a literal.
		case tLITERAL:
			expr, err := parseLiteral(token)
			if err != nil {
				return e, fmt.Errorf("unable to parse literal %w", err)
			}
			if e == nil {
				e = expr
				continue // break out of switch and parse next token
			}

			e, err = e.insert(expr)
			if err != nil {
				return e, fmt.Errorf("unable to insert literal into expression: %w", err)
			}

		// quoted value:
		// 		- we make this quoted value a literal string and ignore keywords and whitespace
		case tQUOTED:
			// strip the quotes off because we don't need them
			val := strings.ReplaceAll(token.val, "\"", "")
			literal := &Literal{
				val: val,
			}

			if e == nil {
				e = literal
				continue // breaks out of the switch and parse next token
			}

			e, err = e.insert(literal)
			if err != nil {
				return e, fmt.Errorf("unable to insert quoted string into expression: %w", err)
			}

		// regexp value:
		// 	- we make this regexp value a literal string and ignore everything in it, much like a quote
		case tREGEXP:
			// strip the quotes off because we don't need them
			val := strings.ReplaceAll(token.val, "/", "")
			literal := &RegexpLiteral{
				Literal: Literal{val: val},
			}

			if e == nil {
				e = literal
				continue // breaks out of the switch and parse next token
			}

			e, err = e.insert(literal)
			if err != nil {
				return e, fmt.Errorf("unable to insert quoted string into expression: %w", err)
			}

		// equal operator:
		//		- if we see an equal we enforce that we have literals and transition the
		// 		  the expression state to handle the equal.
		case tEQUAL, tCOLON:
			if e == nil {
				return e, errors.New("invalid syntax: can't start expression with '= or :'")
			}

			// this is a hack but idk how to do it otherwise. The must and must nots must only
			// apply to the equals directly following them
			e, err = e.insert(&Equals{isMust: p.hasMust, isMustNot: p.hasMustNot})
			if err != nil {
				return e, fmt.Errorf("error updating expression with equals token: %w", err)
			}
			p.hasMust = false
			p.hasMustNot = false

		// not operator
		// 		- if we see a not then parse the following expression and wrap it with not
		case tNOT:
			sub, err := p.parse()
			if err != nil {
				return e, err
			}

			not := &Not{
				expr: sub,
			}

			if e == nil {
				e = not
				break
			}
			e.insert(not)
		// boolean operators:
		//		- these just wrap the existing terms
		case tAND:
			and := &And{
				left: e,
			}
			right, err := p.parse()
			if err != nil {
				return e, fmt.Errorf("unable to build AND clause: %w", err)
			}
			and.right = right
			return and, nil
		case tOR:
			or := &Or{
				left: e,
			}
			right, err := p.parse()
			if err != nil {
				return e, fmt.Errorf("unable to build AND clause: %w", err)
			}
			or.right = right
			return or, nil

		// subexpressions
		// 		- if you see a left paren then recursively parse the expression.
		// 		- if you see a right paren we must be done with the current recursion
		case tLPAREN:
			p.updateExpressionStack(token.val)
			sub, err := p.parse()
			if err != nil {
				return e, fmt.Errorf("unable to parse sub-expression: %w", err)
			}
			if e != nil {
				e, err = e.insert(sub)
				if err != nil {
					return e, err
				}
				break
			}

			e = sub
		case tRPAREN:
			p.updateExpressionStack(token.val)
			if p.subExpressionCount < 0 {
				return e, errors.New("unbalanced closing paren")
			}
			return e, nil

		// range operators
		//		- if you see a left square/curly bracket then parse the sub expression that has to be a range
		// 		- then insert it into the existing expression (should only be for the equals expression)
		case tLSQUARE:
			if e == nil {
				return e, errors.New("unable to insert range into empty expression")
			}
			sub, err := p.parse()
			if err != nil {
				return e, fmt.Errorf("unable to parse inclusive range: %w", err)
			}
			// we are inclusive so update that here
			r, ok := sub.(*Range)
			if !ok {
				return e, errors.New("brackets must surround a range query (hint: use the TO operator in the brackets)")
			}
			r.Inclusive = true
			e, err = e.insert(r)
			if err != nil {
				return e, err
			}
		case tLCURLY:
			if e == nil {
				return e, errors.New("unable to insert range into empty expression")
			}
			sub, err := p.parse()
			if err != nil {
				return e, fmt.Errorf("unable to parse inclusive range: %w", err)
			}
			// we are inclusive so update that here
			r, ok := sub.(*Range)
			if !ok {
				return e, errors.New("brackets must surround a range query (hint: use the TO operator in the brackets)")
			}
			r.Inclusive = false
			e, err = e.insert(r)
			if err != nil {
				return e, err
			}
		case tTO:
			switch e.(type) {
			case *Literal, *WildLiteral:
				// do nothing
			default:
				return nil, errors.New("the TO keyword must follow a literal expression")
			}

			r := &Range{
				Min: e,
			}
			e = r
		case tRSQUARE, tRCURLY:
			return e, nil

		// must and must not operators
		// 		- if we see a plus or minus then we need to apply it to the next term only
		case tPLUS:
			p.hasMust = true
		case tMINUS:
			p.hasMustNot = true

		// boost operator
		//     - if we see a carrot we get the boost term and wrap left term in the boost
		case tCARROT:
			next := p.next()

			if next.typ != tLITERAL {
				return e, errors.New("term boost must be follow by positive number")
			}

			f, err := toPositiveFloat(next.val)
			if err != nil {
				return e, fmt.Errorf("not able to parse boost number: %w", err)
			}

			e, err = wrapInBoost(e, f)
			if err != nil {
				return e, fmt.Errorf("unable to wrap expression in boost: %w", err)
			}

		// fuzzy search operator
		//     - if we see a tilde try to fuzzy try to wrap the left term in a fuzzy search with an optional edit distance
		//     - according to https://lucene.apache.org/core/7_3_1/core/org/apache/lucene/search/FuzzyQuery.html#defaultMinSimilarity
		//       the minSimilarity rating is deprecated so this can just be an edit distance.
		case tTILDE:
			next := p.next()

			if next.typ != tLITERAL {
				p.backup()
				e, err = wrapInFuzzy(e, 1)
				if err != nil {
					return e, fmt.Errorf("not able to wrap expression in fuzzy search: %w", err)
				}
				continue
			}

			i, err := toPositiveInt(next.val)
			if err != nil {
				return e, fmt.Errorf("not able to parse fuzzy distance: %w", err)
			}

			e, err = wrapInFuzzy(e, i)
			if err != nil {
				return e, fmt.Errorf("unable to wrap expression in boost: %w", err)
			}
		}

	}
}

func validate(expr Expression) (err error) {
	switch e := expr.(type) {
	case *Equals:
		if e.term == "" || e.value == nil {
			return errors.New("EQUALS operator must have both sides of the expression")
		}
		return validate(e.value)
	case *And:
		if e.left == nil || e.right == nil {
			return errors.New("AND clause must have two sides")
		}
		err = validate(e.left)
		if err != nil {
			return err
		}
		err = validate(e.right)
		if err != nil {
			return err
		}
	case *Or:
		if e.left == nil || e.right == nil {
			return errors.New("OR clause must have two sides")
		}
		err = validate(e.left)
		if err != nil {
			return err
		}
		err = validate(e.right)
		if err != nil {
			return err
		}
	case *Not:
		if e.expr == nil {
			return errors.New("NOT expression must have a sub expression to negate")
		}
		return validate(e.expr)
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
		err = validate(e.Min)
		if err != nil {
			return err
		}
		err = validate(e.Max)
		if err != nil {
			return err
		}
	case *Must:
		if e.expr == nil {
			return errors.New("MUST expression must have a sub expression")
		}
		_, isMustNot := e.expr.(*MustNot)
		_, isMust := e.expr.(*Must)
		if isMust || isMustNot {
			return errors.New("MUST cannot be repeated with itself or MUST NOT")
		}
		return validate(e.expr)
	case *MustNot:
		if e.expr == nil {
			return errors.New("MUST NOT expression must have a sub expression")
		}
		_, isMustNot := e.expr.(*MustNot)
		_, isMust := e.expr.(*Must)
		if isMust || isMustNot {
			return errors.New("MUST NOT cannot be repeated with itself or MUST")
		}
		return validate(e.expr)
	case *Boost:
		if e.expr == nil {
			return errors.New("BOOST expression must have a subexpression")
		}
		return validate(e.expr)
	case *Fuzzy:
		if e.expr == nil {
			return errors.New("FUZZY expression must have a subexpression")
		}
		return validate(e.expr)
	default:
		return fmt.Errorf("unable to validate Expression type: %s", reflect.TypeOf(e))
	}

	return nil

}

func toPositiveInt(in string) (i int, err error) {
	i, err = strconv.Atoi(in)
	if err == nil && i > 0 {
		return i, nil
	}

	return i, fmt.Errorf("[%v] is not a positive number", in)
}

func toPositiveFloat(in string) (f float32, err error) {
	i, err := strconv.Atoi(in)
	if err == nil && i > 0 {
		return float32(i), nil
	}

	pf, err := strconv.ParseFloat(in, 64)
	if err == nil && pf > 0 {
		return float32(pf), nil
	}

	return f, fmt.Errorf("[%v] is not a positive number", in)
}

func (p *parser) parseBoolean(e Expression) (Expression, error) {
	// assume e is expression that will be put into an and clause
	and := &And{
		left: e,
	}

	for {
		token := p.next()
		switch token.typ {
		case tERR:
			return nil, fmt.Errorf(token.val)
		case tEOF:
			return nil, errors.New("unterminitated boolean expression")

		case tLITERAL:
			and.right = &Literal{token.val}
			return and, nil

		default:
			return nil, errors.New("unable to insert a sub expression in a boolean")
		}
	}
}

func (p *parser) updateExpressionStack(s string) {
	if s == "(" {
		p.subExpressionCount++
		return
	}

	p.subExpressionCount--
	return
}

func (p *parser) checkExpressionStack() error {
	if p.subExpressionCount != 0 {
		return fmt.Errorf("unterminated paren")
	}

	return nil
}

func parseLiteral(token token) (e Expression, err error) {
	val := token.val
	ival, err := strconv.Atoi(val)
	if err == nil {
		return &Literal{val: ival}, nil
	}

	if strings.ContainsAny(val, "*?") {
		return &WildLiteral{Literal{val: val}}, nil
	}

	return &Literal{val: val}, nil

}

func wrapInBoost(e Expression, power float32) (Expression, error) {
	if e == nil {
		return e, errors.New("carrot must follow another expression")
	}

	e = &Boost{
		expr:  e,
		power: power,
	}
	return e, nil
}

func wrapInFuzzy(e Expression, distance int) (Expression, error) {
	if e == nil {
		return e, errors.New("carrot must follow another expression")
	}

	e = &Fuzzy{
		expr:     e,
		distance: distance,
	}
	return e, nil
}

// Parse will parse the lucene grammar out of a string
func Parse(input string) (e Expression, err error) {
	p := parser{
		lex:    lex(input),
		tokIdx: -1,
	}
	e, err = p.parse()
	if err != nil {
		return e, err
	}

	err = validate(e)
	if err != nil {
		return e, err
	}

	return e, nil
}
