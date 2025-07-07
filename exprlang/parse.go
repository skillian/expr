package exprlang

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math/big"
	"unicode"
	"unicode/utf8"

	"github.com/skillian/expr"
	"github.com/skillian/logging"
)

const (
	// PkgName gets this package's name as a string constant
	PkgName = "github.com/skillian/expr/exprlang"

	arbitraryCapacity = 8
)

var (
	_ = logging.GetLogger(
		PkgName,
		logging.LoggerLevel(logging.EverythingLevel),
		logging.LoggerPropagate(true),
	)
)

// ParseExpr parses from a reader into an expression tree
func ParseExpr(r io.Reader, vs expr.Values) (e expr.Expr, err error) {
	ep := exprParser{
		stack: make([]expr.Expr, 0, arbitraryCapacity),
		vs:    vs,
	}
	if ep.vs == nil {
		ep.vs = expr.NoValues()
	}
	if rs, ok := r.(io.RuneScanner); ok {
		ep.scanner = rs
	} else {
		ep.scanner = bufio.NewReader(r)
	}
	if err := ep.tokenize(); err != nil && err != io.EOF {
		return nil, err
	}
	return ep.parse()
}

type exprParser struct {
	scanner io.RuneScanner
	buf     []rune
	rat     big.Rat
	stack   []expr.Expr
	vs      expr.Values
	locs    [2]Loc
}

type openParen struct{}

var _ expr.Expr = openParen{}

type closeParen struct{}

var _ expr.Expr = closeParen{}

func (ep *exprParser) parse() (expr.Expr, error) {
	return ep.parseOr()
	// TODO: function call
}

func (ep *exprParser) parseOr() (e expr.Expr, err error) {
	e, err = ep.parseAnd()
	if err != nil {
		return nil, fmt.Errorf("parse or: %w", err)
	}
	e2, err := ep.peekFront()
	if err != nil {
		if err == io.EOF {
			err = nil
			return
		}
		return nil, fmt.Errorf("peeking or: %w", err)
	}
	switch e2.(type) {
	case expr.Or:
		ep.removeFrontUnsafe()
		e2, err = ep.parseAnd()
		if err != nil {
			return nil, fmt.Errorf("or operand: %w", err)
		}
		return expr.Or{e, e2}, nil
	}
	return e, nil
}

func (ep *exprParser) parseAnd() (e expr.Expr, err error) {
	e, err = ep.parseTerm()
	if err != nil {
		return nil, fmt.Errorf("parse and: %w", err)
	}
	e2, err := ep.peekFront()
	if err != nil {
		if err == io.EOF {
			err = nil
			return
		}
		return nil, fmt.Errorf("peeking and: %w", err)
	}
	switch e2.(type) {
	case expr.And:
		ep.removeFrontUnsafe()
		e2, err = ep.parseTerm()
		if err != nil {
			return nil, fmt.Errorf("and operand: %w", err)
		}
		return expr.And{e, e2}, nil
	}
	return e, nil
}

func (ep *exprParser) parseTerm() (e expr.Expr, err error) {
	e, err = ep.parseFactor()
	if err != nil {
		return nil, fmt.Errorf("parse term: %w", err)
	}
	e2, err := ep.peekFront()
	if err != nil {
		if err == io.EOF {
			err = nil
			return
		}
		return nil, fmt.Errorf("peeking term: %w", err)
	}
	switch op := e2.(type) {
	case expr.Add, expr.Sub:
		ep.removeFrontUnsafe()
		e2, err = ep.parseFactor()
		if err != nil {
			return nil, fmt.Errorf("term operand: %w", err)
		}
		switch op.(type) {
		case expr.Add:
			return expr.Add{e, e2}, nil
		}
		return expr.Sub{e, e2}, nil
	}
	return e, nil
}

func (ep *exprParser) parseFactor() (e expr.Expr, err error) {
	e, err = ep.parseMember()
	if err != nil {
		return nil, fmt.Errorf("parse factor: %w", err)
	}
	e2, err := ep.peekFront()
	if err != nil {
		if err == io.EOF {
			err = nil
			return
		}
		return nil, fmt.Errorf("peeking factor: %w", err)
	}
	switch op := e2.(type) {
	case expr.Mul, expr.Div:
		ep.removeFrontUnsafe()
		e2, err = ep.parseMember()
		if err != nil {
			return nil, fmt.Errorf("factor operand: %w", err)
		}
		switch op.(type) {
		case expr.Mul:
			return expr.Mul{e, e2}, nil
		}
		return expr.Div{e, e2}, nil
	}
	return e, nil
}

func (ep *exprParser) parseMember() (e expr.Expr, err error) {
	e, err = ep.parseTerminal()
	if err != nil {
		return nil, fmt.Errorf("parse member terminal: %w", err)
	}
	e2, err := ep.peekFront()
	if err != nil {
		if err == io.EOF {
			err = nil
			return
		}
		return nil, fmt.Errorf("peeking member: %w", err)
	}
	if _, ok := e2.(expr.Mem); ok {
		ep.removeFrontUnsafe()
		e2, err = ep.parseTerminal()
		if err != nil {
			return nil, fmt.Errorf("member operand: %w", err)
		}
		return expr.Mem{e, e2}, nil
	}
	return e, nil
}

func (ep *exprParser) parseTerminal() (e expr.Expr, err error) {
	e, err = ep.popFront()
	if err != nil {
		return nil, fmt.Errorf("parse terminal: %w", err)
	}
	if _, ok := e.(openParen); ok {
		e, err = ep.parse()
		if err != nil {
			return nil, fmt.Errorf("parse paren expr: %w", err)
		}
		e2, err := ep.peekFront()
		if err != nil {
			return nil, fmt.Errorf("parsing closing paren: %w", err)
		}
		if _, ok := e2.(closeParen); !ok {
			return nil, fmt.Errorf("missing closing paren")
		}
		ep.removeFrontUnsafe()
		return e, err
	}
	if expr.HasOperands(e) {
		return nil, fmt.Errorf("non-terminal: %[1]v (type: %[1]T)", e)
	}
	return
}

func (ep *exprParser) peekFront() (expr.Expr, error) {
	if len(ep.stack) > 0 {
		return ep.stack[0], nil
	}
	return nil, io.EOF
}

func (ep *exprParser) popFront() (e expr.Expr, err error) {
	if e, err = ep.peekFront(); err == nil {
		ep.removeFrontUnsafe()
	}
	return
}

func (ep *exprParser) removeFrontUnsafe() {
	ep.stack[0] = nil // "free" memory
	ep.stack = ep.stack[1:]
}

func (ep *exprParser) consume(e expr.Expr) {
	ep.stack = append(ep.stack, e)
	ep.buf = ep.buf[:0]
}

func (ep *exprParser) tokenize() error {
	for {
		r, err := ep.readRune()
		if err != nil {
			return err
		}
		switch r {
		case '(':
			ep.consume(openParen{})
		case ')':
			ep.consume(closeParen{})
		case '.':
			ep.consume(expr.Mem{})
		case '+':
			ep.consume(expr.Add{})
		case '-':
			ep.consume(expr.Sub{})
		case '*':
			ep.consume(expr.Mul{})
		case '/':
			ep.consume(expr.Div{})
		default:
			switch {
			case unicode.IsSpace(r):
				ep.buf = ep.buf[:len(ep.buf)-1]
				continue
			case r == '"':
				err = ep.parseString()
			case '0' <= r && r <= '9':
				err = ep.parseNumber()
			case r == '_' || unicode.IsLetter(r):
				err = ep.tokenizeIdent()
			default:
				return fmt.Errorf(
					"unable to tokenize at %v",
					ep.locs[(len(ep.buf)-1)&1],
				)
			}
		}
		if err != nil {
			return err
		}
	}
}

func (ep *exprParser) parseString() error {
	escaping := false
forLoop:
	for {
		r, err := ep.readRune()
		if err != nil {
			return fmt.Errorf("parsing string: %w", err)
		}
		switch r {
		case '"':
			if !escaping {
				break forLoop
			}
		case '\\':
			escaping = !escaping
			ep.buf = ep.buf[:len(ep.buf)-1]
		}
	}
	ep.consume(string(ep.buf))
	return nil
}

const (
	bigRatBase2 = iota
	bigRatBase8
	bigRatBase10
	bigRatBase16
	bigRatBaseCount
)

var bigRatBases = func() (bigRatBases [bigRatBaseCount]big.Rat) {
	bigRatBases[bigRatBase2].SetInt64(2)
	bigRatBases[bigRatBase8].SetInt64(8)
	bigRatBases[bigRatBase10].SetInt64(10)
	bigRatBases[bigRatBase16].SetInt64(16)
	return
}()

func (ep *exprParser) parseNumber() (parseNumberErr error) {
	ep.rat.SetInt64(int64(ep.buf[len(ep.buf)-1] - rune('0')))
	var (
		i            big.Int
		afterDecimal bool
		max          rune
	)
	base := &bigRatBases[bigRatBase10]
	if ep.buf[len(ep.buf)-1] == '0' {
		r, err := ep.readRune()
		if err == io.EOF {
			parseNumberErr = err
			goto consume
		}
		if err != nil {
			return fmt.Errorf("parsing number: %w", err)
		}
		switch r {
		case 'b':
			base = &bigRatBases[bigRatBase2]
		case 'o':
			base = &bigRatBases[bigRatBase8]
		case 'x':
			base = &bigRatBases[bigRatBase16]
		default:
			if !('0' <= r && r <= '9') {
				return fmt.Errorf(
					"%q not expected while parsing number",
					string(r),
				)
			}
			n := ep.rat.Num()
			n.Mul(n, base.Num()).Add(n, i.SetInt64(int64(r-'0')))
		}
	}
	max = rune('0' + base.Num().Int64())
forLoop:
	for {
		r, err := ep.readRune()
		if err == io.EOF {
			parseNumberErr = err
			break forLoop
		}
		if err != nil {
			return fmt.Errorf("parsing number: %w", err)
		}
		switch r {
		case '.':
			if afterDecimal {
				if err := ep.unreadRune(); err != nil {
					return fmt.Errorf(
						"unreading rune %q: %w",
						string(r), err,
					)
				}
				break forLoop
			}
			afterDecimal = true
		case 'e', 'E':
			return fmt.Errorf(
				"'E' notation not yet supported",
			)
		default:
			if '0' <= r && r < max {
				n := ep.rat.Num()
				n.Mul(n, base.Num()).Add(n, i.SetInt64(int64(r-'0')))
				if afterDecimal {
					d := ep.rat.Denom()
					d.Mul(d, base.Num())
				}
			} else {
				if err := ep.unreadRune(); err != nil {
					return fmt.Errorf(
						"unreading rune %q: %w",
						string(r), err,
					)
				}
				break forLoop
			}
		}
	}
consume:
	if ep.rat.IsInt() {
		num := ep.rat.Num()
		if num.IsInt64() {
			i64 := num.Int64()
			i := int(i64)
			if int64(i) == i64 {
				ep.consume(i)
				return parseNumberErr
			}
			ep.consume(i64)
			return parseNumberErr
		}
	}
	ep.consume((&big.Rat{}).Set(&ep.rat))
	return parseNumberErr
}

var alphanumranges = []*unicode.RangeTable{
	unicode.Letter,
	unicode.Digit,
}

func (ep *exprParser) tokenizeIdent() error {
	for {
		r, err := ep.readRune()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("tokenize identifier: %w", err)
		}
		if r != '_' && !unicode.In(r, alphanumranges...) {
			if err := ep.unreadRune(); err != nil {
				return fmt.Errorf(
					"unreading %s from "+
						"identifier: %w",
					string(r), err,
				)
			}
			break
		}
	}
	ep.consume(expr.Ident(string(ep.buf)))
	return nil
}

func (ep *exprParser) readRune() (rune, error) {
	r, n, err := ep.scanner.ReadRune()
	if err == io.EOF && n > 0 {
		// choosing not to handle this at this time.
		err = nil
	}
	if err != nil {
		return utf8.RuneError, err
	}
	ep.buf = append(ep.buf, r)
	lenMod2 := len(ep.buf) % 2
	ep.locs[lenMod2] = ep.locs[(lenMod2+1)%2].Append(r)
	return r, nil
}

func (ep *exprParser) unreadRune() (err error) {
	if err = ep.scanner.UnreadRune(); err != nil {
		return
	}
	ep.buf = ep.buf[:len(ep.buf)-1]
	return
}

type Loc struct {
	Line int32
	Col  int32
}

func (loc Loc) Append(r rune) Loc {
	switch r {
	case '\n':
		return Loc{loc.Line + 1, func(loc Loc) int32 {
			if loc.Col == 0 {
				return 0
			}
			return 1
		}(loc)}
	}
	return Loc{loc.Line, loc.Col + 1}
}

func (loc Loc) String() string {
	const (
		lineOnlyFormat = "line:%d"
		format         = lineOnlyFormat + ":%d"
	)
	if loc.Col == 0 {
		return fmt.Sprintf(lineOnlyFormat, loc.Line+1)
	}
	return fmt.Sprintf(format, loc.Line+1, loc.Col)
}

type namedVar struct{ name string }

var _ expr.NamedVar = (*namedVar)(nil)

func (v *namedVar) Name() string  { return v.name }
func (v *namedVar) Var() expr.Var { return v }
