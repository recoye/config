package config

import (
	"bytes"
	"reflect"
	"strings"
)

func (cfg *Config) getElement(s string) error {
	cfg.Lock()
	defer cfg.Unlock()
	s = strings.TrimSpace(s)

	if !cfg.current.IsValid() {
		if ok := cfg.fetchDirective(s); ok {
			return nil
		} else {
			cfg.current = cfg.entry
		}
	}

	cfg.restoreElement()

	if cfg.current.Kind() != reflect.Struct {
		return cfg.error("unknown directive %s, current kind is %s, but struct required", s, cfg.current.Kind())
	}

	// 如果是驼峰，就要处理一下
	field := cfg.fixedField(s)

	var ok bool
	if cfg.typ, ok = cfg.current.Type().FieldByName(field); !ok {
		cfg.popElement()
		if cfg.current == cfg.entry {
			if ok := cfg.fetchDirective(field); ok {
				return nil
			}
		}
		return cfg.error("unknown directive %s ", s)
	}

	cfg.current = cfg.current.FieldByName(field)

	return nil
}

func (cfg *Config) fetchDirective(s string) bool {
	if rev, ok := cfg.directives[s]; ok {
		rev.Runnable = true
		cfg.current = rev.config
		cfg.fixedElement()
		return true
	}
	return false
}

func (cfg *Config) getStruct() (reflect.Value, bool) {
	if len(cfg.queue) < 1 {
		return reflect.Value{}, false
	}
	return cfg.queue[len(cfg.queue)-1], true
}

func (cfg *Config) getMethod(s string) (reflect.Value, bool) {
	if element, ok := cfg.getStruct(); ok {
		if element.Kind() != reflect.Ptr && element.CanAddr() {
			element = element.Addr()
		}
		fn := element.MethodByName(s)
		if fn.IsValid() && fn.Kind() == reflect.Func {
			return fn, true
		}
	}

	return reflect.Value{}, false
}

func (cfg *Config) getNearestSlice() (reflect.Value, bool) {
	if len(cfg.queue) < 2 {
		return reflect.Value{}, false
	}
	return cfg.queue[len(cfg.queue)-2], true
}

func (cfg *Config) restoreElement() {
	cfg.fixedElement()
	cfg.queue = append(cfg.queue, cfg.current)
}

func (cfg *Config) fixedElement() {
	if cfg.current.Kind() == reflect.Ptr {
		if cfg.current.IsNil() {
			cfg.current.Set(reflect.New(cfg.current.Type().Elem()))
		}
		cfg.current = cfg.current.Elem()
	}
}

func (cfg *Config) pushElement(v reflect.Value) {
	cfg.queue = append(cfg.queue, cfg.current)
	cfg.current = v
}

func (cfg *Config) popElement() {
	if len(cfg.queue) < 1 {
		cfg.current = reflect.Value{}
		return
	}
	cfg.current = cfg.queue[len(cfg.queue)-1]
	cfg.queue = cfg.queue[:len(cfg.queue)-1]
}

func (cfg *Config) fixedField(s string) string {
	if cfg.camel {
		stmp := strings.Split(s, "_")
		buffer := bytes.Buffer{}
		for _, v := range stmp {
			buffer.WriteString(strings.Title(v))
		}

		s = buffer.String()
	} else {
		s = strings.Title(s)
	}

	return s
}
