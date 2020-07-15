package config

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"time"
)

type IConfigLog interface {
}

// Config A Config struct
type Config struct {
	filename       string
	queue          []reflect.Value
	current        reflect.Value
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
}

// Cofnig.New create a config parser with filename
func New(filename string) *Config {
	return &Config{filename: filename, camel: true}
}

// Config.AutoCamel auto replace _ to camel
func (c *Config) AutoCamel(b bool) {
	c.camel = b
}

// Config.Unmarshal  unmarshal config file to v
func (c *Config) Unmarshal(v interface{}) error {
	rev := reflect.ValueOf(v)
	if rev.Kind() != reflect.Ptr {
		err := errors.New("non-pointer passed to Unmarshal")
		return err
	}
	c.current = rev.Elem()
	c.inSearchKey()
	c.inBlock = 0
	c.inInclude = 0
	c.currentVar = make(map[string]string)
	c.searchVar = false
	c.setVar = false
	c.searchVarBlock = false
	return c.parse()
}

// Config.Reload reload config file
func (c *Config) Reload() error {
	return c.parse()
}

func (c *Config) parse() error {
	var err error
	var s bytes.Buffer
	var vs bytes.Buffer
	var vsb bytes.Buffer
	if _, err = os.Stat(c.filename); os.IsNotExist(err) {
		return err
	}

	var fp *os.File
	fp, err = os.Open(c.filename)
	defer fp.Close()
	if err != nil {
		return err
	}

	reader := bufio.NewReader(fp)
	var b byte
	for err == nil {
		b, err = reader.ReadByte()
		if err == bufio.ErrBufferFull {
			return nil
		}

		if c.canSkip && b == '#' {
			reader.ReadLine()
			continue
		}
		if c.canSkip && b == '/' {
			if c.skip {
				reader.ReadLine()
				c.skip = false
				continue
			}
			c.skip = true
			continue
		}
		if c.searchKey {
			if c.delimiter(b) {
				if s.Len() > 0 {
					c.inSearchVal()
					if strings.Compare(s.String(), "include") == 0 {
						s.Reset()
						c.inInclude++
						if c.inInclude > 100 {
							return errors.New("too many include, exceeds 100 limit!")
						}
						continue
					} else if strings.Compare(s.String(), "set") == 0 {
						s.Reset()
						c.setVar = true
						continue
					}
					c.getElement(s.String())
					s.Reset()
				}
				continue
			}
		}

		if b == '{' && !c.searchVar && vs.Len() == 0 {
			if err := c.createBlock(&s); err != nil {
				return err
			}
			continue
		}

		if c.searchKey && b == '}' && c.inBlock > 0 {
			c.closeBlock(&s)
			continue
		}

		if c.searchVal {
			if b == '$' {
				c.searchVar = true
				vs.Reset()
				vsb.Reset()
				vsb.WriteByte(b)
				continue
			}

			if c.searchVar {
				if b == '{' {
					if vs.Len() == 0 {
						c.searchVarBlock = true
						vsb.WriteByte(b)
						continue
					}
					if !c.searchVarBlock {
						if !c.replace(&s, &vs) {
							s.Write(vsb.Bytes())
						}

						// is block?
						if err := c.createBlock(&s); err != nil {
							return err
						} else {
							continue
						}
					}
				} // if b == '{'
				// Is space
				if c.delimiter(b) {
					if !c.replace(&s, &vs) {
						s.Write(vsb.Bytes())
					}
				}
			}

			if c.searchVarBlock && b == '}' {
				c.searchVarBlock = false
				// replace $???
				c.searchVar = false
				if !c.replace(&s, &vs) {
					vsb.WriteByte(b)
					s.Write(vsb.Bytes())
				}
				continue
			}

			if b == ';' {
				//  copy to c.current
				c.inSearchKey()
				if c.searchVar {
					if c.searchVarBlock {
						return errors.New(vsb.String() + " is not terminated by }")
					}
					c.searchVar = false
					c.searchVarBlock = false
					if !c.replace(&s, &vs) {
						s.Write(vsb.Bytes())
					}
				}

				// set to map
				if c.setVar {
					sf := strings.Fields(s.String())
					if len(sf) != 2 {
						return errors.New("Invalid Config")
					}
					c.currentVar[sf[0]] = sf[1]
					c.setVar = false
					s.Reset()
					continue
				}

				if c.inInclude > 0 {
					c.filename = strings.TrimSpace(s.String())
					s.Reset()
					c.inInclude--
					files, err := filepath.Glob(c.filename)
					if err != nil {
						return err
					}
					for _, file := range files {
						c.filename = file
						if err := c.parse(); err != nil {
							return err
						}
					}
					continue
				}

				err := c.set(s.String())
				if err != nil {
					return err
				}

				s.Reset()
				c.popElement()
				continue
			} else if c.searchVar { // if b == ';'
				vs.WriteByte(b)
				vsb.WriteByte(b)
				if !c.searchVarBlock {
					c.replace(&s, &vs)
				}
				continue
			}
		}

		s.WriteByte(b)
	}

	if !c.searchKey && c.inBlock > 0 {
		return errors.New("Invalid config file!")
	}

	return nil
}

func (c *Config) set(s string) error {
	s = strings.TrimSpace(s)
	if c.current.Kind() == reflect.Ptr {
		if c.current.IsNil() {
			c.current.Set(reflect.New(c.current.Type().Elem()))
		}
		c.current = c.current.Elem()
	}

	switch c.current.Kind() {
	case reflect.String:
		c.current.SetString(c.clearQuoted(s))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if c.current.Type().String() == "time.Duration" {
			if time, err := time.ParseDuration(s);err != nil {
				return err
			}else{
				c.current.Set(reflect.ValueOf(time))
			}
		}else{
			itmp, err := strconv.ParseInt(s, 10, c.current.Type().Bits())
			if err != nil {
				return err
			}
			c.current.SetInt(itmp)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		itmp, err := strconv.ParseUint(s, 10, c.current.Type().Bits())
		if err != nil {
			return err
		}
		c.current.SetUint(itmp)
	case reflect.Float32, reflect.Float64:
		ftmp, err := strconv.ParseFloat(s, c.current.Type().Bits())
		if err != nil {
			return err
		}
		c.current.SetFloat(ftmp)
	case reflect.Bool:
		if s == "yes" || s == "on" {
			c.current.SetBool(true)
		} else if s == "no" || s == "off" {
			c.current.SetBool(false)
		} else {
			btmp, err := strconv.ParseBool(s)
			if err != nil {
				return err
			}
			c.current.SetBool(btmp)
		}
	case reflect.Slice:
		sf, err := c.splitQuoted(s)
		if err != nil {
			return err
		}
		for _, sv := range sf {
			n := c.current.Len()
			c.current.Set(reflect.Append(c.current, reflect.Zero(c.current.Type().Elem())))
			c.pushElement(c.current.Index(n))
			c.set(sv)
			c.popElement()
		}
	case reflect.Map:
		if c.current.IsNil() {
			c.current.Set(reflect.MakeMap(c.current.Type()))
		}

		sf, err := c.splitQuoted(s)
		if err != nil {
			return err
		}
		if len(sf) != 2 {
			return errors.New("Invalid Config")
		}
		var v reflect.Value
		v = reflect.New(c.current.Type().Key())
		c.pushElement(v)
		c.set(sf[0])
		key := c.current
		c.popElement()
		v = reflect.New(c.current.Type().Elem())
		c.pushElement(v)
		c.set(sf[1])
		val := c.current
		c.popElement()

		c.current.SetMapIndex(key, val)
	default:
		return errors.New(fmt.Sprintf("Invalid Type:%s", c.current.Kind()))
	}
	return nil
}

func (c *Config) replace(s *bytes.Buffer, vs *bytes.Buffer) bool {
	if vs.Len() == 0 {
		return false
	}

	for k, v := range c.currentVar {
		if strings.Compare(k, vs.String()) == 0 {
			// found
			c.searchVar = false
			s.WriteString(v)
			vs.Reset()
			return true
		}
	}

	for i := len(c.vars) - 1; i >= 0; i-- {
		for k, v := range c.vars[i] {
			if strings.Compare(k, vs.String()) == 0 {
				s.WriteString(v)
				c.searchVar = false
				vs.Reset()
				return true
			}
		}
	}

	return false
}

func (c *Config) createBlock(s *bytes.Buffer) error {
	// fixed { be close to key like server{
	if c.searchKey && s.Len() > 0 {
		c.getElement(s.String())
		s.Reset()
		c.inSearchVal()
	}

	// vars
	vars := make(map[string]string)
	c.vars = append(c.vars, c.currentVar)
	c.currentVar = vars

	c.inBlock++
	//  slice or map?
	c.bkQueue = append(c.bkQueue, c.bkMulti)
	c.bkMulti = false
	if c.searchVal && s.Len() > 0 && c.current.Kind() == reflect.Map {
		c.bkMulti = true
		if c.current.IsNil() {
			c.current.Set(reflect.MakeMap(c.current.Type()))
		}
		var v reflect.Value
		v = reflect.New(c.current.Type().Key())
		c.pushElement(v)
		err := c.set(s.String())
		if err != nil {
			return err
		}
		c.mapKey = c.current
		c.popElement()
		val := reflect.New(c.current.Type().Elem())
		c.pushElement(val)
	}

	if c.current.Kind() == reflect.Slice {
		c.pushMultiBlock()
		n := c.current.Len()
		if c.current.Type().Elem().Kind() == reflect.Ptr {
			c.current.Set(reflect.Append(c.current, reflect.New(c.current.Type().Elem().Elem())))
		} else {
			c.current.Set(reflect.Append(c.current, reflect.Zero(c.current.Type().Elem())))
		}
		c.pushElement(c.current.Index(n))
	}
	c.inSearchKey()
	s.Reset()
	return nil
}

func (c *Config) closeBlock(s *bytes.Buffer) {
	if c.bkMulti {
		val := c.current
		c.popElement()
		if c.current.Kind() == reflect.Map {
			c.current.SetMapIndex(c.mapKey, val)
		}
	}
	c.popMultiBlock()

	// vars
	c.currentVar = c.vars[len(c.vars)-1]
	c.vars = c.vars[:len(c.vars)-1]

	c.inBlock--
	c.popElement()
	c.inSearchKey()
}

func (c *Config) inSearchKey() {
	c.searchVal = false
	c.searchKey = true
	c.canSkip = true
}

func (c *Config) inSearchVal() {
	c.searchKey = false
	c.searchVal = true
	c.canSkip = false
}

func (c *Config) getElement(s string) {
	s = strings.TrimSpace(s)
	if c.current.Kind() == reflect.Ptr {
		if c.current.IsNil() {
			c.current.Set(reflect.New(c.current.Type().Elem()))
		}
		c.current = c.current.Elem()
	}
	c.queue = append(c.queue, c.current)
	// 如果是驼峰，就要处理一下
	if c.camel {
		stmp := strings.Split(s, "_")
		buffer := bytes.Buffer{}
		for _, v := range stmp {
			buffer.WriteString(strings.Title(v))
		}
		c.current = c.current.FieldByName(buffer.String())
	}else{
		c.current = c.current.FieldByName(strings.Title(s))
	}
}

func (c *Config) pushElement(v reflect.Value) {
	c.queue = append(c.queue, c.current)
	c.current = v
}

func (c *Config) popElement() {
	c.current = c.queue[len(c.queue)-1]
	c.queue = c.queue[:len(c.queue)-1]
}

func (c *Config) delimiter(b byte) bool {
	return unicode.IsSpace(rune(b))
}

func (c *Config) pushMultiBlock() {
	c.bkQueue = append(c.bkQueue, c.bkMulti)
	c.bkMulti = true
}

func (c *Config) popMultiBlock() {
	c.bkMulti = c.bkQueue[len(c.bkQueue)-1]
	c.bkQueue = c.bkQueue[:len(c.bkQueue)-1]
}

func (c *Config) clearQuoted(s string) string {
	s = strings.TrimSpace(s)
	if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}

	return s
}

func (c *Config) splitQuoted(s string) ([]string, error) {
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
			if need_space && c.delimiter(ch) {
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
		return nil, errors.New(fmt.Sprintf("Invalid value: %v", s))
	}

	if vs.Len() > 0 {
		sq = append(sq, vs.String())
	}

	return sq, nil
}
