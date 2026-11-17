package modelprovider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"csgclaw/internal/modelcap"
)

func TestModelDiscoveryRetainsCapacity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[
			{"id":"vision","context_length":64000,"architecture":{"input_modalities":["text","image"]}},
			{"id":"text","context_window":8192,"max_output_tokens":2048,"input_modalities":["text"]},
			{"id":"deepseek-flash","context_window":1048576,"max_output_tokens":393216},
			{"id":"output-only","max_output_tokens":16384},
			{"id":"invalid-output","context_window":8000,"max_output_tokens":"32K"},
			{"id":"unknown"}
		]}`)
	}))
	defer server.Close()
	got, err := ListOpenAIModelDirectoryWithClient(context.Background(), server.Client(), server.URL, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	for model, want := range map[string]modelcap.Metadata{
		"vision":         {ContextWindow: 64000},
		"text":           {ContextWindow: 8192, MaxOutputTokens: 2048},
		"deepseek-flash": {ContextWindow: 1_048_576, MaxOutputTokens: 393_216},
		"output-only":    {MaxOutputTokens: 16_384},
		"invalid-output": {ContextWindow: 8000},
		"unknown":        {},
	} {
		if got.ModelMetadata[model] != want {
			t.Errorf("%s metadata = %+v, want %+v", model, got.ModelMetadata[model], want)
		}
	}
}
