package inspect

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

func Convert(i1 interface{}, i2 interface{}) error {
	b, err := json.Marshal(i1)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, i2)
}

func ConvertJSON(b []byte) (map[string]interface{}, error) {
	response := make(map[string]interface{})
	if len(b) != 0 {
		if err := json.Unmarshal(b, &response); err != nil {
			return nil, err
		}
	}
	return response, nil
}

// Source will copy values from settings to the given interface for
// all fields that are setup with json serialization in i.
// It's a shallow copy and i needs to be a struct or a pointer to a struct.
func Source(i interface{}, settings map[string]interface{}) error {
	v := reflect.ValueOf(i)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return errors.New("can only source structs")
	}

	t := v.Type()
	for j := 0; j < t.NumField(); j++ {
		f := t.Field(j)
		s, ok := f.Tag.Lookup("json")
		if !ok {
			continue
		}
		p := strings.Index(s, ",")
		if p > -1 {
			s = s[:p]
		}
		setting, ok := settings[s]
		if !ok {
			continue
		}
		sv := reflect.ValueOf(setting)
		switch sv.Kind() {
		case reflect.Slice, reflect.Array:
			av, e := toSpecificArray(sv, f.Type)
			if e != nil {
				return e
			}
			v.FieldByName(f.Name).Set(av)
		default:
			if !sv.Type().AssignableTo(f.Type) {
				return fmt.Errorf("found unassignable type at %s, expected %v but found %v", f.Name, f.Type, sv.Type())
			}
			v.FieldByName(f.Name).Set(sv)
		}
	}
	return nil
}

// Merge combines two maps. Keys in the second map override the ones in the first. Arrays are merged.
func Merge(a, b map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, av := range a {
		ar := reflect.ValueOf(av)
		if avm, ok := av.(map[string]interface{}); ok {
			if bv, ok := b[k]; ok {
				if bvm, ok := bv.(map[string]interface{}); ok {
					result[k] = Merge(avm, bvm)
				} else {
					result[k] = bv
				}
			} else {
				result[k] = av
			}
		} else if ar.Kind() == reflect.Slice {
			if bv, ok := b[k]; ok {
				br := reflect.ValueOf(bv)
				if br.Kind() == reflect.Slice {
					combined := reflect.MakeSlice(ar.Type(), 0, ar.Len())
					for i := 0; i < ar.Len(); i++ {
						combined = reflect.Append(combined, ar.Index(i))
					}
					for i := 0; i < br.Len(); i++ {
						combined = reflect.Append(combined, br.Index(i))
					}
					result[k] = combined.Interface()
				} else {
					result[k] = bv
				}
			}
		} else {
			if bv, ok := b[k]; ok {
				result[k] = bv
			} else {
				result[k] = av
			}
		}
	}
	for k, bv := range b {
		if _, ok := a[k]; !ok {
			result[k] = bv
		}
	}
	return result
}

// toSpecificArray converts an array of one type to an array of a desired type if it's assignable.
func toSpecificArray(array reflect.Value, target reflect.Type) (reflect.Value, error) {
	result := reflect.MakeSlice(reflect.SliceOf(target.Elem()), 0, array.Cap())
	for i := 0; i < array.Len(); i++ {
		v := array.Index(i)
		// TODO: Fix the case when v is a struct, like for customResources in an account config
		if v.Kind() == reflect.Interface || v.Kind() == reflect.Ptr {
			v = v.Elem()
		}
		if !v.Type().AssignableTo(target.Elem()) {
			return reflect.Value{}, fmt.Errorf("found unassignable type, expected %v but found %v", target.Elem(), v.Type())
		}
		result = reflect.Append(result, v)
	}
	return result, nil
}

type stringHandler func(val string) (string, error)

func InspectStrings(i interface{}, handler stringHandler) (interface{}, error) {
	t, err := inspectStringReflect(reflect.ValueOf(i), handler)
	return t.Interface(), err
}

func inspectStringReflect(v reflect.Value, stringHandler stringHandler) (reflect.Value, error) {
	switch v.Kind() {
	case reflect.Ptr:
		rv, err := inspectStringReflect(v.Elem(), stringHandler)
		if err != nil {
			return v, err
		}
		eV := reflect.New(v.Elem().Type())
		eV.Elem().Set(rv)
		return eV, nil
	case reflect.Struct:
		nsv := reflect.New(v.Type())
		for j := 0; j < v.NumField(); j++ {
			f := v.Field(j)
			rv, err := inspectStringReflect(f, stringHandler)
			if err != nil {
				return v, err
			}
			// Replace in the new struct
			nf := nsv.Elem().Field(j)
			if nf.CanAddr() {
				nf.Set(rv)
			} else {
				return v, fmt.Errorf("unaddressable value found %v", nf)
			}
		}
		return nsv.Elem(), nil
	case reflect.String:
		s, err := stringHandler(v.String())
		if err != nil {
			return v, err
		}
		return reflect.ValueOf(s), nil
	case reflect.Slice, reflect.Array:
		if v.Len() == 0 {
			return v, nil
		}
		nsv := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for j := 0; j < v.Len(); j++ {
			rv, err := inspectStringReflect(v.Index(j), stringHandler)
			if err != nil {
				return v, err
			}
			nsv.Index(j).Set(rv)
		}
		return nsv, nil
	case reflect.Map:
		nmv := reflect.MakeMap(v.Type())
		keys := v.MapKeys()
		for _, k := range keys {
			rv, err := inspectStringReflect(v.MapIndex(k), stringHandler)
			if err != nil {
				return v, err
			}
			nmv.SetMapIndex(k, rv)
		}
		return nmv, nil
	case reflect.Interface:
		return inspectStringReflect(v.Elem(), stringHandler)
	}
	return v, nil
}
