package config

import (
	"bytes"
	"reflect"
)

func (cfg *Config) createBlock(s *bytes.Buffer) error {
	// fixed { be close to key like server{
	if cfg.searchKey && s.Len() > 0 {
		if err := cfg.getElement(s.String()); err != nil {
			return err
		}

		s.Reset()
		cfg.inSearchVal()
	}

	// vars
	vars := make(map[string]string)
	cfg.vars = append(cfg.vars, cfg.currentVar)
	cfg.currentVar = vars

	cfg.inBlock++
	//  slice or map?
	cfg.bkQueue = append(cfg.bkQueue, cfg.bkMulti)
	cfg.bkMulti = false
	if cfg.searchVal && s.Len() > 0 && cfg.current.Kind() == reflect.Map {
		cfg.bkMulti = true
		if cfg.current.IsNil() {
			cfg.current.Set(reflect.MakeMap(cfg.current.Type()))
		}

		v := reflect.New(cfg.current.Type().Key())
		cfg.pushElement(v)
		err := cfg.set(s.String())
		if err != nil {
			return err
		}
		cfg.mapKey = cfg.current
		cfg.popElement()
		val := reflect.New(cfg.current.Type().Elem())
		cfg.pushElement(val)
	}

	if cfg.current.Kind() == reflect.Slice {
		cfg.pushMultiBlock()
		n := cfg.current.Len()
		if cfg.current.Type().Elem().Kind() == reflect.Ptr {
			ref := reflect.New(cfg.current.Type().Elem().Elem())
			if err := cfg.init(ref); err != nil {
				return err
			}
			cfg.current.Set(reflect.Append(cfg.current, ref))
		} else {
			ref := reflect.New(cfg.current.Type().Elem())
			if err := cfg.init(ref); err != nil {
				return err
			}
			cfg.current.Set(reflect.Append(cfg.current, ref.Elem()))
		}
		cfg.pushElement(cfg.current.Index(n))
	}
	cfg.inSearchKey()
	s.Reset()
	return nil
}

func (cfg *Config) closeBlock(s *bytes.Buffer) {
	if cfg.bkMulti {
		val := cfg.current
		cfg.popElement()
		if cfg.current.Kind() == reflect.Map {
			cfg.current.SetMapIndex(cfg.mapKey, val)
		}
	}
	cfg.popMultiBlock()

	// vars
	cfg.currentVar = cfg.vars[len(cfg.vars)-1]
	cfg.vars = cfg.vars[:len(cfg.vars)-1]

	cfg.inBlock--
	cfg.popElement()
	cfg.inSearchKey()
}

func (cfg *Config) pushMultiBlock() {
	cfg.bkQueue = append(cfg.bkQueue, cfg.bkMulti)
	cfg.bkMulti = true
}

func (cfg *Config) popMultiBlock() {
	cfg.bkMulti = cfg.bkQueue[len(cfg.bkQueue)-1]
	cfg.bkQueue = cfg.bkQueue[:len(cfg.bkQueue)-1]
}
