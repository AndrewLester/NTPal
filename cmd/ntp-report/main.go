package main

import (
	"log"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/AndrewLester/ntp/internal/templates"
)

func main() {
	port := os.Getenv("REPORT_PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var now syscall.Timeval
		syscall.Gettimeofday(&now)

		nowTime := time.Unix(0, now.Nano())

		data := map[string]string{
			"Region": os.Getenv("FLY_REGION"),
			"Time":   nowTime.Format("RFC3339"),
		}
		// time, err := ntp.Time("0.pool.ntp.org")
		// if err != nil {
		// 	fmt.Fprintf(os.Stderr, "Error querying time %v", err)
		// 	templates.TemplateExecutor.ExecuteTemplate(w, "error.html.tmpl", data)
		// 	return
		// }
		// fmt.Printf("CF time: %v\n", time)

		// fmt.Printf("Our time: %v\n", time.Local())

		templates.TemplateExecutor.ExecuteTemplate(w, "index.html.tmpl", data)

	})

	log.Println("listening on", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
