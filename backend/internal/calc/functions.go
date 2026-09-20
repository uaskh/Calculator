package calc

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
)

// functionFunc evaluates a one-argument function.
type functionFunc func(n *numbers, argument decimal.Decimal) (decimal.Decimal, error)

// function is one entry of the function table. Adding a function is adding an entry.
type function struct {
	name     string // ASCII letters, case-sensitive
	evaluate functionFunc
}

// functionTable maps a name to its function.
type functionTable map[string]*function

// register adds a function to the table. Registration happens at construction time, so
// an invalid or duplicate entry is a programming error and panics.
func (t functionTable) register(fn *function) {
	if err := validateFunction(t, fn); err != nil {
		panic(fmt.Sprintf("calc: register function %q: %v", fn.name, err))
	}
	t[fn.name] = fn
}

func validateFunction(t functionTable, fn *function) error {
	if fn.name == "" {
		return errors.New("empty name")
	}
	for _, r := range fn.name {
		if !isLetter(r) {
			return errors.New("name must be ASCII letters only")
		}
	}
	if _, exists := t[fn.name]; exists {
		return errors.New("already registered")
	}
	if fn.evaluate == nil {
		return errors.New("function without an evaluate function")
	}
	return nil
}

// defaultFunctions is the function table of the specification.
func defaultFunctions() functionTable {
	t := functionTable{}
	t.register(&function{name: "sqrt", evaluate: (*numbers).sqrt})
	return t
}
