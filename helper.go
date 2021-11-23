package config

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"unicode"
)

func (cfg *Config) reset() {
	cfg.inSearchKey()
	cfg.inBlock = 0
	cfg.inInclude = 0
	cfg.currentVar = make(map[string]string)
	cfg.searchVar = false
	cfg.setVar = false
	cfg.searchVarBlock = false
	cfg.line = 1
}

func (cfg *Config) valueOf(conf interface{}) (reflect.Value, error) {
	rev := reflect.ValueOf(conf)
	if rev.Type().Name() == "Value" {
		rev = rev.Interface().(reflect.Value)
	} else if rev.Kind() != reflect.Ptr {
		return reflect.Value{}, cfg.error("non-pointer and can't be addr")
	}

	if err := cfg.init(rev); err != nil {
		return reflect.Value{}, err
	}

	rev = rev.Elem()

	return rev, nil
}

func (cfg *Config) init(rev reflect.Value) error {
	if rev.Kind() == reflect.Ptr {
		rev = rev.Elem()
	}

	if rev.Type().Kind() == reflect.Slice {
		for i := 0; i < rev.Len(); i++ {
			if err := cfg.init(rev.Index(i)); err != nil {
				return err
			}
		}
		return nil
	}

	if rev.Type().Kind() != reflect.Struct {
		return nil
	}

	fn := rev.MethodByName("Init")
	found := false
	if fn.IsValid() && fn.Kind() == reflect.Func {
		found = true
	}

	if !found && rev.CanAddr() {
		fn = rev.Addr().MethodByName("Init")
		if fn.IsValid() && fn.Kind() == reflect.Func {
			found = true
		}
	}

	if found {
		if fn.Type().NumOut() != 1 {
			return cfg.error("init in %s,invalid val kind, %s result required", rev.Type().String(), rev.Type().String())
		}

		if !fn.Type().Out(0).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
			return cfg.error("init in %s, func return invalid result, error required but return %s", rev.Type().String(), fn.Type().Out(0).String())
		}

		result := fn.Call([]reflect.Value{})
		if result[0].IsNil() {
			return nil
		} else {
			return cfg.error("init in %s, result: %s", rev.Type().String(), result[0].Interface().(error).Error())
		}
	}

	typs := rev.Type()

	for i := 0; i < typs.NumField(); i++ {
		field := typs.Field(i)
		// 这里是设置INIT
		ref := rev.FieldByName(field.Name)

		// 在配置本身上面找
		if ref.Kind() == reflect.Struct {
			if err := cfg.init(ref); err != nil {
				return err
			}
		} else if ref.Kind() == reflect.Ptr && ref.Elem().Kind() == reflect.Struct {
			if err := cfg.init(ref); err != nil {
				return err
			}
		}

		if init := field.Tag.Get("init"); init != "" {
			// 获取函数
			method := cfg.fixedField(init)
			found := false
			fn := rev.MethodByName(method)
			if fn.IsValid() && fn.Kind() == reflect.Func {
				found = true
			}

			if !found && rev.CanAddr() {
				fn = rev.Addr().MethodByName(method)
				if !fn.IsValid() || fn.Kind() != reflect.Func {
					return cfg.error("tag: init:\"%s\" in %s, func not exists", init, rev.Type().String())
				}
			}

			if fn.Type().NumOut() != 1 {
				return cfg.error("tag: init:\"%s\" in %s,invalid val kind, one result required, but %s return", init, field.Name, ref.Type().String(), fn.Type().NumOut())
			}

			result := fn.Call([]reflect.Value{})
			ref.Set(result[0])
		}

	}
	return nil
}

func (cfg *Config) inSearchKey() {
	cfg.searchVal = false
	cfg.searchKey = true
	cfg.canSkip = true
}

func (cfg *Config) inSearchVal() {
	cfg.searchKey = false
	cfg.searchVal = true
	cfg.canSkip = false
}

func (cfg *Config) delimiter(b byte) bool {
	return unicode.IsSpace(rune(b))
}

func (cfg *Config) clearQuoted(s string) string {
	s = strings.TrimSpace(s)
	if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}

	return s
}

func (cfg *Config) splitQuoted(s string) ([]string, error) {
	var sq []string
	s = strings.TrimSpace(s)
	var last_space bool = true
	var need_space bool = true
	var d_quote bool = false
	var s_quote bool = false
	var quote bool = false
	var ch byte
	var vs bytes.Buffer

	for i := 0; i < len(s); i++ {
		ch = s[i]

		if quote {
			quote = false
			vs.WriteByte(ch)
			continue
		}

		if ch == '\\' {
			quote = true
			last_space = false
			continue
		}

		if last_space {
			last_space = false
			switch ch {
			case '"':
				d_quote = true
				need_space = false
				continue
			case '\'':
				s_quote = true
				need_space = false
				continue
			case ' ':
				last_space = true
				continue
			}
			vs.WriteByte(ch)
		} else {
			if need_space && cfg.delimiter(ch) {
				if vs.Len() > 0 {
					sq = append(sq, vs.String())
				}
				vs.Reset()
				last_space = true
				continue
			}

			if d_quote {
				if ch == '"' {
					d_quote = false
					need_space = true
					continue
				}
			} else if s_quote {
				if ch == '\'' {
					s_quote = false
					need_space = true
					continue
				}
			}

			vs.WriteByte(ch)
		}
	}

	if quote || s_quote || d_quote {
		return nil, cfg.error(fmt.Sprintf("invalid value: %v", s))
	}

	if vs.Len() > 0 {
		sq = append(sq, vs.String())
	}

	return sq, nil
}
