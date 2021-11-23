package config

import (
	"bytes"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

func (cfg *Config) set(s string) error {
	s = strings.TrimSpace(s)
	if cfg.current.Kind() == reflect.Ptr {
		if cfg.current.IsNil() {
			cfg.current.Set(reflect.New(cfg.current.Type().Elem()))
		}
		cfg.current = cfg.current.Elem()
	}

	// 这里判定一下，是不是要格式化数值
	if format := cfg.typ.Tag.Get("format"); format != "" {
		if err := cfg.setByFormat(format, s); err != nil {
			return err
		}
	} else {
		if err := cfg.setByRaw(s); err != nil {
			return err
		}
	}

	// hook check
	if hook := cfg.typ.Tag.Get("hook"); hook != "" {
		return cfg.runHook(hook, s)
	}

	return nil
}

func (cfg *Config) setByRaw(s string) error {
	switch cfg.current.Kind() {
	case reflect.String:
		cfg.current.SetString(cfg.clearQuoted(s))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if cfg.current.Type().String() == "time.Duration" {
			if time, err := time.ParseDuration(s); err != nil {
				return cfg.error(err.Error())
			} else {
				cfg.current.Set(reflect.ValueOf(time))
			}
		} else {
			itmp, err := strconv.ParseInt(s, 10, cfg.current.Type().Bits())
			if err != nil {
				return cfg.error(err.Error())
			}
			if !cfg.current.OverflowInt(itmp) {
				cfg.current.SetInt(itmp)
			} else {
				return cfg.error("value overflow")
			}
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		itmp, err := strconv.ParseUint(s, 10, cfg.current.Type().Bits())
		if err != nil {
			return cfg.error(err.Error())
		}
		if !cfg.current.OverflowUint(itmp) {
			cfg.current.SetUint(itmp)
		} else {
			return cfg.error("value overflow")
		}
	case reflect.Float32, reflect.Float64:
		ftmp, err := strconv.ParseFloat(s, cfg.current.Type().Bits())
		if err != nil {
			return cfg.error(err.Error())
		}
		if !cfg.current.OverflowFloat(ftmp) {
			cfg.current.SetFloat(ftmp)
		} else {
			return cfg.error("value overflow")
		}
	case reflect.Bool:
		if s == "yes" || s == "on" {
			cfg.current.SetBool(true)
		} else if s == "no" || s == "off" {
			cfg.current.SetBool(false)
		} else {
			btmp, err := strconv.ParseBool(s)
			if err != nil {
				return cfg.error(err.Error())
			}
			cfg.current.SetBool(btmp)
		}
	case reflect.Slice:
		sf, err := cfg.splitQuoted(s)
		if err != nil {
			return err
		}
		for _, sv := range sf {
			n := cfg.current.Len()
			ref := reflect.Zero(cfg.current.Type().Elem())
			cfg.init(ref)
			cfg.current.Set(reflect.Append(cfg.current, ref))
			cfg.pushElement(cfg.current.Index(n))
			cfg.set(sv)
			cfg.popElement()
		}
	case reflect.Map:
		if cfg.current.IsNil() {
			cfg.current.Set(reflect.MakeMap(cfg.current.Type()))
		}

		sf, err := cfg.splitQuoted(s)
		if err != nil {
			return err
		}
		if len(sf) != 2 {
			return cfg.error("invalid map config: %s", s)
		}
		var v reflect.Value
		v = reflect.New(cfg.current.Type().Key())
		cfg.pushElement(v)
		cfg.set(sf[0])
		key := cfg.current
		cfg.popElement()
		v = reflect.New(cfg.current.Type().Elem())
		cfg.pushElement(v)
		cfg.set(sf[1])
		val := cfg.current
		cfg.popElement()

		cfg.current.SetMapIndex(key, val)
	default:
		if cfg.current.Kind() == reflect.Struct {
			return cfg.error("invalid block, start a block with '{'")
		} else {
			return cfg.error(fmt.Sprintf("invalid type:%s", cfg.current.Kind()))
		}
	}

	return nil
}

func (cfg *Config) setByFormat(format, s string) error {
	var fn reflect.Value
	found := false
	method := cfg.fixedField(format)
	// 在配置本身上面找
	if element, ok := cfg.getStruct(); ok {
		if element.Kind() != reflect.Ptr && element.CanAddr() {
			element = element.Addr()
		}

		fn = element.MethodByName(method)
		found = fn.IsValid() && fn.Kind() == reflect.Func
	}

	if !found {
		fn = reflect.ValueOf(cfg.format).MethodByName(method)

		if !fn.IsValid() || fn.Kind() != reflect.Func {
			return cfg.error("tag: fomrat:\"%s\" in %s, func not exists", format, cfg.typ.Name)
		}
	}

	if fn.Type().NumOut() != 2 {
		return cfg.error("tag: fomrat:\"%s\" in %s, func return invalid result, 2 result required", format, cfg.typ.Name)
	}

	if fn.Type().Out(0) != cfg.current.Type() {
		return cfg.error("tag: fomrat:\"%s\" in %s, invalid val kind, result is %s, but %s required", format, cfg.typ.Name, fn.Type().Out(0).String(), cfg.current.Type().String())
	}

	if !fn.Type().Out(1).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		return cfg.error("tag: fomrat:\"%s\" in %s, func return invalid result, error type required", format, cfg.typ.Name)
	}

	result := fn.Call([]reflect.Value{reflect.ValueOf(s)})

	if !result[1].IsNil() {
		return cfg.error("tag: fomrat:\"%s\" in %s, invalid val with %s, and return %s", format, cfg.typ.Name, s, result[1].Interface().(error).Error())
	}

	cfg.current.Set(result[0])
	return nil
}

func (cfg *Config) replace(s *bytes.Buffer, vs *bytes.Buffer) bool {
	if vs.Len() == 0 {
		return false
	}

	for k, v := range cfg.currentVar {
		if strings.Compare(k, vs.String()) == 0 {
			// found
			cfg.searchVar = false
			s.WriteString(v)
			vs.Reset()
			return true
		}
	}

	for i := len(cfg.vars) - 1; i >= 0; i-- {
		for k, v := range cfg.vars[i] {
			if strings.Compare(k, vs.String()) == 0 {
				s.WriteString(v)
				cfg.searchVar = false
				vs.Reset()
				return true
			}
		}
	}

	return false
}

func (cfg *Config) runHook(hook string, s string) error {
	hook = cfg.fixedField(hook)
	fn, found := cfg.getMethod(hook)

	if !found {
		fn = reflect.ValueOf(cfg.hook).MethodByName(hook)
		if !fn.IsValid() || fn.Kind() != reflect.Func {
			return cfg.error("tag: hook:\"%s\" in %s, func not exists", hook, cfg.typ.Name)
		}
	}

	if fn.Type().NumOut() != 1 {
		return cfg.error("tag: hook:\"%s\" in %s, func return invalid result", hook, cfg.typ.Name)
	}

	if !fn.Type().Out(0).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		return cfg.error("tag: hook:\"%s\" in %s, func return invalid result, error required", hook, cfg.typ.Name)
	}

	result := fn.Call([]reflect.Value{reflect.ValueOf(s)})

	if !result[0].IsNil() {
		return result[0].Interface().(error)
	}

	return nil
}
