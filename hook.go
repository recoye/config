package config

import "reflect"

type hook struct {
	*Config
}

func (hk *hook) Unique(s string) error {
	parent, b := hk.getNearestSlice()
	if !b {
		return hk.error("can't not hook unique on \"%s\" directive, unique must on slice with struct", hk.typ.Name)
	}

	if parent.Kind() != reflect.Slice {
		return hk.error("unique only can be hook in struct of slice")
	}

	// last is current
	for i := 0; i < parent.Len()-1; i++ {
		cur := parent.Index(i).FieldByName(hk.typ.Name)
		if !cur.CanInterface() {
			return hk.error("can't be interface at parent")
		}
		if cur.Interface() == hk.current.Interface() {
			return hk.error("duplication value \"%s\" for %s", s, hk.typ.Name)
		}
	}

	return nil
}
