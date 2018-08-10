package graffiti

import (
	//"stash.hcom/run/istio-namespace-webhook/pkg/log"
	"encoding/json"
	"fmt"

	// "github.com/davecgh/go-spew/spew"

	admission "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	labels "k8s.io/apimachinery/pkg/labels"
	"stash.hcom/run/kube-graffiti/pkg/log"
)

const (
	componentName = "grafitti"
)

type BooleanOperator int

// BooleanOperator defines the logical boolean operator applied to label and field selector results.
// It is AND by default, i.e. both label selector and field selector must match to
const (
	AND BooleanOperator = iota
	OR
	XOR
)

type Selection struct {
	LabelSelectors  []string        `json:"label-selectors,omitempty"`
	FieldSelectors  []string        `json:"field-selectors,omitempty"`
	BooleanOperator BooleanOperator `json:"boolean-operator,omitempty"`
}

type Additions struct {
	Annotations map[string]string `json:"annotations,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type Rule struct {
	Selection Selection `json:"selection,omitempty"`
	Additions Additions `json:"additions`
}

// genericObject is used only for pulling out object metadata
type metaObject struct {
	Meta metav1.ObjectMeta `json:"metadata"`
}

func (r Rule) Mutate(req *admission.AdmissionRequest) *admission.AdmissionResponse {
	mylog := log.ComponentLogger(componentName, "Mutate")
	var (
		paintIt      = false
		labelMatches = false
		fieldMatches = false
		metaObject   metaObject
		err          error
	)

	if err := json.Unmarshal(req.Object.Raw, &metaObject); err != nil {
		mylog.Error().Err(err).Msg("failed to unmarshal generic object metadata from the admission request")
		return admissionResponseError(err)
	}

	if len(r.Selection.LabelSelectors) == 0 && len(r.Selection.FieldSelectors) == 0 {
		paintIt = true
	} else {
		// match against all of the label selectors
		labelMatches, err = r.matchLabelSelectors(metaObject)
		if err != nil {
			return admissionResponseError(err)
		}

		// test if we match any field selectors
		fieldMatches, err = r.matchFieldSelectors(req.Object.Raw)
		if err != nil {
			return admissionResponseError(err)
		}
	}

	// Combine selector booleans and decide to paint object or not
	if !paintIt {
		switch r.Selection.BooleanOperator {
		case AND:
			paintIt = (len(r.Selection.LabelSelectors) == 0 || labelMatches) && (len(r.Selection.FieldSelectors) == 0 || fieldMatches)
		case OR:
			paintIt = (len(r.Selection.LabelSelectors) == 0 || labelMatches) || (len(r.Selection.FieldSelectors) == 0 || fieldMatches)
		case XOR:
			paintIt = labelMatches != fieldMatches
		}
	}

	if !paintIt {
		mylog.Info().Str("name", metaObject.Meta.Name).Str("namespace", metaObject.Meta.Namespace).Msg("rules did not match, no modifications made")
		return admissionResponseError(fmt.Errorf("rules did not match, object not updated"))
	}

	return r.paintObject(metaObject)
}

func (r Rule) matchLabelSelectors(object metaObject) (bool, error) {
	mylog := log.ComponentLogger(componentName, "matchLabelSelectors")
	// test if we matched any of the label selectors
	if len(r.Selection.LabelSelectors) != 0 {
		// add name and namespace as labels so they can be matched with the label selector
		if len(object.Meta.Labels) == 0 {
			object.Meta.Labels = make(map[string]string)
		}
		object.Meta.Labels["name"] = object.Meta.Name
		object.Meta.Labels["namespace"] = object.Meta.Namespace

		for _, selector := range r.Selection.LabelSelectors {
			mylog.Debug().Str("label-selector", selector).Msg("testing label selector")
			selectorMatch, err := matchLabelSelector(selector, object.Meta.Labels)
			if err != nil {
				return false, err
			}
			if selectorMatch {
				mylog.Debug().Str("label-selector", selector).Msg("selector matches, will modify object")
				return true, nil
			}
		}
	}
	return false, nil
}

// matchSelector will apply a kubernetes labels.Selector to a map[string]string and return a matched bool and error.
func matchLabelSelector(selector string, target map[string]string) (bool, error) {
	mylog := log.ComponentLogger(componentName, "matchLabelSelector")
	selLog := mylog.With().Str("selector", selector).Logger()

	realSelector, err := labels.Parse(selector)
	if err != nil {
		selLog.Error().Err(err).Msg("could not parse selector")
		return false, err
	}

	set := labels.Set(target)
	if !realSelector.Matches(set) {
		selLog.Debug().Msg("selector does not match")
		return false, nil
	}
	selLog.Debug().Msg("selector matches")
	return true, nil
}

func (r Rule) matchFieldSelectors(raw []byte) (bool, error) {
	mylog := log.ComponentLogger(componentName, "matchFieldSelectors")
	if len(r.Selection.FieldSelectors) != 0 {
		fieldMap, err := makeFieldMap(raw)
		if err != nil {
			return false, err
		}

		for _, selector := range r.Selection.FieldSelectors {
			mylog.Debug().Str("field-selector", selector).Msg("testing field selector")
			selectorMatch, err := matchFieldSelector(selector, fieldMap)
			if err != nil {
				return false, err
			}
			if selectorMatch {
				mylog.Debug().Str("field-selector", selector).Msg("selector matches, will modify object")
				return true, nil
			}
		}
	}
	return false, nil
}

// matchSelector will apply a kubernetes labels.Selector to a map[string]string and return a matched bool and error.
func matchFieldSelector(selector string, target map[string]string) (bool, error) {
	mylog := log.ComponentLogger(componentName, "matchFieldSelector")
	selLog := mylog.With().Str("selector", selector).Logger()
	realSelector, err := fields.ParseSelector(selector)
	if err != nil {
		selLog.Error().Err(err).Msg("could not parse selector")
		return false, err
	}

	set := labels.Set(target)
	if !realSelector.Matches(set) {
		selLog.Debug().Msg("selector does not match")
		return false, nil
	}
	selLog.Debug().Msg("selector matches")
	return true, nil
}

func admissionResponseError(err error) *admission.AdmissionResponse {
	return &admission.AdmissionResponse{
		Allowed: true,
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
}

func (r Rule) paintObject(object metaObject) *admission.AdmissionResponse {
	mylog := log.ComponentLogger(componentName, "paintObject")
	reviewResponse := admission.AdmissionResponse{}
	reviewResponse.Allowed = true

	if len(r.Additions.Labels) == 0 && len(r.Additions.Annotations) == 0 {
		return admissionResponseError(fmt.Errorf("rule does contain any labels or annotations to add"))
	}
	patch, err := r.createObjectPatch(object)
	if err != nil {
		return admissionResponseError(fmt.Errorf("could not create the json patch"))
	}
	mylog.Debug().Str("patch", string(patch)).Msg("created json patch")
	reviewResponse.Patch = patch
	pt := admission.PatchTypeJSONPatch
	reviewResponse.PatchType = &pt
	return &reviewResponse
}
