package environment

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestServiceRejectsAnyPublicPort(t *testing.T) {
	for name, body := range map[string]string{
		"value":       `{"ports":{"http":{"port":80}},"publicPort":"http"}`,
		"empty":       `{"ports":{"http":{"port":80}},"publicPort":""}`,
		"null":        `{"ports":{"http":{"port":80}},"publicPort":null}`,
		"with routes": `{"ports":{"http":{"port":80}},"publicPort":"http","publicRoutes":[{"path":"/api","port":"http"}]}`,
	} {
		var service Service
		if err := json.Unmarshal([]byte(body), &service); !errors.Is(err, ErrPublicPortRemoved) {
			t.Errorf("%s: want ErrPublicPortRemoved, got %v", name, err)
		}
		var doc Document
		if err := json.Unmarshal([]byte(`{"modules":{"web":{"profile":"p","spec":{"containers":{},"service":`+body+`}}}}`), &doc); !errors.Is(err, ErrPublicPortRemoved) {
			t.Errorf("%s: Document decode must fail, got %v", name, err)
		}
	}
}

func TestServiceKeepsMultiplePublicRoutes(t *testing.T) {
	var service Service
	body := `{"ports":{"http":{"port":80}},"publicRoutes":[{"path":"/","port":"http"},{"path":"/api","port":"http"}]}`
	if err := json.Unmarshal([]byte(body), &service); err != nil || len(service.PublicRoutes) != 2 {
		t.Fatalf("routes lost: %+v, %v", service, err)
	}
	if out, _ := json.Marshal(service); string(out) == "" || json.Valid(out) == false {
		t.Fatal("service does not re-encode")
	}
}

func TestServiceDecodingIsStrictAtEveryDepth(t *testing.T) {
	for name, body := range map[string]string{
		"service key": `{"ports":{"http":{"port":80}},"extra":1}`,
		"port key":    `{"ports":{"http":{"port":80,"bogus":1}}}`,
		"route key":   `{"ports":{"http":{"port":80}},"publicRoutes":[{"path":"/","port":"http","bogus":1}]}`,
	} {
		var service Service
		if err := json.Unmarshal([]byte(body), &service); err == nil || errors.Is(err, ErrPublicPortRemoved) {
			t.Errorf("%s: want unknown-field error, got %v", name, err)
		}
	}
}
