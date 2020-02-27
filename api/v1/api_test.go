package v1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-kit/kit/log"

	"github.com/prometheus/pushgateway/storage"
)

var (
	logger    = log.NewNopLogger()
	testFlags = map[string]string{
		"flag1": "value1",
		"flag2": "value2",
		"flag3": "value3",
	}
	testBuildInfo = map[string]string{
		"build1": "value1",
		"build2": "value2",
		"build3": "value3",
	}
)

func compareMaps(mainMap map[string]interface{}, testMap map[string]string) bool {
	if len(mainMap) != len(testMap) {
		return false
	}

	for key, _ := range mainMap {
		if mainMap[key].(string) != testMap[key] {
			return false
		}
	}
	return true
}

func TestStatusAPI(t *testing.T) {
	dms := storage.NewDiskMetricStore("", 100*time.Millisecond, nil, logger)
	TestApi := New(logger, dms, testFlags, testBuildInfo)

	req, err := http.NewRequest("GET", "http://example.org/", &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()

	testResponse := response{}
	TestApi.status(w, req)
	json.Unmarshal(w.Body.Bytes(), &testResponse)
	jsonData := testResponse.Data.(map[string]interface{})
	responseFlagData := jsonData["flags"].(map[string]interface{})
	responseBuildInfo := jsonData["build_information"].(map[string]interface{})

	if expected, got := http.StatusOK, w.Code; expected != got {
		t.Errorf("Wanted status code %v, got %v.", expected, got)
	}

	if !compareMaps(responseFlagData, testFlags) {
		t.Errorf("Wanted following flags %v, got %v.", testFlags, responseFlagData)
	}

	if !compareMaps(responseBuildInfo, testBuildInfo) {
		t.Errorf("Wanted following build info %v, got %v.", testBuildInfo, responseBuildInfo)
	}
}
