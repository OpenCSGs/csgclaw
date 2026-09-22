package modelprovider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestModelDiscoveryRetainsCapacity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"vision","context_length":64000,"architecture":{"input_modalities":["text","image"]}},{"id":"text","context_window":8192,"input_modalities":["text"]},{"id":"unknown"}]}`)
	}))
	defer server.Close()
	got, err := ListOpenAIModelDirectoryWithClient(context.Background(), server.Client(), server.URL, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.ModelMetadata["vision"].ContextWindow != 64000 || got.ModelMetadata["text"].ContextWindow != 8192 || got.ModelMetadata["unknown"].ContextWindow != 0 {
		t.Fatalf("%+v", got.ModelMetadata)
	}
}
