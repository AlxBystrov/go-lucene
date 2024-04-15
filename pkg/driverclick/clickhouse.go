package driverclick

import "github.com/AlxBystrov/go-lucene/pkg/lucene/expr"

// PostgresDriver transforms a parsed lucene expression to a sql filter.
type ClickhouseDriver struct {
	Base
}

// NewPostgresDriver creates a new driver that will output a parsed lucene expression as a SQL filter.
func NewClickhouseDriver() ClickhouseDriver {
	fns := map[expr.Operator]RenderFN{
		expr.Literal: literal,
	}

	for op, sharedFN := range Shared {
		_, found := fns[op]
		if !found {
			fns[op] = sharedFN
		}
	}

	return ClickhouseDriver{
		Base{
			RenderFNs: fns,
		},
	}
}
