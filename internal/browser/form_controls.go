package browser

import (
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
)

func fillElement(element *rod.Element, text string) error {
	kind, err := element.Eval(`function(){ return this.tagName === 'INPUT' ? this.type : '' }`)
	if err != nil {
		return err
	}
	switch kind.Value.Str() {
	case "date", "datetime-local", "time", "month", "week":
		if err = element.Focus(); err != nil {
			return err
		}
		if err = element.WaitEnabled(); err != nil {
			return err
		}
		if err = element.WaitWritable(); err != nil {
			return err
		}
		// Chromium's segmented date/time controls do not accept InsertText like
		// textboxes. Use their native value setter and normal form events. A
		// detached probe validates the format without destroying the old value.
		_, err = element.Eval(`function(value){
			const document = this.ownerDocument;
			const probe = document.createElement('input');
			probe.type = this.type;
			probe.value = value;
			if (probe.value !== value) throw new Error('Invalid ' + this.type + ' value; use the HTML ISO format');
			const window = document.defaultView;
			Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set.call(this, value);
			this.dispatchEvent(new window.Event('input', {bubbles: true}));
			this.dispatchEvent(new window.Event('change', {bubbles: true}));
		}`, strings.TrimSpace(text))
		return err
	default:
		if err = element.SelectAllText(); err != nil {
			return err
		}
		if text == "" {
			return element.Type(input.Backspace)
		}
		return element.Input(text)
	}
}
