package config

import (
	"os"
	"reflect"
)

type stash struct {
	file           *os.File
	line           int64
	searchVal      bool
	searchKey      bool
	inBlock        int
	canSkip        bool
	skip           bool
	bkQueue        []bool
	bkMulti        bool
	mapKey         reflect.Value
	setVar         bool
	searchVar      bool
	searchVarBlock bool
	cwd            string
	vars           []map[string]string
}

func (cfg *Config) popStash() error {
	if cfg.inBlock > 0 {
		return cfg.error("invalid config file, block not closed by \"}\"")
	}

	if len(cfg.stash) > 0 {
		current := cfg.stash[len(cfg.stash)-1]
		cfg.stash = cfg.stash[:len(cfg.stash)-1]
		cfg.file = current.file
		cfg.line = current.line
		cfg.searchVal = current.searchVal
		cfg.searchKey = current.searchKey
		cfg.inBlock = current.inBlock
		cfg.canSkip = current.canSkip
		cfg.skip = current.skip
		cfg.bkMulti = current.bkMulti
		cfg.mapKey = current.mapKey
		cfg.setVar = current.setVar
		cfg.searchVar = current.searchVar
		cfg.searchVarBlock = current.searchVarBlock
		cfg.cwd = current.cwd

		cfg.bkQueue = make([]bool, len(current.bkQueue))
		cfg.vars = make([]map[string]string, len(current.vars))
		copy(cfg.bkQueue, current.bkQueue)
		copy(cfg.vars, current.vars)
	}

	return nil
}

func (cfg *Config) pushStash() {
	s := &stash{
		file:           cfg.file,
		line:           cfg.line,
		searchVal:      cfg.searchVal,
		searchKey:      cfg.searchKey,
		inBlock:        cfg.inBlock,
		canSkip:        cfg.canSkip,
		skip:           cfg.skip,
		bkMulti:        cfg.bkMulti,
		mapKey:         cfg.mapKey,
		setVar:         cfg.setVar,
		searchVar:      cfg.searchVar,
		searchVarBlock: cfg.searchVarBlock,
		cwd:            cfg.cwd,
	}

	s.bkQueue = make([]bool, len(cfg.bkQueue))
	s.vars = make([]map[string]string, len(cfg.vars))
	copy(s.bkQueue, cfg.bkQueue)
	copy(s.vars, cfg.vars)

	cfg.inSearchKey()
	cfg.inBlock = 0
	cfg.inInclude = 0
	cfg.currentVar = make(map[string]string)
	cfg.searchVar = false
	cfg.setVar = false
	cfg.searchVarBlock = false

	cfg.stash = append(cfg.stash, s)
}
