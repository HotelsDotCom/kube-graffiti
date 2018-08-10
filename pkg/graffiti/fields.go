package graffiti

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

	"stash.hcom/run/kube-graffiti/pkg/log"
)

// makeFieldMap converts a raw json object into a compatible field map
func makeFieldMap(raw []byte) (map[string]string, error) {
	mylog := log.ComponentLogger(componentName, "makeFieldMap")
	fieldMap := make(map[string]string)
	var jsonObject map[string]interface{}

	if len(raw) == 0 {
		mylog.Error().Msg("object is empty, can't convert to fields")
		return fieldMap, fmt.Errorf("no fields found")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	err := d.Decode(&jsonObject)
	if err != nil {
		return fieldMap, fmt.Errorf("failed to unmarshal object: %v", err)
	}
	for k, v := range jsonObject {
		addFieldRecursive(fieldMap, "", k, v)
	}

	return fieldMap, nil
}

func addFieldRecursive(fm map[string]string, prefix, k string, v interface{}) {
	mylog := log.ComponentLogger(componentName, "addFieldRecursive")
	reclog := mylog.With().Str("prefix", prefix).Logger()

	if reflect.ValueOf(k).Kind() != reflect.String {
		reclog.Debug().Msg("key is not a string")
		return
	}

	if reflect.TypeOf(v) == nil {
		reclog.Debug().Str("key", k).Str("value", "").Msg("adding empty value to fieldmap")
		fm[prefix+k] = ""
		return
	}

	if reflect.TypeOf(v).String() == "json.Number" {
		reclog.Debug().Str("key", k).Str("value", v.(json.Number).String()).Msg("adding json number to fieldmap")
		fm[prefix+k] = v.(json.Number).String()
		return
	}

	switch reflect.ValueOf(v).Kind() {
	case reflect.String:
		reclog.Debug().Str("key", k).Str("value", v.(string)).Msg("adding string to fieldmap")
		fm[prefix+k] = v.(string)
		return
	case reflect.Bool:
		reclog.Debug().Str("key", k).Bool("value", v.(bool)).Msg("adding bool to fieldmap")
		fm[prefix+k] = strconv.FormatBool(v.(bool))
		return
	case reflect.Slice:
		reclog.Debug().Str("key", k).Msg("adding slice to fieldmap")
		for i, val := range v.([]interface{}) {
			addFieldRecursive(fm, prefix+k+".", strconv.Itoa(i), val)
		}
	case reflect.Map:
		reclog.Debug().Str("key", k).Msg("adding map to fieldmap")
		for x, y := range v.(map[string]interface{}) {
			addFieldRecursive(fm, prefix+k+".", x, y)
		}
	default:
		reclog.Debug().Str("key", k).Str("kind", reflect.ValueOf(v).Kind().String()).Msg("can't handle kind")
	}
	return
}
