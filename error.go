package config

import "fmt"

func (cfg *Config) error(s string, a ...interface{}) error {
	if len(a) > 0 {
		s = fmt.Sprintf(s, a...)
	}
	return fmt.Errorf("%s in %s:%d", s, cfg.filename, cfg.line)
}
