package agentsdk

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIErrorRetainsHTTPStatus(t *testing.T) {
	for _, status := range []int{404, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status); _, _ = w.Write([]byte("failure")) }))
			defer server.Close()
			client := newAirlockClient(server.URL, "test", server.Client())
			err := client.doJSON(context.Background(), "GET", "/missing", nil, nil)
			var api *APIError
			if !errors.As(err, &api) || api.StatusCode != status {
				t.Fatalf("error=%#v", err)
			}
			if err.Error() != fmt.Sprintf("GET /missing: status %d: failure", status) {
				t.Fatal("error text changed")
			}
		})
	}
}
