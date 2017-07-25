package config

import (
	"os"
	"log"
	"os/signal"
	"syscall"
	"bufio"
	"bytes"
	"reflect"
	"errors"
	"strings"
	"strconv"
	"fmt"
	"path/filepath"
)

type Config struct {
	filename string
	data interface{}
	queue []reflect.Value
	current reflect.Value
	searchVal bool
	searchKey bool
	inBlock int
	inInclude int
	canSkip bool
	skip bool
	bkMulti int
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
	this.data = v
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

func (this *Config) watch() {
	l := log.New(os.Stderr, "", 0)
	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)
	for {
		<-sighup
		l.Println("Caught SIGHUP, reloading config...")
		this.parse()
	}
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
			if b == ' ' || b == '\r' || b == '\n' || b == '\t' {
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
				if b == ' ' || b == '\r' || b == '\n' || b == '\t' {
					if !this.replace(&s, &vs) {
						s.Write(vsb.Bytes())
					}
				}
			}


			if this.searchVarBlock && b == '}' {
				this.searchVarBlock = false
			    // 判定是不是有值
				this.searchVar = false
				if !this.replace(&s, &vs) {
					vsb.WriteByte(b)
					s.Write(vsb.Bytes())
				}
				continue
			}

			if b == ';' {
				//	这里是要处理数据到this.current
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
		this.current.SetString(s)
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
		sf := strings.Fields(s)
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

		sf := strings.Fields(s)
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
	//	这里说明了可能是切片或者map
	if this.searchVal && s.Len() > 0 && this.current.Kind() == reflect.Map {
		this.bkMulti++
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
		this.bkMulti++
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
	if this.bkMulti > 0{
		this.bkMulti--
		val := this.current
		this.popElement()
		if this.current.Kind() == reflect.Map {
			this.current.SetMapIndex(this.mapKey, val)
		}
	}

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
