package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/aksh/calculator/backend/internal/calc"
)

// Evaluator is what the HTTP layer needs from the calculator domain.
type Evaluator interface {
	Evaluate(ctx context.Context, expression string) (calc.Result, error)
}

// evaluateHandler serves POST /api/v1/evaluate.
type evaluateHandler struct {
	evaluator    Evaluator
	maxBodyBytes int64
	// arithmeticDetails is the single table mapping 422 codes to the user-facing sentence
	// of spec §7. A new arithmetic code is one entry here and one row in the tests.
	arithmeticDetails map[string]string
}

func newEvaluateHandler(evaluator Evaluator, maxBodyBytes int64) evaluateHandler {
	return evaluateHandler{
		evaluator:    evaluator,
		maxBodyBytes: maxBodyBytes,
		arithmeticDetails: map[string]string{
			calc.CodeDivisionByZero:     "Cannot divide by zero.",
			calc.CodeNegativeSquareRoot: "Cannot take the square root of a negative number.",
			calc.CodeInvalidPower:       "Cannot raise a negative number to a fractional power.",
			calc.CodeExponentTooLarge:   "Exponent must be between -1000 and 1000.",
			calc.CodeResultTooLarge:     "Result is too large to calculate.",
		},
	}
}

// evaluateRequest uses a pointer so that a missing or null expression is told apart from
// an empty string (which is the domain's EMPTY validation error, not REQUIRED).
type evaluateRequest struct {
	Expression *string `json:"expression"`
}

// validate checks the request against the contract (api/openapi.yaml). Everything about
// the expression's content is the domain's job.
func (r evaluateRequest) validate() []FieldError {
	if r.Expression == nil {
		return []FieldError{{Field: "expression", Code: "REQUIRED", Message: "expression is required"}}
	}
	return nil
}

type evaluateResponse struct {
	Expression string `json:"expression"`
	Result     string `json:"result"`
}

func (h evaluateHandler) evaluate(w http.ResponseWriter, r *http.Request) {
	var req evaluateRequest
	if p := decodeJSON(w, r, h.maxBodyBytes, &req); p != nil {
		writeProblem(w, r, *p)
		return
	}
	if errs := req.validate(); len(errs) > 0 {
		writeProblem(w, r, Problem{
			Status: http.StatusBadRequest,
			Code:   CodeValidationFailed,
			Detail: errs[0].Message,
			Errors: errs,
		})
		return
	}
	result, err := h.evaluator.Evaluate(r.Context(), *req.Expression)
	if err != nil {
		h.writeEvaluateError(w, r, err)
		return
	}
	writeJSON(w, r, evaluateResponse{Expression: result.Expression, Result: result.Value})
}

// writeEvaluateError is the single place where calculator domain errors become HTTP
// responses.
func (h evaluateHandler) writeEvaluateError(w http.ResponseWriter, r *http.Request, err error) {
	ctx := r.Context()
	var (
		validation *calc.ValidationError
		arithmetic *calc.ArithmeticError
	)
	switch {
	case errors.As(err, &validation):
		writeProblem(w, r, Problem{
			Status: http.StatusBadRequest,
			Code:   CodeValidationFailed,
			Detail: validation.Message,
			Errors: []FieldError{toFieldError(validation)},
		})
	case errors.As(err, &arithmetic):
		detail, known := h.arithmeticDetails[arithmetic.Code]
		if !known {
			loggerFrom(ctx).WarnContext(ctx, "arithmetic code without wording", "code", arithmetic.Code)
			detail = "The expression cannot be calculated."
		}
		writeProblem(w, r, Problem{
			Status: http.StatusUnprocessableEntity,
			Code:   arithmetic.Code,
			Detail: detail,
		})
	case errors.Is(err, context.DeadlineExceeded):
		writeProblem(w, r, Problem{
			Status: http.StatusServiceUnavailable,
			Code:   CodeTimeout,
			Detail: "The calculation took too long.",
		})
	case errors.Is(err, context.Canceled):
		// The client went away; nobody is listening for a body.
		loggerFrom(ctx).DebugContext(ctx, "client cancelled evaluation")
	default:
		loggerFrom(ctx).ErrorContext(ctx, "evaluate expression", "error", err)
		writeProblem(w, r, Problem{Status: http.StatusInternalServerError, Code: CodeInternal})
	}
}

func toFieldError(v *calc.ValidationError) FieldError {
	fe := FieldError{Field: "expression", Code: v.Code, Message: v.Message}
	if v.HasPosition {
		position := v.Position
		fe.Position = &position
	}
	return fe
}
