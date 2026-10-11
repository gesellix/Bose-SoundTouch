//go:build browsertest

package soundtouchweb

import (
	"context"

	"github.com/chromedp/chromedp"
)

// The browser tests run long chromedp.Do sequences that collect results into
// local variables along the way. chromedp's actions return their value
// instead, so these helpers store it and keep the sequences readable.

// evalInto evaluates expr and stores the result in dst.
func evalInto[T any](expr string, dst *T, opts ...chromedp.EvaluateOption) chromedp.Action[chromedp.Void] {
	return into(chromedp.Evaluate[T](expr, opts...), dst)
}

// textInto stores the visible text of the first node matching sel in dst.
func textInto[S chromedp.Selectable](sel S, dst *string, opts ...chromedp.QueryOption) chromedp.Action[chromedp.Void] {
	return into(chromedp.Text(sel, opts...), dst)
}

// attrInto stores the value of the attribute name of the first node matching
// sel in dst.
func attrInto[S chromedp.Selectable](sel S, name string, dst *string, opts ...chromedp.QueryOption) chromedp.Action[chromedp.Void] {
	return func(ctx context.Context, t *chromedp.Target) (chromedp.Void, error) {
		res, err := chromedp.AttributeValue(sel, name, opts...)(ctx, t)
		*dst = res.Value
		return chromedp.Void{}, err
	}
}

// into runs a and stores its value in dst.
func into[T any](a chromedp.Action[T], dst *T) chromedp.Action[chromedp.Void] {
	return func(ctx context.Context, t *chromedp.Target) (chromedp.Void, error) {
		v, err := a(ctx, t)
		if err == nil {
			*dst = v
		}
		return chromedp.Void{}, err
	}
}
