package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/AndrewLester/ntpal/internal/templates"
	"github.com/AndrewLester/ntpal/pkg/ntp"
)

type SyncRequest struct {
	Orig string
}

type SyncResponse struct {
	Orig, Recv, Xmt string
}

func main() {
	port := os.Getenv("REPORT_PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data := map[string]string{
			"Region": strings.ToUpper(os.Getenv("FLY_REGION")),
		}

		// Set these headers to bump performance.now() precision to 5 microseconds
		headerMap := w.Header()
		headerMap.Add("Cross-Origin-Opener-Policy", "same-origin")
		headerMap.Add("Cross-Origin-Embedder-Policy", "require-corp")
		w.WriteHeader(200)

		templates.TemplateExecutor.ExecuteTemplate(w, "index.tmpl.html", data)
	})

	http.HandleFunc("/sync", func(w http.ResponseWriter, r *http.Request) {
		var syncRequest SyncRequest
		err := json.NewDecoder(r.Body).Decode(&syncRequest)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		recv := strconv.FormatUint(ntp.GetSystemTime(), 10)

		syncResponse := SyncResponse{
			Orig: syncRequest.Orig,
			Recv: recv,
			Xmt:  "",
		}

		encoder := json.NewEncoder(w)

		syncResponse.Xmt = strconv.FormatUint(ntp.GetSystemTime(), 10)
		encoder.Encode(syncResponse)
	})

	log.Println("listening on", port)
	log.Fatal(http.ListenAndServe("localhost:"+port, nil))
}
