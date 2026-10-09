package snif

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"
)

// Decode copies a form's answers into a struct, which is how a program built on
// snif usually consumes a page:
//
//	var v struct {
//	    Due   time.Time `snif:"due"`
//	    Prio  string    `snif:"prio"`
//	    Mood  float64   `snif:"mood"`
//	    Done  bool      `snif:"done"`
//	    Notes string    `snif:"notes,required"`
//	}
//	err := form.Decode(&v)
//
// A field is matched to a widget by its snif tag, or by its lower-cased name when
// it has none; `snif:"-"` skips it. Field types follow the widget kinds: bool for
// a Check or Button, string for a Radio or Input, a float or integer for a Slider,
// and time.Time (or a string, formatted "YYYY-MM-DD") for a Date.
//
// An unanswered widget leaves its field at the zero value without complaint — a
// form is allowed to be partly filled in — unless the tag says ",required". A
// widget that failed to read (an ambiguous group, an incomplete date) is always an
// error: the page is contradictory, not merely blank.
func (f *Form) Decode(v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("Decode wants a pointer to a struct, got %T", v)
	}
	rv = rv.Elem()
	rt := rv.Type()

	for i := 0; i < rt.NumField(); i++ {
		ft := rt.Field(i)
		if !ft.IsExported() {
			continue
		}
		name, required, skip := fieldTag(ft)
		if skip {
			continue
		}
		w, ok := f.Widget(name)
		if !ok {
			return fmt.Errorf("field %s: no widget %q on this form", ft.Name, name)
		}
		if w.Err != nil {
			return fmt.Errorf("field %s: %s: %w", ft.Name, name, w.Err)
		}
		if !w.Set {
			if required {
				return fmt.Errorf("field %s: %s is not filled in", ft.Name, name)
			}
			continue
		}
		if err := assign(rv.Field(i), w); err != nil {
			return fmt.Errorf("field %s (%s %s): %w", ft.Name, w.Kind, name, err)
		}
	}
	return nil
}

// fieldTag reads a field's widget name and options.
func fieldTag(ft reflect.StructField) (name string, required, skip bool) {
	tag, ok := ft.Tag.Lookup("snif")
	if !ok {
		return strings.ToLower(ft.Name), false, false
	}
	if tag == "-" {
		return "", false, true
	}
	name, opts, _ := strings.Cut(tag, ",")
	if name == "" {
		name = strings.ToLower(ft.Name)
	}
	for _, o := range strings.Split(opts, ",") {
		if o == "required" {
			required = true
		}
	}
	return name, required, false
}

var timeType = reflect.TypeOf(time.Time{})

// assign writes one widget's value into a struct field, converting only where the
// conversion is exact — a slider read into an int must land on a whole number, or
// the field would silently disagree with the page.
func assign(dst reflect.Value, w Widget) error {
	if dst.Type() == timeType {
		if w.Kind != Date {
			return fmt.Errorf("a %s does not produce a time.Time", w.Kind)
		}
		dst.Set(reflect.ValueOf(w.Date))
		return nil
	}
	switch dst.Kind() {
	case reflect.Bool:
		if w.Kind != Check && w.Kind != Button {
			return fmt.Errorf("a %s does not produce a bool", w.Kind)
		}
		dst.SetBool(w.Bool)
	case reflect.String:
		switch w.Kind {
		case Radio:
			dst.SetString(w.Choice)
		case Input:
			dst.SetString(w.Text)
		case Date:
			dst.SetString(w.Date.Format("2006-01-02"))
		default:
			return fmt.Errorf("a %s does not produce a string", w.Kind)
		}
	case reflect.Float32, reflect.Float64:
		if w.Kind != Slider {
			return fmt.Errorf("a %s does not produce a number", w.Kind)
		}
		dst.SetFloat(w.Number)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if w.Kind != Slider {
			return fmt.Errorf("a %s does not produce a number", w.Kind)
		}
		if w.Number != math.Trunc(w.Number) {
			return fmt.Errorf("value %v is not a whole number; use a float field or a step", w.Number)
		}
		dst.SetInt(int64(w.Number))
	default:
		return fmt.Errorf("unsupported field type %s", dst.Type())
	}
	return nil
}
