package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/AndrewLester/ntp/internal/templates"
	"github.com/AndrewLester/ntp/pkg/ntp"
	"golang.org/x/sys/unix"
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
		var now unix.Timespec
		unix.ClockGettime(unix.CLOCK_MONOTONIC, &now)

		nowTime := time.Unix(now.Unix())

		data := map[string]string{
			"Region": os.Getenv("FLY_REGION"),
			"Time":   nowTime.Format(time.RFC3339),
		}
		// time, err := ntp.Time("0.pool.ntp.org")
		// if err != nil {
		// 	fmt.Fprintf(os.Stderr, "Error querying time %v", err)
		// 	templates.TemplateExecutor.ExecuteTemplate(w, "error.html.tmpl", data)
		// 	return
		// }
		// fmt.Printf("CF time: %v\n", time)

		// fmt.Printf("Our time: %v\n", time.Local())

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
			Xmt:  strconv.FormatUint(ntp.GetSystemTime(), 10),
		}

		json.NewEncoder(w).Encode(syncResponse)
	})

	log.Println("listening on", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
