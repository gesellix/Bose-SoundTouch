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

// navigateFixture opens a fixture page and waits until Preact has run the
// effects of its first render.
//
// The fixtures render synchronously in a module script, so the DOM is there
// once Navigate returns, but Preact runs useEffect only after the next frame.
// Mount effects reset component state: Controls, for example, ends any volume
// gesture and shows the speaker's volume again. A test that starts a gesture
// in that gap loses it to the mount effect, and the next speaker update moves
// the slider it believes is held. That made the volume tests fail at random,
// about one run in twenty.
func navigateFixture(url string) chromedp.Action[chromedp.Void] {
	return func(ctx context.Context, t *chromedp.Target) (chromedp.Void, error) {
		if _, err := chromedp.Navigate(url)(ctx, t); err != nil {
			return chromedp.Void{}, err
		}
		return chromedp.Evaluate[chromedp.Void](afterEffectsScript, chromedp.EvalAwaitPromise)(ctx, t)
	}
}

// afterEffectsScript resolves once Preact has run pending effects: Preact
// schedules them for after the next frame, from a requestAnimationFrame
// callback followed by a timeout.
const afterEffectsScript = `new Promise(resolve =>
	requestAnimationFrame(() => requestAnimationFrame(() => setTimeout(resolve))))`
