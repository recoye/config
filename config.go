package config

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
)

type Configurable struct {
	Runnable bool
	config   reflect.Value
}

// Config A Config struct
type Config struct {
	sync.Mutex
	filename       string
	queue          []reflect.Value
	current        reflect.Value
	entry          reflect.Value
	typ            reflect.StructField
	searchVal      bool
	searchKey      bool
	inBlock        int
	inInclude      int
	canSkip        bool
	skip           bool
	bkQueue        []bool
	bkMulti        bool
	mapKey         reflect.Value
	setVar         bool
	searchVar      bool
	searchVarBlock bool
	vars           []map[string]string
	currentVar     map[string]string
	camel          bool
	directives     map[string]*Configurable
	cwd            string
	file           *os.File
	line           int64
	stash          []*stash
	format         *format
	hook           *hook
}

// New a config parser with filename
func New(filename string) *Config {
	conf := &Config{filename: filename, camel: true}
	conf.directives = make(map[string]*Configurable)
	conf.format = &format{}
	conf.hook = &hook{conf}
	return conf
}

// Config.AutoCamel auto replace _ to camel
func (cfg *Config) AutoCamel(b bool) {
	cfg.camel = b
}

// Config.Entry set an entry for parser
func (cfg *Config) Entry(entry interface{}) error {
	if cfg.entry.IsValid() {
		return cfg.error("entry already set")
	}
	var err error
	cfg.entry, err = cfg.valueOf(entry)
	if err != nil {
		return err
	}

	return nil
}

// Config.Directive add a directive for paser
func (cfg *Config) Directive(directive string, conf interface{}) (*Configurable, error) {
	cfg.Lock()
	defer cfg.Unlock()

	directive = cfg.fixedField(directive)
	rev, err := cfg.valueOf(conf)
	if err != nil {
		return nil, err
	}

	config := &Configurable{Runnable: false, config: rev}
	if _, ok := cfg.directives[directive]; ok {
		return nil, cfg.error("directive [ %s ] duplication", directive)
	} else {
		cfg.directives[directive] = config
	}

	return config, nil
}

// Config.Parse  parse config with Entry and directive from file
func (cfg *Config) Parse() error {
	if !cfg.entry.IsValid() || cfg.entry.IsZero() {
		return cfg.error("entry be required")
	}

	cfg.current = reflect.Value{}

	cfg.reset()

	if err := cfg.parse(); err != nil {
		return err
	}

	cfg.current = reflect.Value{}
	return nil
}

// Config.Unmarshal  unmarshal config file to v
func (cfg *Config) Unmarshal(v interface{}) error {
	var err error
	if cfg.current, err = cfg.valueOf(v); err != nil {
		return err
	}

	cfg.reset()

	return cfg.parse()
}

// Config.Reload reload config file
func (cfg *Config) Reload() error {
	cfg.reset()
	return cfg.parse()
}

func (cfg *Config) parse() error {
	var err error
	var s bytes.Buffer
	var vs bytes.Buffer
	var vsb bytes.Buffer
	if _, err = os.Stat(cfg.filename); os.IsNotExist(err) {
		return cfg.error(err.Error())
	}

	filename, err := filepath.Abs(cfg.filename)
	if err != nil {
		return cfg.error(err.Error())
	} else {
		cfg.cwd = filepath.Dir(filename)
	}

	var file *os.File
	file, err = os.Open(filename)
	if err != nil {
		return cfg.error(err.Error())
	}
	defer func() {
		file.Close()
	}()

	cfg.filename = file.Name()
	cfg.file = file

	cfg.line = 1

	reader := bufio.NewReader(cfg.file)
	var b byte
	for {
		b, err = reader.ReadByte()
		if err == io.EOF {
			if !cfg.searchKey {
				return cfg.error("invalid config vaild")
			}

			if s.Len() > 0 {
				return cfg.error("\"%s\" directive is not allowed here", s.String())
			}

			if err := cfg.popStash(); err != nil {
				return err
			}
			break
		} else if err != nil {
			return cfg.error(err.Error())
		}

		// bug in \r
		if b == '\n' {
			cfg.line++
		}

		if cfg.canSkip && b == '#' {
			reader.ReadLine()
			cfg.line++
			continue
		}
		if cfg.canSkip && b == '/' {
			if cfg.skip {
				reader.ReadLine()
				cfg.line++
				cfg.skip = false
				continue
			}
			cfg.skip = true
			continue
		}
		if cfg.searchKey {
			if b == ';' {
				return cfg.error("unknown directive \"" + s.String() + "\"")
			}

			if cfg.delimiter(b) {
				if s.Len() > 0 {
					cfg.inSearchVal()
					if strings.Compare(s.String(), "include") == 0 {
						s.Reset()
						cfg.inInclude++
						if cfg.inInclude > 100 {
							return cfg.error("too many include, exceeds 100 limit")
						}
						continue
					} else if strings.Compare(s.String(), "set") == 0 {
						s.Reset()
						cfg.setVar = true
						continue
					}
					if err := cfg.getElement(s.String()); err != nil {
						return err
					}
					s.Reset()
				}
				continue
			}
		}

		if b == '{' && !cfg.searchVar && vs.Len() == 0 {
			if err := cfg.createBlock(&s); err != nil {
				return err
			}
			continue
		}

		if cfg.searchKey && b == '}' && cfg.inBlock > 0 {
			cfg.closeBlock(&s)
			continue
		}

		if cfg.searchVal {
			if b == '$' {
				cfg.searchVar = true
				vs.Reset()
				vsb.Reset()
				vsb.WriteByte(b)
				continue
			}

			if cfg.searchVar {
				if b == '{' {
					if vs.Len() == 0 {
						cfg.searchVarBlock = true
						vsb.WriteByte(b)
						continue
					}
					if !cfg.searchVarBlock {
						if !cfg.replace(&s, &vs) {
							s.Write(vsb.Bytes())
						}

						// is block?
						if err := cfg.createBlock(&s); err != nil {
							return err
						} else {
							continue
						}
					}
				} // if b == '{'
				// Is space
				if cfg.delimiter(b) {
					if !cfg.replace(&s, &vs) {
						s.Write(vsb.Bytes())
					}
				}
			}

			if cfg.searchVarBlock && b == '}' {
				cfg.searchVarBlock = false
				// replace $???
				cfg.searchVar = false
				if !cfg.replace(&s, &vs) {
					vsb.WriteByte(b)
					s.Write(vsb.Bytes())
				}
				continue
			}

			if b == ';' {
				if s.Len() < 1 {
					return cfg.error("unknown value of  directive \"" + s.String() + "\"")
				}
				//  copy to cfg.current
				cfg.inSearchKey()
				if cfg.searchVar {
					if cfg.searchVarBlock {
						return cfg.error(vsb.String() + " is not terminated by }")
					}
					cfg.searchVar = false
					cfg.searchVarBlock = false
					if !cfg.replace(&s, &vs) {
						s.Write(vsb.Bytes())
					}
				}

				// set to map
				if cfg.setVar {
					sf := strings.Fields(s.String())
					if len(sf) != 2 {
						return cfg.error("set map with %s invalid", s.String())
					}
					cfg.currentVar[sf[0]] = sf[1]
					cfg.setVar = false
					s.Reset()
					continue
				}

				if cfg.inInclude > 0 {
					cfg.filename = strings.TrimSpace(s.String())
					s.Reset()
					cfg.inInclude--
					var files []string
					if filepath.IsAbs(cfg.filename) {
						files, err = filepath.Glob(cfg.filename)
					} else {
						files, err = filepath.Glob(filepath.Join(cfg.cwd, cfg.filename))
					}
					if err != nil {
						return cfg.error(err.Error())
					}
					for _, file := range files {
						cfg.pushStash()
						cfg.filename = file
						if err := cfg.parse(); err != nil {
							return err
						}
					}
					continue
				}

				err := cfg.set(s.String())
				if err != nil {
					return err
				}

				s.Reset()
				cfg.popElement()
				continue
			} else if cfg.searchVar { // if b == ';'
				vs.WriteByte(b)
				vsb.WriteByte(b)
				if !cfg.searchVarBlock {
					cfg.replace(&s, &vs)
				}
				continue
			}
		}

		s.WriteByte(b)
	}

	if !cfg.searchKey && cfg.inBlock > 0 {
		return cfg.error("invalid config file")
	}

	return nil
}
