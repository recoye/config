package config

import (
	"os"
	"bufio"
	"bytes"
	"reflect"
	"errors"
	"strings"
	"strconv"
	"fmt"
	"path/filepath"
	"unicode"
)

type IConfigLog interface {

}

type Config struct {
	filename string
	queue []reflect.Value
	current reflect.Value
	searchVal bool
	searchKey bool
	inBlock int
	inInclude int
	canSkip bool
	skip bool
	bkQueue []bool
	bkMulti bool
	mapKey reflect.Value
	setVar bool
	searchVar bool
	searchVarBlock bool
	vars []map[string]string
	currentVar map[string]string
}

func New(filename string) *Config {
	return &Config{filename:filename}
}

func (this *Config) Unmarshal(v interface{}) error{
	rev := reflect.ValueOf(v)
	if rev.Kind() != reflect.Ptr {
		err := errors.New("non-pointer passed to Unmarshal")
		return err
	}
	this.current = rev.Elem()
	this.inSearchKey()
	this.inBlock = 0
	this.inInclude = 0
	this.currentVar = make(map[string]string)
	this.searchVar = false
	this.setVar = false
	this.searchVarBlock = false
	return this.parse()
}

func (this *Config) Reload() error {
	return this.parse()
}

func (this *Config) parse() error {
	var err error
	var s bytes.Buffer
	var vs bytes.Buffer
	var vsb bytes.Buffer
	if _, err = os.Stat(this.filename);os.IsNotExist(err) {
		return err
	}

	var fp *os.File
	fp, err = os.Open(this.filename)
	if err != nil {
		return err
	}
	defer fp.Close()

	reader := bufio.NewReader(fp)
	var b byte
	for err == nil {
		b, err = reader.ReadByte()
		if err == bufio.ErrBufferFull {
			return nil
		}

		if this.canSkip && b == '#' {
			reader.ReadLine()
			continue
		}
		if this.canSkip && b == '/' {
			if this.skip {
				reader.ReadLine()
				this.skip = false
				continue
			}
			this.skip = true
			continue
		}
		if this.searchKey {
			if this.delimiter(b) {
				if s.Len() > 0 {
					this.inSearchVal()
					if strings.Compare(s.String(), "include") == 0 {
						s.Reset()
						this.inInclude++
						if this.inInclude > 100 {
							return errors.New("too many include, exceeds 100 limit!")
						}
						continue
					}else if strings.Compare(s.String(), "set") == 0 {
						s.Reset()
						this.setVar = true
						continue
					}
					this.getElement(s.String())
					s.Reset()
				}
				continue
			}
		}

		if b == '{' && !this.searchVar && vs.Len() == 0 {
			if err := this.createBlock(&s); err != nil {
				return err
			}
			continue
		}

		if this.searchKey && b == '}' && this.inBlock > 0 {
			this.closeBlock(&s)
			continue
		}

		if this.searchVal {
			if b == '$' {
				this.searchVar = true
				vs.Reset()
				vsb.Reset()
				vsb.WriteByte(b)
				continue;
			}

			if this.searchVar {
				if b == '{' {
					if vs.Len() == 0 {
						this.searchVarBlock = true
						vsb.WriteByte(b)
						continue
					}
					if !this.searchVarBlock {
						if !this.replace(&s, &vs) {
							s.Write(vsb.Bytes())
						}

						// is block?
						if err := this.createBlock(&s);err != nil {
							return err
						}else{
							continue
						}
					}
				} // if b == '{'
				// Is space
				if this.delimiter(b) {
					if !this.replace(&s, &vs) {
						s.Write(vsb.Bytes())
					}
				}
			}


			if this.searchVarBlock && b == '}' {
				this.searchVarBlock = false
			    // replace $???
				this.searchVar = false
				if !this.replace(&s, &vs) {
					vsb.WriteByte(b)
					s.Write(vsb.Bytes())
				}
				continue
			}

			if b == ';' {
				//	copy to this.current
				this.inSearchKey()
				if this.searchVar {
					if this.searchVarBlock {
						return errors.New(vsb.String() + " is not terminated by }")
					}
					this.searchVar = false
					this.searchVarBlock = false
					if !this.replace(&s, &vs) {
						s.Write(vsb.Bytes())
					}
				}

				// set to map
				if this.setVar {
					sf := strings.Fields(s.String())
					if len(sf) != 2 {
						return errors.New("Invalid Config")
					}
					this.currentVar[sf[0]] = sf[1]
					this.setVar = false
					s.Reset()
					continue
				}

				if this.inInclude > 0 {
					this.filename = strings.TrimSpace(s.String())
					s.Reset()
					this.inInclude--
					files, err := filepath.Glob(this.filename)
					if err != nil{
						return err
					}
					for _,file := range files {
						this.filename = file
						if err := this.parse(); err != nil {
							return err
						}
					}
					continue
				}

				err := this.set(s.String())
				if err != nil {
					return err
				}

				s.Reset()
				this.popElement()
				continue
			}else if(this.searchVar) { // if b == ';'
				vs.WriteByte(b)
				vsb.WriteByte(b)
				if ! this.searchVarBlock{
					this.replace(&s, &vs)
				}
				continue
			}
		}

		s.WriteByte(b)
	}

	if !this.searchKey && this.inBlock > 0 {
		return errors.New("Invalid config file!")
	}

	return nil
}

func (this *Config) set(s string) error {
	s = strings.TrimSpace(s)
	if this.current.Kind() == reflect.Ptr {
		if this.current.IsNil() {
			this.current.Set(reflect.New(this.current.Type().Elem()))
		}
		this.current = this.current.Elem()
	}

	switch this.current.Kind() {
	case reflect.String:
		this.current.SetString(this.clearQuoted(s))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		itmp,err := strconv.ParseInt(s, 10, this.current.Type().Bits())
		if  err != nil {
			return err
		}
		this.current.SetInt(itmp)
	case  reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		itmp,err := strconv.ParseUint(s, 10, this.current.Type().Bits())
		if  err != nil {
			return err
		}
		this.current.SetUint(itmp)
	case reflect.Float32, reflect.Float64:
		ftmp, err := strconv.ParseFloat(s, this.current.Type().Bits())
		if err != nil {
			return err
		}
		this.current.SetFloat(ftmp)
	case reflect.Bool:
		if s == "yes" || s == "on" {
			this.current.SetBool(true)
		}else if s == "no" || s == "off" {
			this.current.SetBool(false)
		}else{
			btmp, err := strconv.ParseBool(s)
			if err != nil {
				return err
			}
			this.current.SetBool(btmp)
		}
	case reflect.Slice:
		sf, err := this.splitQuoted(s)
		if err != nil {
			return err
		}
		for _,sv := range sf {
			n := this.current.Len()
			this.current.Set(reflect.Append(this.current, reflect.Zero(this.current.Type().Elem())))
			this.pushElement(this.current.Index(n))
			this.set(sv)
			this.popElement()
		}
	case reflect.Map:
		if this.current.IsNil() {
			this.current.Set(reflect.MakeMap(this.current.Type()))
		}

		sf, err := this.splitQuoted(s)
		if err != nil {
			return err
		}
		if len(sf) != 2 {
			return errors.New("Invalid Config")
		}
		var v reflect.Value
		v=reflect.New(this.current.Type().Key())
		this.pushElement(v)
		this.set(sf[0])
		key := this.current
		this.popElement()
		v = reflect.New(this.current.Type().Elem())
		this.pushElement(v)
		this.set(sf[1])
		val := this.current
		this.popElement()

		this.current.SetMapIndex(key, val)
	default:
		return errors.New(fmt.Sprintf("Invalid Type:%s", this.current.Kind()))
	}
	return nil
}

func (this *Config) replace(s *bytes.Buffer, vs *bytes.Buffer) bool {
	if vs.Len() == 0 {
		return false
	}

	for k,v := range this.currentVar{
		if strings.Compare(k, vs.String()) == 0 {
			// found
			this.searchVar = false
			s.WriteString(v)
			vs.Reset()
			return true
		}
	}

	for i:= len(this.vars) - 1; i >= 0 ;i-- {
		for k, v := range this.vars[i] {
			if strings.Compare(k, vs.String()) == 0 {
				s.WriteString(v)
				this.searchVar = false
				vs.Reset()
				return true
			}
		}
	}

	return false
}

func (this *Config) createBlock(s *bytes.Buffer) error {
	// fixed { be close to key like server{
	if this.searchKey && s.Len() > 0 {
		this.getElement(s.String())
		s.Reset()
		this.inSearchVal()
	}

	// vars
	vars := make(map[string]string)
	this.vars = append(this.vars, this.currentVar)
	this.currentVar = vars

	this.inBlock++
	//	slice or map?
	this.bkQueue = append(this.bkQueue, this.bkMulti)
	this.bkMulti = false
	if this.searchVal && s.Len() > 0 && this.current.Kind() == reflect.Map {
		this.bkMulti = true
		if this.current.IsNil() {
			this.current.Set(reflect.MakeMap(this.current.Type()))
		}
		var v reflect.Value
		v = reflect.New(this.current.Type().Key())
		this.pushElement(v)
		err := this.set(s.String())
		if err != nil {
			return err
		}
		this.mapKey = this.current
		this.popElement()
		val := reflect.New(this.current.Type().Elem())
		this.pushElement(val)
	}

	if this.current.Kind() == reflect.Slice {
		this.pushMultiBlock()
		n := this.current.Len()
		if this.current.Type().Elem().Kind() == reflect.Ptr {
			this.current.Set(reflect.Append(this.current, reflect.New(this.current.Type().Elem().Elem())))
		}else{
			this.current.Set(reflect.Append(this.current, reflect.Zero(this.current.Type().Elem())))
		}
		this.pushElement(this.current.Index(n))
	}
	this.inSearchKey()
	s.Reset()
	return nil
}

func (this *Config) closeBlock(s *bytes.Buffer) {
	if this.bkMulti{
		val := this.current
		this.popElement()
		if this.current.Kind() == reflect.Map {
			this.current.SetMapIndex(this.mapKey, val)
		}
	}
	this.popMultiBlock()

	// vars
	this.currentVar = this.vars[len(this.vars) - 1]
	this.vars = this.vars[:len(this.vars) - 1]

	this.inBlock--
	this.popElement()
	this.inSearchKey()
}

func (this *Config) inSearchKey() {
	this.searchVal = false
	this.searchKey = true
	this.canSkip = true
}

func (this *Config) inSearchVal() {
	this.searchKey = false
	this.searchVal = true
	this.canSkip = false
}

func (this *Config) getElement(s string){
	s = strings.TrimSpace(s)
	if this.current.Kind() == reflect.Ptr {
		if this.current.IsNil() {
			this.current.Set(reflect.New(this.current.Type().Elem()))
		}
		this.current = this.current.Elem()
	}
	this.queue = append(this.queue, this.current)
	this.current = this.current.FieldByName(strings.Title(s))
}

func (this *Config) pushElement(v reflect.Value) {
	this.queue = append(this.queue, this.current)
	this.current = v
}

func (this *Config) popElement() {
	this.current = this.queue[len(this.queue) - 1]
	this.queue = this.queue[:len(this.queue) - 1]
}

func (this *Config) delimiter(b byte) bool {
	return unicode.IsSpace(rune(b))
}

func (this *Config) pushMultiBlock() {
	this.bkQueue = append(this.bkQueue, this.bkMulti)
	this.bkMulti = true
}

func (this *Config) popMultiBlock() {
	this.bkMulti = this.bkQueue[len(this.bkQueue) - 1]
	this.bkQueue = this.bkQueue[:len(this.bkQueue) - 1]
}

func (this *Config) clearQuoted(s string) string {
	s = strings.TrimSpace(s)
	if (s[0] == '"' && s[len(s) - 1] == '"') || (s[0] == '\'' && s[len(s) - 1] == '\'') {
		return s[1:len(s) - 1]
	}

	return s
}

func (this *Config) splitQuoted(s string) ([]string, error){
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
		}else {
			if need_space && this.delimiter(ch)  {
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
			}else if s_quote {
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