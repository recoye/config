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
					}
					this.getElement(s.String())
					s.Reset()
				}
				continue
			}
		}

		if b == '{' {
			// fixed { be close to key like server{
			if this.searchKey && s.Len() > 0 {
				this.getElement(s.String())
				s.Reset()
				this.inSearchVal()
			}

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
				this.current.Set(reflect.Append(this.current, reflect.Zero(this.current.Type().Elem())))
				this.pushElement(this.current.Index(n))
			}
			this.inSearchKey()
			s.Reset()
			continue
		}

		if b == '}' && this.inBlock > 0 {
			if this.bkMulti > 0{
				this.bkMulti--
				val := this.current
				this.popElement()
				if this.current.Kind() == reflect.Map {
					this.current.SetMapIndex(this.mapKey, val)
				}
			}
			this.inBlock--
			this.popElement()
			this.inSearchKey()
			continue
		}

		if this.searchVal {
			if b == ';' {
				//	这里是要处理数据到this.current
				this.inSearchKey()

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
				cs := s.String()
				log.Println(cs)
				if err != nil {
					return err
				}

				s.Reset()
				this.popElement()
				continue
			}

		}

		s.WriteByte(b)
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
		if s == "yes" {
			this.current.SetBool(true)
		}else if s == "no" {
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
