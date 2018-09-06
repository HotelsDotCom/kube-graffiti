package graffiti

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"text/template"
)

// createJSONPatch will generate a JSON patch for replacing an objects labels and/or annotations
// It is designed to replace the whole path in order to work around a bug in kubernetes that does not correctly
// unescape ~1 (/) in paths preventing annotation labels with slashes in them.
func (r Rule) createObjectPatch(obj metaObject, fm map[string]string) (string, error) {
	var patches []string

	if len(r.Additions.Labels) > 0 {
		op, err := createPatchOperand(obj.Meta.Labels, r.Additions.Labels, fm, "/metadata/labels")
		if err != nil {
			return "", err
		}
		if op != "" {
			patches = append(patches, op)
		}
	}

	if len(r.Additions.Annotations) > 0 {
		op, err := createPatchOperand(obj.Meta.Annotations, r.Additions.Annotations, fm, "/metadata/annotations")
		if err != nil {
			return "", err
		}
		if op != "" {
			patches = append(patches, op)
		}
	}

	if len(patches) == 0 {
		return "", nil
	}
	return `[ ` + strings.Join(patches, ", ") + ` ]`, nil
}

func createPatchOperand(src, additions, fm map[string]string, path string) (string, error) {
	if len(additions) == 0 {
		return "", nil
	}

	rendered, err := renderMapValues(additions, fm)
	if err != nil {
		return "", err
	}

	modified := mergeMaps(src, rendered)
	// don't produce a patch when there are no changes
	if reflect.DeepEqual(src, modified) {
		return "", nil
	}

	if len(src) == 0 {
		return renderStringMapAsPatch("add", path, modified), nil
	}
	return renderStringMapAsPatch("replace", path, modified), nil
}

// renderStringMapAsPatch builds a json patch string from operand, path and a map
func renderStringMapAsPatch(op, path string, m map[string]string) string {
	patch := `{ "op": "` + op + `", "path": "` + path + `", "value": { `
	var values []string
	for k, v := range m {
		values = append(values, `"`+k+`": "`+escapeString(v)+`"`)
	}
	patch = patch + strings.Join(values, ", ") + ` }}`
	return patch
}

func escapeString(s string) string {
	result := strings.Replace(s, "\n", "", -1)
	return strings.Replace(result, `"`, `\"`, -1)
}

func mergeMaps(sources ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, source := range sources {
		for k, v := range source {
			result[k] = v
		}
	}

	return result
}

// renderMapValues - treat each map value as a template and render it using the data map as a context
func renderMapValues(src, data map[string]string) (map[string]string, error) {
	result := make(map[string]string)
	for k, v := range src {
		if rendered, err := renderStringTemplate(v, data); err != nil {
			return result, err
		} else {
			result[k] = rendered
		}
	}
	return result, nil
}

// renderStringTemplate will treat the input string as a template and render with data as its context
// useful for allowing dynamically created values.
func renderStringTemplate(field string, data interface{}) (string, error) {
	tmpl, err := template.New("field").Parse(field)
	if err != nil {
		return "", fmt.Errorf("failed to parse field template: %v", err)
	}

	var b bytes.Buffer
	err = tmpl.Execute(&b, data)
	if err != nil {
		return "", fmt.Errorf("error rendering template: %v", err)
	}
	return b.String(), nil
}
